// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestGetUserUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetUser(context.Background(), &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetUser(c *qt.C) {
	ctx := context.Background()
	err := s.Database.GetUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `upgrade in progress`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetUser(ctx, &dbmodel.User{})
	c.Check(err, qt.ErrorMatches, `invalid username ""`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	u := dbmodel.User{
		Username: "bob@external",
	}
	err = s.Database.GetUser(ctx, &u)
	c.Assert(err, qt.IsNil)
	c.Check(u.ControllerAccess, qt.Equals, "add-model")

	u2 := dbmodel.User{
		Username: u.Username,
	}
	err = s.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)
	c.Check(u2, qt.DeepEquals, u)
}
