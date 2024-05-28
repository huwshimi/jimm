// Copyright 2024 Canonical Ltd.

package cmd_test

import (
	"context"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/names/v5"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/cmd/jaas/cmd"
	"github.com/canonical/jimm/internal/cmdtest"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
	jimmnames "github.com/canonical/jimm/pkg/names"
	jujucloud "github.com/juju/juju/cloud"
)

type updateCredentialsSuite struct {
	cmdtest.JimmCmdSuite
}

var _ = gc.Suite(&updateCredentialsSuite{})

func (s *updateCredentialsSuite) TestUpdateCredentialsWithNewCredentials(c *gc.C) {
	ctx := context.Background()

	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	clientIDWithDomain := clientID + "@serviceaccount"

	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	sa, err := dbmodel.NewIdentity(clientIDWithDomain)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.GetIdentity(ctx, sa)
	c.Assert(err, gc.IsNil)

	// Make alice admin of the service account
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientIDWithDomain)),
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuple)
	c.Assert(err, gc.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "kubernetes",
	}
	err = s.JIMM.Database.AddCloud(ctx, &cloud)
	c.Assert(err, gc.IsNil)

	clientStore := s.ClientStore()

	err = clientStore.UpdateCredential("test-cloud", jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"test-credentials": jujucloud.NewCredential(jujucloud.EmptyAuthType, map[string]string{
				"foo": "bar",
			}),
		},
	})
	c.Assert(err, gc.IsNil)

	cmdContext, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient), clientID, "test-cloud", "test-credentials")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(cmdContext), gc.Equals, `results:
- credentialtag: cloudcred-test-cloud_abda51b2-d735-4794-a8bd-49c506baa4af@serviceaccount_test-credentials
  error: null
  models: []
`)

	ofgaUser := openfga.NewUser(sa, s.JIMM.AuthorizationClient())
	cloudCredentialTag := names.NewCloudCredentialTag("test-cloud/" + clientIDWithDomain + "/test-credentials")
	cloudCredential2, err := s.JIMM.GetCloudCredential(ctx, ofgaUser, cloudCredentialTag)
	c.Assert(err, gc.IsNil)
	attrs, _, err := s.JIMM.GetCloudCredentialAttributes(ctx, ofgaUser, cloudCredential2, true)
	c.Assert(err, gc.IsNil)

	c.Assert(attrs, gc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *updateCredentialsSuite) TestUpdateCredentialsWithExistingCredentials(c *gc.C) {
	ctx := context.Background()

	clientID := "abda51b2-d735-4794-a8bd-49c506baa4af"
	clientIDWithDomain := clientID + "@serviceaccount"

	// alice is superuser
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	sa, err := dbmodel.NewIdentity(clientIDWithDomain)
	c.Assert(err, gc.IsNil)
	err = s.JIMM.Database.GetIdentity(ctx, sa)
	c.Assert(err, gc.IsNil)

	// Make alice admin of the service account
	tuple := openfga.Tuple{
		Object:   ofganames.ConvertTag(names.NewUserTag("alice@canonical.com")),
		Relation: ofganames.AdministratorRelation,
		Target:   ofganames.ConvertTag(jimmnames.NewServiceAccountTag(clientIDWithDomain)),
	}
	err = s.JIMM.OpenFGAClient.AddRelation(ctx, tuple)
	c.Assert(err, gc.IsNil)

	cloud := dbmodel.Cloud{
		Name: "test-cloud",
		Type: "kubernetes",
	}
	err = s.JIMM.Database.AddCloud(ctx, &cloud)
	c.Assert(err, gc.IsNil)

	cloudCredential := dbmodel.CloudCredential{
		Name:              "test-credentials",
		CloudName:         "test-cloud",
		OwnerIdentityName: clientIDWithDomain,
		AuthType:          "empty",
	}
	err = s.JIMM.Database.SetCloudCredential(ctx, &cloudCredential)
	c.Assert(err, gc.IsNil)

	clientStore := s.ClientStore()

	err = clientStore.UpdateCredential("test-cloud", jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"test-credentials": jujucloud.NewCredential(jujucloud.EmptyAuthType, map[string]string{
				"foo": "bar",
			}),
		},
	})
	c.Assert(err, gc.IsNil)

	cmdContext, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient), clientID, "test-cloud", "test-credentials")
	c.Assert(err, gc.IsNil)
	c.Assert(cmdtesting.Stdout(cmdContext), gc.Equals, `results:
- credentialtag: cloudcred-test-cloud_abda51b2-d735-4794-a8bd-49c506baa4af@serviceaccount_test-credentials
  error: null
  models: []
`)

	ofgaUser := openfga.NewUser(sa, s.JIMM.AuthorizationClient())
	cloudCredentialTag := names.NewCloudCredentialTag("test-cloud/" + clientIDWithDomain + "/test-credentials")
	cloudCredential2, err := s.JIMM.GetCloudCredential(ctx, ofgaUser, cloudCredentialTag)
	c.Assert(err, gc.IsNil)
	attrs, _, err := s.JIMM.GetCloudCredentialAttributes(ctx, ofgaUser, cloudCredential2, true)
	c.Assert(err, gc.IsNil)

	c.Assert(attrs, gc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}

func (s *updateCredentialsSuite) TestCloudNotInLocalStore(c *gc.C) {
	bClient := jimmtest.NewUserSessionLogin(c, "alice")
	_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(s.ClientStore(), bClient),
		"00000000-0000-0000-0000-000000000000",
		"non-existing-cloud",
		"foo",
	)
	c.Assert(err, gc.ErrorMatches, "failed to fetch local credentials for cloud \"non-existing-cloud\"")
}

func (s *updateCredentialsSuite) TestCredentialNotInLocalStore(c *gc.C) {
	bClient := jimmtest.NewUserSessionLogin(c, "alice")

	clientStore := s.ClientStore()
	err := clientStore.UpdateCredential("some-cloud", jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"some-credentials": jujucloud.NewCredential(jujucloud.EmptyAuthType, nil),
		},
	})
	c.Assert(err, gc.IsNil)

	_, err = cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient),
		"00000000-0000-0000-0000-000000000000",
		"some-cloud",
		"non-existing-credential-name",
	)
	c.Assert(err, gc.ErrorMatches, "credential \"non-existing-credential-name\" not found on local client.*")
}

func (s *updateCredentialsSuite) TestMissingArgs(c *gc.C) {
	tests := []struct {
		name          string
		args          []string
		expectedError string
	}{{
		name:          "missing client ID",
		args:          []string{},
		expectedError: "client ID not specified",
	}, {
		name:          "missing cloud",
		args:          []string{"some-client-id"},
		expectedError: "cloud not specified",
	}, {
		name:          "missing credential name",
		args:          []string{"some-client-id", "some-cloud"},
		expectedError: "credential name not specified",
	}, {
		name:          "too many args",
		args:          []string{"some-client-id", "some-cloud", "some-credential-name", "extra-arg"},
		expectedError: "too many args",
	}}

	bClient := jimmtest.NewUserSessionLogin(c, "alice")
	clientStore := s.ClientStore()
	for _, t := range tests {
		_, err := cmdtesting.RunCommand(c, cmd.NewUpdateCredentialsCommandForTesting(clientStore, bClient), t.args...)
		c.Assert(err, gc.ErrorMatches, t.expectedError, gc.Commentf("test case failed: %q", t.name))
	}
}