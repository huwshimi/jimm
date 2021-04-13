// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"
	"gorm.io/gorm"

	"github.com/CanonicalLtd/jimm/internal/dbmodel"
)

func TestVersion(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	var v0 dbmodel.Version
	result := db.First(&v0, "component = ?", dbmodel.Component)
	c.Check(result.Error, qt.Equals, gorm.ErrRecordNotFound)

	v1 := dbmodel.Version{
		Component: dbmodel.Component,
		Major:     dbmodel.Major,
		Minor:     dbmodel.Minor,
	}
	result = db.Create(&v1)
	c.Assert(result.Error, qt.IsNil)
	c.Check(result.RowsAffected, qt.Equals, int64(1))

	var v2 dbmodel.Version
	result = db.First(&v2, "component = ?", dbmodel.Component)
	c.Assert(result.Error, qt.IsNil)
	c.Check(v2, qt.DeepEquals, v1)

	v3 := dbmodel.Version{
		Component: dbmodel.Component,
		Major:     v1.Major + 1,
		Minor:     v1.Minor + 1,
	}
	result = db.Create(&v3)
	c.Check(result.Error, qt.ErrorMatches, "UNIQUE constraint failed: versions.component")
	result = db.Save(&v3)
	c.Assert(result.Error, qt.IsNil)

	var v4 dbmodel.Version
	result = db.First(&v4, "component = ?", dbmodel.Component)
	c.Assert(result.Error, qt.IsNil)
	c.Check(v4, qt.DeepEquals, v3)
}
