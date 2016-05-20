// Copyright 2016 Canonical Ltd.

// Package monitor provides monitoring for the controllers in JEM.
//
// We maintain a lease field
// in each controller which we hold as long as we monitor
// the controller so that we don't have multiple units redundantly
// monitoring the same controller.
package monitor

import (
	"time"

	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/params"
)

var logger = loggo.GetLogger("jem.internal.monitor")

var (
	// leaseAcquireInterval holds the duration the
	// monitor waits before trying to reacquire new
	// controller monitor leases.
	leaseAcquireInterval = 30 * time.Second

	// leaseExpiryDuration holds the length of time
	// a lease is acquired for.
	leaseExpiryDuration = time.Minute

	// apiConnectRetryDuration holds the length of time
	// a controller monitor will wait after a failed API
	// connection before trying again.
	apiConnectRetryDuration = 5 * time.Second
)

// Clock holds the clock implementation used by the monitor.
// This is exported so it can be changed for testing purposes.
var Clock clock.Clock = clock.WallClock

// Monitor represents the JEM controller monitoring system.
type Monitor struct {
	pool    *jem.Pool
	tomb    tomb.Tomb
	ownerId string
}

// New returns a new Monitor that will monitor controllers
// that JEM knows about. It uses the given JEM pool for
// accessing the database.
func New(p *jem.Pool, ownerId string) *Monitor {
	m := &Monitor{
		pool:    p,
		ownerId: ownerId,
	}
	m.tomb.Go(m.run)
	return m
}

// Kill asks the monitor to shut down but doesn't
// wait for it to stop.
func (m *Monitor) Kill() {
	m.tomb.Kill(nil)
}

// Wait waits for the monitor to shut down and
// returns any error encountered while it was running.
func (m *Monitor) Wait() error {
	return m.tomb.Wait()
}

func (m *Monitor) run() error {
	for {
		shim := jemShim{m.pool.JEM()}
		m1 := newAllMonitor(shim, m.ownerId)
		select {
		case <-m1.tomb.Dead():
			logger.Warningf("restarting inner monitor after error: %v", m1.tomb.Err())
			shim.Close()
		case <-m.tomb.Dying():
			m1.Kill()
			err := m1.Wait()
			logger.Warningf("inner monitor error during shutdown: %v", err)
			shim.Close()
			return tomb.ErrDying
		}
	}
}

func newAllMonitor(jem jemInterface, ownerId string) *allMonitor {
	m := &allMonitor{
		jem:               jem,
		monitoring:        make(map[params.EntityPath]bool),
		ownerId:           ownerId,
		controllerRemoved: make(chan params.EntityPath),
	}
	m.tomb.Go(m.run)
	return m
}

// Kill implements worker.Worker.Kill.
func (m *allMonitor) Kill() {
	m.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (m *allMonitor) Wait() error {
	return m.tomb.Wait()
}

// Dead returns a channel which is closed when the
// allMonitor has terminated.
func (m *allMonitor) Dead() <-chan struct{} {
	return m.tomb.Dead()
}

// allMonitor is responsible for monitoring all controllers using
// a single JEM connection. It will die if when cannot use
// the connection.
type allMonitor struct {
	tomb    tomb.Tomb
	jem     jemInterface
	ownerId string

	// controllerRemoved receives a value when a controller
	// monitor terminates, holding the path of that controller.
	controllerRemoved chan params.EntityPath

	// monitoring holds a map of all the controllers
	// we are currently monitoring. This field is accessed
	// only by the allMonitor.run goroutine.
	monitoring map[params.EntityPath]bool
}

func (m *allMonitor) run() error {
	for {
		if err := m.startMonitors(); err != nil {
			return errgo.Notef(err, "cannot start monitors")
		}
	waitLoop:
		for {
			select {
			case ctlId := <-m.controllerRemoved:
				delete(m.monitoring, ctlId)
			case <-Clock.After(leaseAcquireInterval):
				break waitLoop
			case <-m.tomb.Dying():
				// Wait for all the controller monitors to terminate.
				for len(m.monitoring) > 0 {
					delete(m.monitoring, <-m.controllerRemoved)
				}
				return tomb.ErrDying
			}
		}
	}
}

// startMonitors starts monitoring all controllers that are
// not currently being monitored.
func (m *allMonitor) startMonitors() error {
	ctls, err := m.jem.AllControllers()
	if err != nil {
		return errgo.Notef(err, "cannot get controllers")
	}
	for _, ctl := range ctls {
		ctl := ctl
		if m.monitoring[ctl.Path] {
			// We're already monitoring this controller; no need to do anything.
			logger.Debugf("already monitoring %v", ctl.Path)
			continue
		}
		if ctl.MonitorLeaseOwner != m.ownerId && Clock.Now().Before(ctl.MonitorLeaseExpiry) {
			// Someone else already holds the lease.
			continue
		}
		newExpiry, err := acquireLease(m.jem, ctl.Path, ctl.MonitorLeaseExpiry, ctl.MonitorLeaseOwner, m.ownerId)
		if isMonitoringStoppedError(err) {
			logger.Infof("cannot acquire lease on %v: %v", ctl.Path, err)
			// Someone else got there first.
			continue
		}
		if err != nil {
			return errgo.Notef(err, "cannot acquire lease")
		}
		// We've acquired the lease.
		m.monitoring[ctl.Path] = true

		ctlMonitor := newControllerMonitor(controllerMonitorParams{
			ctlPath:     ctl.Path,
			jem:         m.jem,
			ownerId:     m.ownerId,
			leaseExpiry: newExpiry,
		})
		m.tomb.Go(func() error {
			select {
			case <-ctlMonitor.Dead():
				// The controller monitor has terminated.
			case <-m.tomb.Dying():
				// The allMonitor is terminating; kill the
				// controller monitor.
				ctlMonitor.Kill()
			}
			err := ctlMonitor.Wait()
			logger.Infof("controller monitor died (path %v): %v", ctl.Path, err)
			m.controllerRemoved <- ctl.Path
			if isMonitoringStoppedError(err) {
				return nil
			}
			return errgo.Mask(err)
		})
	}
	return nil
}