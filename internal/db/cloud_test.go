// Copyright 2020 Canonical Ltd.

package db_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
)

func TestAddCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.AddCloud(context.Background(), &dbmodel.Cloud{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestAddCloud(c *qt.C) {
	ctx := context.Background()

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "dummy",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "dummy-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    "everyone@external",
				DisplayName: "everyone",
			},
			Access: "add-model",
		}},
	}

	err := s.Database.AddCloud(ctx, &cl)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.AddCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	cl2 := dbmodel.Cloud{
		Name: cl.Name,
	}
	err = s.Database.AddCloud(ctx, &cl2)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeAlreadyExists)

	cl3 := dbmodel.Cloud{
		Name: cl.Name,
	}

	err = s.Database.GetCloud(ctx, &cl3)
	c.Assert(err, qt.IsNil)
	c.Check(cl3, qt.CmpEquals(cmpopts.EquateEmpty()), cl)
}

func TestGetCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.GetCloud(context.Background(), &dbmodel.Cloud{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestGetCloud(c *qt.C) {
	ctx := context.Background()

	cl := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err := s.Database.GetCloud(ctx, &cl)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetCloud(ctx, &cl)
	c.Check(err, qt.ErrorMatches, `cloud "test-cloud" not found`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeNotFound)

	cl2 := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "dummy",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "dummy-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    "everyone@external",
				DisplayName: "everyone",
			},
			Access: "add-model",
		}},
	}

	err = s.Database.AddCloud(ctx, &cl2)
	c.Assert(err, qt.IsNil)

	err = s.Database.GetCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)
	c.Check(cl, qt.CmpEquals(cmpopts.EquateEmpty()), cl2)
}

func TestSetCloudUnconfiguredDatabase(t *testing.T) {
	c := qt.New(t)

	var d db.Database
	err := d.SetCloud(context.Background(), &dbmodel.Cloud{})
	c.Check(err, qt.ErrorMatches, `database not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func (s *dbSuite) TestSetCloud(c *qt.C) {
	ctx := context.Background()

	cl := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "dummy",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "dummy-region",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    "everyone@external",
				DisplayName: "everyone",
			},
			Access: "add-model",
		}, {
			User: dbmodel.User{
				Username:    "alice@external",
				DisplayName: "Alice",
			},
			Access: "add-model",
		}},
	}

	err := s.Database.SetCloud(ctx, &cl)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUpgradeInProgress)

	err = s.Database.Migrate(context.Background(), false)
	c.Assert(err, qt.IsNil)

	err = s.Database.SetCloud(ctx, &cl)
	c.Assert(err, qt.IsNil)

	cl2 := dbmodel.Cloud{
		Name:             "test-cloud",
		Type:             "dummy",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		Regions: []dbmodel.CloudRegion{{
			Name: "dummy-region-2",
		}},
		CACertificates: dbmodel.Strings{"CACERT 1", "CACERT 2"},
		Users: []dbmodel.UserCloudAccess{{
			User: dbmodel.User{
				Username:    "alice@external",
				DisplayName: "Alice",
			},
			Access: "admin",
		}, {
			User: dbmodel.User{
				Username:    "bob@external",
				DisplayName: "Bob",
			},
			Access: "add-model",
		}},
	}

	err = s.Database.SetCloud(ctx, &cl2)
	c.Assert(err, qt.IsNil)

	cl3 := dbmodel.Cloud{
		Name: "test-cloud",
	}
	err = s.Database.GetCloud(ctx, &cl3)
	c.Assert(err, qt.IsNil)
	c.Check(cl3, qt.CmpEquals(cmpopts.EquateEmpty()), dbmodel.Cloud{
		ID:               cl.ID,
		CreatedAt:        cl.CreatedAt,
		UpdatedAt:        cl.UpdatedAt,
		Name:             "test-cloud",
		Type:             "dummy",
		Endpoint:         "https://example.com",
		IdentityEndpoint: "https://identity.example.com",
		StorageEndpoint:  "https://storage.example.com",
		CACertificates:   []string{"CACERT 1", "CACERT 2"},
		Regions: []dbmodel.CloudRegion{
			cl.Regions[0],
			cl2.Regions[0],
		},
		Users: []dbmodel.UserCloudAccess{
			cl.Users[0],
			dbmodel.UserCloudAccess{
				Model: gorm.Model{
					ID:        cl.Users[1].ID,
					CreatedAt: cl.Users[1].CreatedAt,
					UpdatedAt: cl2.Users[0].UpdatedAt,
				},
				Username:  "alice@external",
				User:      cl.Users[1].User,
				CloudName: "test-cloud",
				Access:    "admin",
			},
			cl2.Users[1],
		},
	})
}
