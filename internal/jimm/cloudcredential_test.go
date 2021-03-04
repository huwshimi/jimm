// Copyright 2020 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestUpdateCloudCredential(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	arch := "amd64"
	mem := uint64(8096)
	cores := uint64(8)

	tests := []struct {
		about                  string
		checkCredentialErrors  []error
		updateCredentialErrors []error
		createEnv              func(*qt.C, *jimm.JIMM) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string)
	}{{
		about: "all ok",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}

			expectedCredential := cred
			expectedCredential.AuthType = "test-auth-type"
			expectedCredential.Attributes = map[string]string{
				"key1": "value1",
				"key2": "value2",
			}

			return &u, arg, expectedCredential, ""
		},
	}, {
		about:                  "update credential error returned by controller",
		updateCredentialErrors: []error{nil, errors.E("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return &u, arg, dbmodel.CloudCredential{}, "test error"
		},
	}, {
		about:                  "check credential error returned by controller",
		checkCredentialErrors:  []error{errors.E("test error")},
		updateCredentialErrors: []error{nil},
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return &u, arg, dbmodel.CloudCredential{}, "test error"
		},
	}, {
		about: "user is controller superuser",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)
			eve := dbmodel.User{
				Username: "eve@external",
			}
			c.Assert(j.Database.DB.Create(&eve).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   eve.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag("eve@external"),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/eve@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/eve@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
			}
			return &u, arg, dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				Cloud: dbmodel.Cloud{
					Name: cloud.Name,
					Type: cloud.Type,
				},
				OwnerID: eve.Username,
				Attributes: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
				AuthType: "test-auth-type",
			}, ""
		},
	}, {
		about:                 "skip check, which would return an error",
		checkCredentialErrors: []error{errors.E("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
				SkipCheck: true,
			}

			expectedCredential := cred
			expectedCredential.AuthType = "test-auth-type"
			expectedCredential.Attributes = map[string]string{
				"key1": "value1",
				"key2": "value2",
			}

			return &u, arg, expectedCredential, ""
		},
	}, {
		about: "skip update",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, jimm.UpdateCloudCredentialArgs, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}
			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			arg := jimm.UpdateCloudCredentialArgs{
				CredentialTag: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
				Credential: jujuparams.CloudCredential{
					Attributes: map[string]string{
						"key1": "value1",
						"key2": "value2",
					},
					AuthType: "test-auth-type",
				},
				SkipUpdate: true,
			}

			return &u, arg, cred, ""
		},
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			checkErrors := test.checkCredentialErrors
			updateErrors := test.updateCredentialErrors
			api := &jimmtest.API{
				SupportsCheckCredentialModels_: true,
				CheckCredentialModels_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
					if len(checkErrors) > 0 {
						var err error
						err, checkErrors = checkErrors[0], checkErrors[1:]
						if err == nil {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
							}}, nil
						} else {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
								Errors: []jujuparams.ErrorResult{{
									Error: &jujuparams.Error{
										Message: err.Error(),
										Code:    "test-error",
									},
								}},
							}}, err
						}
					} else {
						return []jujuparams.UpdateCredentialModelResult{{
							ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
							ModelName: "test-model",
						}}, nil
					}
				},
				UpdateCredential_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
					if len(updateErrors) > 0 {
						var err error
						err, updateErrors = updateErrors[0], updateErrors[1:]
						if err == nil {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
							}}, nil
						} else {
							return []jujuparams.UpdateCredentialModelResult{{
								ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
								ModelName: "test-model",
								Errors: []jujuparams.ErrorResult{{
									Error: &jujuparams.Error{
										Message: err.Error(),
										Code:    "test-error",
									},
								}},
							}}, err
						}
					} else {
						return []jujuparams.UpdateCredentialModelResult{{
							ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
							ModelName: "test-model",
						}}, nil
					}
				},
				GrantJIMMModelAdmin_: func(_ context.Context, _ names.ModelTag) error {
					return nil
				},
				CreateModel_: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
					mi.Name = args.Name
					mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
					mi.CloudTag = args.CloudTag
					mi.CloudCredentialTag = args.CloudCredentialTag
					mi.CloudRegion = args.CloudRegion
					mi.OwnerTag = args.OwnerTag
					mi.Status = jujuparams.EntityStatus{
						Status: status.Started,
						Info:   "running a test",
					}
					mi.Life = life.Alive
					mi.Users = []jujuparams.ModelUserInfo{{
						UserName: "alice@external",
						Access:   jujuparams.ModelAdminAccess,
					}, {
						// "bob" is a local user
						UserName: "bob",
						Access:   jujuparams.ModelReadAccess,
					}}
					mi.Machines = []jujuparams.ModelMachineInfo{{
						Id: "test-machine-id",
						Hardware: &jujuparams.MachineHardware{
							Arch:  &arch,
							Mem:   &mem,
							Cores: &cores,
						},
						DisplayName: "a test machine",
						Status:      "running",
						Message:     "a test message",
						HasVote:     true,
						WantsVote:   false,
					}}
					return nil
				},
			}

			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
			}

			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			u, arg, expectedCredential, expectedError := test.createEnv(c, j)

			result, err := j.UpdateCloudCredential(ctx, u, arg)
			if expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				c.Assert(result, qt.HasLen, 1)
				c.Assert(result[0].Errors, qt.HasLen, 0)
				c.Assert(result[0].ModelName, qt.Equals, "test-model")
				c.Assert(result[0].ModelUUID, qt.Equals, "00000001-0000-0000-0000-0000-000000000001")
				credential := dbmodel.CloudCredential{
					Name:      expectedCredential.Name,
					CloudName: expectedCredential.CloudName,
					OwnerID:   expectedCredential.OwnerID,
				}
				err = j.Database.GetCloudCredential(ctx, &credential)
				c.Assert(err, qt.Equals, nil)
				c.Assert(credential, jimmtest.DBObjectEquals, expectedCredential)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}

func TestRevokeCloudCredential(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	arch := "amd64"
	mem := uint64(8096)
	cores := uint64(8)

	tests := []struct {
		about                  string
		revokeCredentialErrors []error
		createEnv              func(*qt.C, *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string)
	}{{
		about: "credential revoked",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			expectedCredential := cred
			expectedCredential.Valid = sql.NullBool{
				Bool:  false,
				Valid: true,
			}
			return &u, tag, expectedCredential, ""
		},
	}, {
		about: "credential revoked - controller returns a not found error",
		revokeCredentialErrors: []error{&errors.Error{
			Message: "credential not found",
			Code:    jujuparams.CodeNotFound,
		}},
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			cred.Cloud = dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
			}

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			expectedCredential := cred
			expectedCredential.Valid = sql.NullBool{
				Bool:  false,
				Valid: true,
			}
			return &u, tag, expectedCredential, ""
		},
	}, {
		about: "credential still used by a model",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			_, err = j.AddModel(context.Background(), &u, &jimm.ModelCreateArgs{
				Name:            "test-model",
				Owner:           names.NewUserTag(u.Username),
				Cloud:           names.NewCloudTag(cloud.Name),
				CloudRegion:     "test-region-1",
				CloudCredential: names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1"),
			})
			c.Assert(err, qt.Equals, nil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, dbmodel.CloudCredential{}, `cloud credential still used by 1 model\(s\)`
		},
	}, {
		about: "user not owner of credentials - unauthorizer error",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			tag := names.NewCloudCredentialTag("test-cloud/eve@external/test-credential-1")

			return &u, tag, dbmodel.CloudCredential{}, "unauthorized access"
		},
	}, {
		about: "credential not found",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/no-such-credential")

			return &u, tag, dbmodel.CloudCredential{}, `cloudcredential "test-cloud/alice@external/no-such-credential" not found`
		},
	}, {
		about:                  "error revoking credential on controller",
		revokeCredentialErrors: []error{errors.E("test error")},
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				OwnerID:   u.Username,
				AuthType:  "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, dbmodel.CloudCredential{}, "test error"
		},
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			revokeErrors := test.revokeCredentialErrors
			api := &jimmtest.API{
				RevokeCredential_: func(context.Context, names.CloudCredentialTag) error {
					if len(revokeErrors) > 0 {
						var err error
						err, revokeErrors = revokeErrors[0], revokeErrors[1:]
						return err
					}
					return nil
				},
				UpdateCredential_: func(context.Context, jujuparams.TaggedCredential) ([]jujuparams.UpdateCredentialModelResult, error) {
					return []jujuparams.UpdateCredentialModelResult{{
						ModelUUID: "00000001-0000-0000-0000-0000-000000000001",
						ModelName: "test-model",
					}}, nil
				},
				GrantJIMMModelAdmin_: func(_ context.Context, _ names.ModelTag) error {
					return nil
				},
				CreateModel_: func(ctx context.Context, args *jujuparams.ModelCreateArgs, mi *jujuparams.ModelInfo) error {
					mi.Name = args.Name
					mi.UUID = "00000001-0000-0000-0000-0000-000000000001"
					mi.CloudTag = args.CloudTag
					mi.CloudCredentialTag = args.CloudCredentialTag
					mi.CloudRegion = args.CloudRegion
					mi.OwnerTag = args.OwnerTag
					mi.Status = jujuparams.EntityStatus{
						Status: status.Started,
						Info:   "running a test",
					}
					mi.Life = life.Alive
					mi.Users = []jujuparams.ModelUserInfo{{
						UserName: "alice@external",
						Access:   jujuparams.ModelAdminAccess,
					}, {
						// "bob" is a local user
						UserName: "bob",
						Access:   jujuparams.ModelReadAccess,
					}}
					mi.Machines = []jujuparams.ModelMachineInfo{{
						Id: "test-machine-id",
						Hardware: &jujuparams.MachineHardware{
							Arch:  &arch,
							Mem:   &mem,
							Cores: &cores,
						},
						DisplayName: "a test machine",
						Status:      "running",
						Message:     "a test message",
						HasVote:     true,
						WantsVote:   false,
					}}
					return nil
				},
			}

			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
				Dialer: &jimmtest.Dialer{
					API: api,
				},
			}

			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			user, tag, expectedCredential, expectedError := test.createEnv(c, j)

			err = j.RevokeCloudCredential(ctx, user, tag)
			if expectedError == "" {
				c.Assert(err, qt.Equals, nil)

				credential := dbmodel.CloudCredential{
					Name:      expectedCredential.Name,
					CloudName: expectedCredential.CloudName,
					OwnerID:   expectedCredential.OwnerID,
				}
				err = j.Database.GetCloudCredential(ctx, &credential)
				c.Assert(err, qt.Equals, nil)
				c.Assert(credential, jimmtest.DBObjectEquals, expectedCredential)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}

func TestGetCloudCredential(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)

	tests := []struct {
		about                  string
		revokeCredentialErrors []error
		createEnv              func(*qt.C, *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string)
	}{{
		about: "all ok",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			controller1 := dbmodel.Controller{
				Name: "test-controller-1",
				UUID: "00000000-0000-0000-0000-0000-0000000000001",
			}
			err := j.Database.AddController(context.Background(), &controller1)
			c.Assert(err, qt.Equals, nil)

			controller2 := dbmodel.Controller{
				Name: "test-controller-2",
				UUID: "00000000-0000-0000-0000-0000-0000000000002",
			}
			err = j.Database.AddController(context.Background(), &controller2)
			c.Assert(err, qt.Equals, nil)

			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			cloud := dbmodel.Cloud{
				Name: "test-cloud",
				Type: "test-provider",
				Regions: []dbmodel.CloudRegion{{
					Name: "test-region-1",
					Controllers: []dbmodel.CloudRegionControllerPriority{{
						Priority:     0,
						ControllerID: controller1.ID,
					}, {
						// controller2 has a higher priority and the model
						// should be created on this controller
						Priority:     2,
						ControllerID: controller2.ID,
					}},
				}},
				Users: []dbmodel.UserCloudAccess{{
					Username: u.Username,
				}},
			}
			c.Assert(j.Database.DB.Create(&cloud).Error, qt.IsNil)

			cred := dbmodel.CloudCredential{
				Name:      "test-credential-1",
				CloudName: cloud.Name,
				Cloud: dbmodel.Cloud{
					Name: cloud.Name,
					Type: cloud.Type,
				},
				OwnerID:  u.Username,
				AuthType: "empty",
			}
			err = j.Database.SetCloudCredential(context.Background(), &cred)
			c.Assert(err, qt.Equals, nil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, cred, ""
		},
	}, {
		about: "credential not found",
		createEnv: func(c *qt.C, j *jimm.JIMM) (*dbmodel.User, names.CloudCredentialTag, dbmodel.CloudCredential, string) {
			u := dbmodel.User{
				Username:         "alice@external",
				ControllerAccess: "superuser",
			}
			c.Assert(j.Database.DB.Create(&u).Error, qt.IsNil)

			tag := names.NewCloudCredentialTag("test-cloud/alice@external/test-credential-1")

			return &u, tag, dbmodel.CloudCredential{}, `cloudcredential "test-cloud/alice@external/test-credential-1" not found`
		},
	}}
	for _, test := range tests {
		c.Run(test.about, func(c *qt.C) {
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
				},
			}

			ctx := context.Background()
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)

			user, tag, expectedCredential, expectedError := test.createEnv(c, j)

			credential, err := j.GetCloudCredential(ctx, user, tag)
			if expectedError == "" {
				c.Assert(err, qt.Equals, nil)
				c.Assert(credential, jimmtest.DBObjectEquals, &expectedCredential)
			} else {
				c.Assert(err, qt.ErrorMatches, expectedError)
			}
		})
	}
}

const forEachUserCloudCredentialEnv = `clouds:
- name: cloud-1
  regions:
  - name: default
- name: cloud-2
  regions:
  - name: default
cloud-credentials:
- name: cred-1
  cloud: cloud-1
  owner: alice@external
  attributes:
    k1: v1
    k2: v2
- name: cred-2
  cloud: cloud-1
  owner: bob@external
  attributes:
    k1: v1
    k2: v2
- name: cred-3
  cloud: cloud-2
  owner: alice@external
- name: cred-4
  cloud: cloud-2
  owner: bob@external
- name: cred-5
  cloud: cloud-1
  owner: alice@external
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
`

var forEachUserCloudCredentialTests = []struct {
	name              string
	env               string
	username          string
	cloudTag          names.CloudTag
	f                 func(cred *dbmodel.CloudCredential) error
	expectCredentials []string
	expectError       string
	expectErrorCode   errors.Code
}{{
	name:     "UserCredentialsWithCloud",
	env:      forEachUserCloudCredentialEnv,
	username: "alice@external",
	cloudTag: names.NewCloudTag("cloud-1"),
	expectCredentials: []string{
		names.NewCloudCredentialTag("cloud-1/alice@external/cred-1").String(),
		names.NewCloudCredentialTag("cloud-1/alice@external/cred-5").String(),
	},
}, {
	name:     "UserCredentialsWithoutCloud",
	env:      forEachUserCloudCredentialEnv,
	username: "bob@external",
	expectCredentials: []string{
		names.NewCloudCredentialTag("cloud-1/bob@external/cred-2").String(),
		names.NewCloudCredentialTag("cloud-2/bob@external/cred-4").String(),
	},
}, {
	name:     "IterationError",
	env:      forEachUserCloudCredentialEnv,
	username: "alice@external",
	f: func(*dbmodel.CloudCredential) error {
		return errors.E("test error", errors.Code("test code"))
	},
	expectError:     "test error",
	expectErrorCode: "test code",
}}

func TestForEachUserCloudCredential(t *testing.T) {
	c := qt.New(t)

	for _, test := range forEachUserCloudCredentialTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, test.env)
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{},
				},
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)
			u := env.User(test.username).DBObject(c, j.Database)

			var credentials []string
			if test.f == nil {
				test.f = func(cred *dbmodel.CloudCredential) error {
					credentials = append(credentials, cred.Tag().String())
					if cred.Attributes != nil {
						return errors.E("credential contains attributes")
					}
					return nil
				}
			}
			err = j.ForEachUserCloudCredential(ctx, &u, test.cloudTag, test.f)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(credentials, qt.DeepEquals, test.expectCredentials)
		})
	}
}

const getCloudCredentialAttributesEnv = `clouds:
- name: test-cloud
  type: gce
  regions:
  - name: default
cloud-credentials:
- name: cred-1
  cloud: test-cloud
  owner: bob@external
  auth-type: oauth2
  attributes:
    client-email: bob@example.com
    client-id: 1234
    private-key: super-secret
    project-id: 5678    
users:
- username: alice@external
  controller-access: superuser
- username: bob@external
`

var getCloudCredentialAttributesTests = []struct {
	name             string
	username         string
	hidden           bool
	expectAttributes map[string]string
	expectRedacted   []string
	expectError      string
	expectErrorCode  errors.Code
}{{
	name:     "OwnerNoHidden",
	username: "bob@external",
	expectAttributes: map[string]string{
		"client-email": "bob@example.com",
		"client-id":    "1234",
		"project-id":   "5678",
	},
	expectRedacted: []string{"private-key"},
}, {
	name:     "OwnerWithHidden",
	username: "bob@external",
	hidden:   true,
	expectAttributes: map[string]string{
		"client-email": "bob@example.com",
		"client-id":    "1234",
		"private-key":  "super-secret",
		"project-id":   "5678",
	},
}, {
	name:     "SuperUserNoHidden",
	username: "alice@external",
	expectAttributes: map[string]string{
		"client-email": "bob@example.com",
		"client-id":    "1234",
		"project-id":   "5678",
	},
	expectRedacted: []string{"private-key"},
}, {
	name:            "SuperUserWithHiddenUnauthorized",
	username:        "alice@external",
	hidden:          true,
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}, {
	name:            "OtherUserUnauthorized",
	username:        "charlie@external",
	expectError:     `unauthorized access`,
	expectErrorCode: errors.CodeUnauthorized,
}}

func TestGetCloudCredentialAttributes(t *testing.T) {
	c := qt.New(t)

	for _, test := range getCloudCredentialAttributesTests {
		c.Run(test.name, func(c *qt.C) {
			ctx := context.Background()

			env := jimmtest.ParseEnvironment(c, getCloudCredentialAttributesEnv)
			j := &jimm.JIMM{
				Database: db.Database{
					DB: jimmtest.MemoryDB(c, nil),
				},
				Dialer: &jimmtest.Dialer{
					API: &jimmtest.API{},
				},
			}
			err := j.Database.Migrate(ctx, false)
			c.Assert(err, qt.IsNil)
			env.PopulateDB(c, j.Database)
			u := env.User("bob@external").DBObject(c, j.Database)
			cred, err := j.GetCloudCredential(ctx, &u, names.NewCloudCredentialTag("test-cloud/bob@external/cred-1"))
			c.Assert(err, qt.IsNil)

			u = env.User(test.username).DBObject(c, j.Database)
			attr, redacted, err := j.GetCloudCredentialAttributes(ctx, &u, cred, test.hidden)
			if test.expectError != "" {
				c.Check(err, qt.ErrorMatches, test.expectError)
				if test.expectErrorCode != "" {
					c.Check(errors.ErrorCode(err), qt.Equals, test.expectErrorCode)
				}
				return
			}
			c.Assert(err, qt.IsNil)
			c.Check(attr, qt.DeepEquals, test.expectAttributes)
			c.Check(redacted, qt.DeepEquals, test.expectRedacted)
		})
	}
}