// Copyright 2015 Canonical Ltd.

package jem_test

import (
	"github.com/CanonicalLtd/blues-identity/idmclient"
	"github.com/juju/schema"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"

	"github.com/CanonicalLtd/jem/internal/idmtest"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
)

type jemSuite struct {
	jujutesting.IsolatedMgoSuite
	idmSrv *idmtest.Server
	pool   *jem.Pool
	store  *jem.JEM
}

var _ = gc.Suite(&jemSuite{})

func (s *jemSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	s.idmSrv = idmtest.NewServer()
	pool, err := jem.NewPool(
		jem.ServerParams{
			DB: s.Session.DB("jem"),
		},
		bakery.NewServiceParams{
			Location: "here",
		},
		idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	)
	c.Assert(err, gc.IsNil)
	s.pool = pool
	s.store = s.pool.JEM()
}

func (s *jemSuite) TearDownTest(c *gc.C) {
	s.store.Close()
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *jemSuite) TestAddController(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	ctl := &mongodoc.Controller{
		Id:        "ignored",
		Path:      ctlPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	m := &mongodoc.Model{
		Id:            "ignored",
		Path:          params.EntityPath{"ignored-user", "ignored-name"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddController(ctl, m)
	c.Assert(err, gc.IsNil)

	// Check that the fields have been mutated as expected.
	c.Assert(ctl, jc.DeepEquals, &mongodoc.Controller{
		Id:        "bob/x",
		Path:      ctlPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	})
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:            "bob/x",
		Path:          ctlPath,
		Controller:    ctlPath,
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	})

	ctl1, err := s.store.Controller(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(ctl1, jc.DeepEquals, &mongodoc.Controller{
		Id:        "bob/x",
		Path:      ctlPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	})
	m1, err := s.store.Model(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	err = s.store.AddController(ctl, m)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)

	ctlPath2 := params.EntityPath{"bob", "y"}
	ctl2 := &mongodoc.Controller{
		Id:        "ignored",
		Path:      ctlPath2,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	m2 := &mongodoc.Model{
		Id:            "bob/noty",
		Path:          params.EntityPath{"ignored-user", "ignored-name"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err = s.store.AddController(ctl2, m2)
	c.Assert(err, gc.IsNil)
	m3, err := s.store.Model(ctlPath2)
	c.Assert(err, gc.IsNil)
	c.Assert(m3, jc.DeepEquals, m2)
}

func (s *jemSuite) TestDeleteController(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:        "ignored",
		Path:      ctlPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	m := &mongodoc.Model{
		Id:            "dalek/who",
		Path:          params.EntityPath{"ignored", "ignored"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddController(ctl, m)
	c.Assert(err, gc.IsNil)
	err = s.store.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)

	ctl1, err := s.store.Controller(ctlPath)
	c.Assert(ctl1, gc.IsNil)
	m1, err := s.store.Model(ctlPath)
	c.Assert(m1, gc.IsNil)

	err = s.store.DeleteController(ctlPath)
	c.Assert(err, gc.ErrorMatches, "controller \"dalek/who\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Test with non-existing model.
	ctl2 := &mongodoc.Controller{
		Id:        "dalek/who",
		Path:      ctlPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	m2 := &mongodoc.Model{
		Id:            "dalek/exterminated",
		Path:          params.EntityPath{"ignored", "ignored"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err = s.store.AddController(ctl2, m2)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteController(ctlPath)
	c.Assert(err, gc.IsNil)
	ctl3, err := s.store.Controller(ctlPath)
	c.Assert(ctl3, gc.IsNil)
	m3, err := s.store.Model(ctlPath)
	c.Assert(m3, gc.IsNil)
}

func (s *jemSuite) TestDeleteModelemnt(c *gc.C) {
	ctlPath := params.EntityPath{"dalek", "who"}
	ctl := &mongodoc.Controller{
		Id:        "ignored",
		Path:      ctlPath,
		CACert:    "certainly",
		HostPorts: []string{"host1:1234", "host2:9999"},
	}
	m := &mongodoc.Model{
		Id:            "dalek/who",
		Path:          params.EntityPath{"ignored", "ignored"},
		AdminUser:     "foo-admin",
		AdminPassword: "foo-password",
	}
	err := s.store.AddController(ctl, m)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteModel(m.Path)
	c.Assert(err, gc.ErrorMatches, `cannot remove model "dalek/who" because it is a controller`)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrForbidden)

	modelPath := params.EntityPath{"dalek", "exterminate"}
	m2 := &mongodoc.Model{
		Id:   "dalek/exterminate",
		Path: modelPath,
	}
	err = s.store.AddModel(m2)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteModel(m2.Path)
	c.Assert(err, gc.IsNil)
	m3, err := s.store.Model(modelPath)
	c.Assert(m3, gc.IsNil)

	err = s.store.DeleteModel(m2.Path)
	c.Assert(err, gc.ErrorMatches, "model \"dalek/exterminate\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestAddModel(c *gc.C) {
	ctlPath := params.EntityPath{"bob", "x"}
	m := &mongodoc.Model{
		Id:   "ignored",
		Path: ctlPath,
	}
	err := s.store.AddModel(m)
	c.Assert(err, gc.IsNil)
	c.Assert(m, jc.DeepEquals, &mongodoc.Model{
		Id:   "bob/x",
		Path: ctlPath,
	})

	m1, err := s.store.Model(ctlPath)
	c.Assert(err, gc.IsNil)
	c.Assert(m1, jc.DeepEquals, m)

	err = s.store.AddModel(m)
	c.Assert(err, gc.ErrorMatches, "already exists")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)
}

func (s *jemSuite) TestAddTemplate(c *gc.C) {
	path := params.EntityPath{"bob", "x"}
	tmpl := &mongodoc.Template{
		Id:   "ignored",
		Path: path,
		Schema: environschema.Fields{
			"name": {
				Description: "name of model",
				Type:        environschema.Tstring,
				Mandatory:   true,
				Values:      []interface{}{"venus", "pluto"},
			},
			"temperature": {
				Description: "temperature of model",
				Type:        environschema.Tint,
				Example:     400,
				Values:      []interface{}{-400, 864.0},
			},
		},
		Config: map[string]interface{}{
			"name":        "pluto",
			"temperature": -400.0,
		},
	}
	err := s.store.AddTemplate(tmpl)
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl.Id, gc.Equals, "bob/x")

	tmpl1, err := s.store.Template(path)
	c.Assert(err, gc.IsNil)
	c.Assert(tmpl1, jc.DeepEquals, tmpl)

	// Ensure that the schema still works even though some
	// values may have been transformed to float64 by the
	// JSON unmarshaler.
	fields, defaults, err := tmpl.Schema.ValidationSchema()
	c.Assert(err, gc.IsNil)
	config, err := schema.FieldMap(fields, defaults).Coerce(tmpl.Config, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(config, jc.DeepEquals, map[string]interface{}{
		"name":        "pluto",
		"temperature": -400,
	})
}

func (s *jemSuite) TestDeleteTemplate(c *gc.C) {
	path := params.EntityPath{"bob", "x"}
	tmpl := &mongodoc.Template{
		Id:   "ignored",
		Path: path,
		Schema: environschema.Fields{
			"name": {
				Description: "name of model",
				Type:        environschema.Tstring,
				Mandatory:   true,
				Values:      []interface{}{"venus", "pluto"},
			},
			"temperature": {
				Description: "temperature of model",
				Type:        environschema.Tint,
				Example:     400,
				Values:      []interface{}{-400, 864.0},
			},
		},
		Config: map[string]interface{}{
			"name":        "pluto",
			"temperature": -400.0,
		},
	}
	err := s.store.AddTemplate(tmpl)
	c.Assert(err, gc.IsNil)

	err = s.store.DeleteTemplate(tmpl.Path)
	c.Assert(err, gc.IsNil)
	tmpl1, err := s.store.Template(path)
	c.Assert(tmpl1, gc.IsNil)

	err = s.store.DeleteTemplate(tmpl.Path)
	c.Assert(err, gc.ErrorMatches, "template \"bob/x\" not found")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *jemSuite) TestSessionIsCopied(c *gc.C) {
	session := s.Session.Copy()
	pool, err := jem.NewPool(
		jem.ServerParams{
			DB: s.Session.DB("jem"),
		},
		bakery.NewServiceParams{
			Location: "here",
		},
		idmclient.New(idmclient.NewParams{
			BaseURL: s.idmSrv.URL.String(),
			Client:  s.idmSrv.Client("agent"),
		}),
	)
	c.Assert(err, gc.IsNil)

	store := pool.JEM()
	defer store.Close()
	// Check that we get an appropriate error when getting
	// a non-existent model, indicating that database
	// access is going OK.
	_, err = store.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Close the session and check that we still get the
	// same error.
	session.Close()

	_, err = store.Model(params.EntityPath{"bob", "x"})
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// Also check the macaroon storage as that also has its own session reference.
	m, err := store.Bakery.NewMacaroon("", nil, nil)
	c.Assert(err, gc.IsNil)
	c.Assert(m, gc.NotNil)
}
