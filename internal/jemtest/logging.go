package jemtest

import (
	"strings"

	"github.com/juju/loggo"
	"github.com/uber-go/zap"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
)

// LoggingSuite is a replacement for github.com/juju/testing.LoggingSuite
// zap logging but also replaces the global loggo logger.
// When used with juju/testing.LoggingSuite, it should
// be set up after that.
type LoggingSuite struct{}

func (s *LoggingSuite) SetUpSuite(c *gc.C) {
	s.setUp(c)
}

func (s *LoggingSuite) TearDownSuite(c *gc.C) {
}

func (s *LoggingSuite) SetUpTest(c *gc.C) {
	s.setUp(c)
}

func (s *LoggingSuite) TearDownTest(c *gc.C) {
}

func (s *LoggingSuite) setUp(c *gc.C) {
	output := gocheckZapWriter{c}
	logger := zap.New(zap.NewTextEncoder(zap.TextNoTime()), zap.Output(output), zap.DebugLevel)
	zapctx.Default = logger

	loggo.ResetLogging()
	// Don't use the default writer for the test logging, which
	// means we can still get logging output from tests that
	// replace the default writer.
	loggo.RegisterWriter(loggo.DefaultWriterName, discardWriter{})
	loggo.RegisterWriter("loggingsuite", zaputil.NewLoggoWriter(logger))
	err := loggo.ConfigureLoggers("DEBUG")
	c.Assert(err, gc.IsNil)
}

type discardWriter struct{}

func (discardWriter) Write(entry loggo.Entry) {
}

type gocheckZapWriter struct {
	c *gc.C
}

func (w gocheckZapWriter) Write(buf []byte) (int, error) {
	w.c.Output(1, strings.TrimSuffix(string(buf), "\n"))
	return len(buf), nil
}

func (w gocheckZapWriter) Sync() error {
	return nil
}