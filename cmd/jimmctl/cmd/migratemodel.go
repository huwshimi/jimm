// Copyright 2023 Canonical Ltd.

package cmd

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	jujuapi "github.com/juju/juju/api"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/api"
	apiparams "github.com/canonical/jimm/api/params"
	"github.com/canonical/jimm/internal/errors"
)

var migrateModelCommandDoc = `
	The migrate command migrates a model(s) to a new controller. Specify
	a model-tag to migrate and the destination controller name.
	A model-tag is of the form "model-<UUID>" while a controller name is
	simply the name of the controller.

	Note that multiple models can be targeted for migration by supplying
	multiple model tags.

	Example:
		jimmctl migrate <controller-name> <model-tag> 
		jimmctl migrate <controller-name> <model-tag> <model-tag> <model-tag>
`

// NewMigrateModelCommand returns a command to migrate models.
func NewMigrateModelCommand() cmd.Command {
	cmd := &migrateModelCommand{
		store: jujuclient.NewFileClientStore(),
	}

	return modelcmd.WrapBase(cmd)
}

// migrateModelCommand migrates a model.
type migrateModelCommand struct {
	modelcmd.ControllerCommandBase
	out cmd.Output

	store            jujuclient.ClientStore
	dialOpts         *jujuapi.DialOpts
	targetController string
	modelTags        []string
}

func (c *migrateModelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "migrate",
		Purpose: "Migrate models to the target controller",
		Doc:     migrateModelCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *migrateModelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Init implements the cmd.Command interface.
func (c *migrateModelCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.E("Missing controller and model tag arguments")
	}
	for i, arg := range args {
		if i == 0 {
			c.targetController = arg
			continue
		}
		_, err := names.ParseModelTag(arg)
		if err != nil {
			return errors.E(err, fmt.Sprintf("%s is not a valid model tag", arg))
		}
		c.modelTags = append(c.modelTags, arg)
	}
	return nil
}

// Run implements Command.Run.
func (c *migrateModelCommand) Run(ctxt *cmd.Context) error {
	currentController, err := c.store.CurrentController()
	if err != nil {
		return errors.E(err, "could not determine controller")
	}

	apiCaller, err := c.NewAPIRootWithDialOpts(c.store, currentController, "", c.dialOpts)
	if err != nil {
		return err
	}

	client := api.NewClient(apiCaller)
	specs := []apiparams.MigrateModelInfo{}
	for _, model := range c.modelTags {
		specs = append(specs, apiparams.MigrateModelInfo{ModelTag: model, TargetController: c.targetController})
	}
	req := apiparams.MigrateModelRequest{Specs: specs}
	events, err := client.MigrateModel(&req)
	if err != nil {
		return err
	}

	err = c.out.Write(ctxt, events)
	if err != nil {
		return err
	}
	return nil
}
