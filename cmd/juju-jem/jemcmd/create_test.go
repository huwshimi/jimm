// Copyright 2015 Canonical Ltd.

package jemcmd_test

import (
	"io/ioutil"
	"path/filepath"

	"github.com/juju/juju/juju"
	"github.com/juju/juju/jujuclient"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/yaml.v1"

	"github.com/CanonicalLtd/jem/params"
)

type createSuite struct {
	commonSuite
}

var _ = gc.Suite(&createSuite{})

var createErrorTests = []struct {
	about        string
	args         []string
	expectStderr string
	expectCode   int
}{{
	about:        "too few arguments",
	args:         []string{},
	expectStderr: "missing model name argument",
	expectCode:   2,
}, {
	about:        "only one part in model id",
	args:         []string{"a"},
	expectStderr: `invalid entity path "a": wrong number of parts in entity path`,
	expectCode:   2,
}, {
	about:        "controller cannot be specified with location",
	args:         []string{"foo/bar", "-c", "xx/yy", "foo=bar"},
	expectStderr: `cannot specify explicit controller name with location`,
	expectCode:   2,
}, {
	about:        "invalid location key-value",
	args:         []string{"foo/bar", "foobar"},
	expectStderr: `expected "key=value", got "foobar"`,
	expectCode:   2,
}}

func (s *createSuite) TestCreateError(c *gc.C) {
	for i, test := range createErrorTests {
		c.Logf("test %d: %s", i, test.about)
		stdout, stderr, code := run(c, c.MkDir(), "create", test.args...)
		c.Assert(code, gc.Equals, test.expectCode, gc.Commentf("stderr: %s", stderr))
		c.Assert(stderr, gc.Matches, "(error:|ERROR) "+test.expectStderr+"\n")
		c.Assert(stdout, gc.Equals, "")
	}
}

func (s *createSuite) TestCreate(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"-c", "bob/foo",
		"--config", configPath,
		"bob/newmodel",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-foo:newmodel\n")

	// Check that we can attach to the new model
	// through the usual juju connection mechanism.
	store := jujuclient.NewFileClientStore()
	params, err := newAPIConnectionParams(
		store, "jem-foo", "", "newmodel", httpbakery.NewClient(),
	)
	c.Assert(err, gc.IsNil)
	client, err := juju.NewAPIConnection(params)
	c.Assert(err, jc.ErrorIsNil)
	client.Close()
}

func (s *createSuite) TestCreateWithTemplate(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/foo")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Then add a template containing the mandatory controller parameter.
	stdout, stderr, code = run(c, c.MkDir(), "create-template", "bob/template", "-c", "bob/foo", "controller=true")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	// Then create an model that uses the template as additional config.
	// Note that because the controller attribute is mandatory, this
	// will fail if the template logic is not working correctly.
	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
	})
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"-c", "bob/foo",
		"--config", configPath,
		"-t", "bob/template",
		"bob/newmodel",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-foo:newmodel\n")

	// Check that we can attach to the new model
	// through the usual juju connection mechanism.
	store := jujuclient.NewFileClientStore()
	params, err := newAPIConnectionParams(
		store, "jem-foo", "", "newmodel", httpbakery.NewClient(),
	)
	c.Assert(err, jc.ErrorIsNil)
	client, err := juju.NewAPIConnection(params)
	c.Assert(err, jc.ErrorIsNil)
	client.Close()
}

func (s *createSuite) TestCreateWithLocation(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	// First add the controller that we're going to use
	// to create the new model.
	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/aws", "cloud=aws")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	stdout, stderr, code = run(c, c.MkDir(), "add-controller", "bob/azure", "cloud=azure")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"bob/newmodel",
		"cloud=aws",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-aws:newmodel\n")

	client := s.jemClient("bob")
	m, err := client.GetModel(&params.GetModel{
		EntityPath: params.EntityPath{"bob", "newmodel"},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(m.ControllerPath.String(), gc.Equals, "bob/aws")
}

func (s *createSuite) TestCreateWithLocationWithExistingModel(c *gc.C) {
	s.idmSrv.SetDefaultUser("bob")

	stdout, stderr, code := run(c, c.MkDir(), "add-controller", "bob/aws", "cloud=aws")
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "")

	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"bob/newmodel",
		"cloud=aws",
	)
	c.Assert(code, gc.Equals, 0, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Equals, "jem-aws:newmodel\n")

	// Create a second model with the same local name.
	// This should be rejected even though we haven't
	// specified a controller, because all jem controllers should
	// be searched.
	stdout, stderr, code = run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"--local", "newmodel",
		"bob/anothermodel",
		"cloud=aws",
	)
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR local model "newmodel" already exists in controller "jem-aws"\n`)
}

func (s *createSuite) TestCreateWithLocationNoMatch(c *gc.C) {
	configPath := writeConfig(c, map[string]interface{}{
		"authorized-keys": fakeSSHKey,
		"controller":      true,
	})
	stdout, stderr, code := run(c, c.MkDir(),
		"create",
		"--config", configPath,
		"bob/newmodel",
	)
	c.Assert(code, gc.Equals, 1, gc.Commentf("stderr: %s", stderr))
	c.Assert(stdout, gc.Equals, "")
	c.Assert(stderr, gc.Matches, `ERROR cannot get schema info: GET http://.*: no matching controllers\n`)
}

func writeConfig(c *gc.C, config map[string]interface{}) string {
	data, err := yaml.Marshal(config)
	c.Assert(err, gc.IsNil)
	configPath := filepath.Join(c.MkDir(), "config.yaml")
	err = ioutil.WriteFile(configPath, data, 0666)
	c.Assert(err, gc.IsNil)
	return configPath
}
