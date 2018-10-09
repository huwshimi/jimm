// Copyright 2015 Canonical Ltd.

package jem

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	cloudapi "github.com/juju/juju/api/cloud"
	"github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/utils/cache"
	"github.com/juju/utils/clock"
	"github.com/juju/version"
	"github.com/rogpeppe/fastuuid"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	usageauth "github.com/CanonicalLtd/jimm/internal/usagesender/auth"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

// wallClock provides access to the current time. It is a variable so
// that it can be overridden in tests.
var wallClock clock.Clock = clock.WallClock

// Functions defined as variables so they can be overridden in tests.
var (
	randIntn = rand.Intn

	NewUsageSenderAuthorizationClient = func(url string, client *httpbakery.Client) (UsageSenderAuthorizationClient, error) {
		return usageauth.NewAuthorizationClient(url, client), nil
	}
)

// UsageSenderAuthorizationClient is used to obtain authorization to
// collect and report usage metrics.
type UsageSenderAuthorizationClient interface {
	GetCredentials(ctx context.Context, applicationUser string) ([]byte, error)
}

// Params holds parameters for the NewPool function.
type Params struct {
	// DB holds the mongo database that will be used to
	// store the JEM information.
	DB *mgo.Database

	// SessionPool holds a pool from which session objects are
	// taken to be used in database operations.
	SessionPool *mgosession.Pool

	// ControllerAdmin holds the identity of the user
	// or group that is allowed to create controllers.
	ControllerAdmin params.User

	// UsageSenderURL holds the URL where we obtain authorization
	// to collect and report usage metrics.
	UsageSenderURL string

	// Client is used to make the request for usage metrics authorization
	Client *httpbakery.Client
}

type Pool struct {
	config    Params
	connCache *apiconn.Cache

	// dbName holds the name of the database to use.
	dbName string

	// regionCache caches region information about models
	regionCache *cache.Cache

	// mu guards the fields below it.
	mu sync.Mutex

	// closed holds whether the Pool has been closed.
	closed bool

	// refCount holds the number of JEM instances that
	// currently refer to the pool. The pool is finally
	// closed when all JEM instances are closed and the
	// pool itself has been closed.
	refCount int

	usageSenderAuthorizationClient UsageSenderAuthorizationClient

	// uuidGenerator is used to generate temporary UUIDs during the
	// creation of models, these UUIDs will be replaced with the ones
	// generated by the controllers themselves.
	uuidGenerator *fastuuid.Generator
}

var APIOpenTimeout = 15 * time.Second

var notExistsQuery = bson.D{{"$exists", false}}

// NewPool represents a pool of possible JEM instances that use the given
// database as a store, and use the given bakery parameters to create the
// bakery.Service.
func NewPool(ctx context.Context, p Params) (*Pool, error) {
	// TODO migrate database
	if p.ControllerAdmin == "" {
		return nil, errgo.Newf("no controller admin group specified")
	}
	if p.SessionPool == nil {
		return nil, errgo.Newf("no session pool provided")
	}
	uuidGen, err := fastuuid.NewGenerator()
	if err != nil {
		return nil, errgo.Mask(err)
	}
	pool := &Pool{
		config:        p,
		dbName:        p.DB.Name,
		connCache:     apiconn.NewCache(apiconn.CacheParams{}),
		regionCache:   cache.New(24 * time.Hour),
		refCount:      1,
		uuidGenerator: uuidGen,
	}
	if pool.config.UsageSenderURL != "" {
		client, err := NewUsageSenderAuthorizationClient(p.UsageSenderURL, p.Client)
		if err != nil {
			return nil, errgo.Notef(err, "cannot make omnibus authorization client")
		}
		pool.usageSenderAuthorizationClient = client
	}
	jem := pool.JEM(ctx)
	defer jem.Close()
	if err := jem.DB.ensureIndexes(); err != nil {
		return nil, errgo.Notef(err, "cannot ensure indexes")
	}
	return pool, nil
}

// Close closes the pool. Its resources will be freed
// when the last JEM instance created from the pool has
// been closed.
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.decRef()
	p.closed = true
}

func (p *Pool) decRef() {
	// called with p.mu held.
	if p.refCount--; p.refCount == 0 {
		p.connCache.Close()
	}
	if p.refCount < 0 {
		panic("negative reference count")
	}
}

// ClearAPIConnCache clears out the API connection cache.
// This is useful for testing purposes.
func (p *Pool) ClearAPIConnCache() {
	p.connCache.EvictAll()
}

// JEM returns a new JEM instance from the pool, suitable
// for using in short-lived requests. The JEM must be
// closed with the Close method after use.
//
// This method will panic if called after the pool has been
// closed.
func (p *Pool) JEM(ctx context.Context) *JEM {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		panic("JEM call on closed pool")
	}
	p.refCount++
	return &JEM{
		DB:                             newDatabase(ctx, p.config.SessionPool, p.dbName),
		pool:                           p,
		usageSenderAuthorizationClient: p.usageSenderAuthorizationClient,
	}
}

// UsageAuthorizationClient returns the UsageSenderAuthorizationClient.
func (p *Pool) UsageAuthorizationClient() UsageSenderAuthorizationClient {
	return p.usageSenderAuthorizationClient
}

type JEM struct {
	// DB holds the mongodb-backed identity store.
	DB *Database

	// pool holds the Pool from which the JEM instance
	// was created.
	pool *Pool

	// closed records whether the JEM instance has
	// been closed.
	closed bool

	usageSenderAuthorizationClient UsageSenderAuthorizationClient
}

// Clone returns an independent copy of the receiver
// that uses a cloned database connection. The
// returned value must be closed after use.
func (j *JEM) Clone() *JEM {
	j.pool.mu.Lock()
	defer j.pool.mu.Unlock()

	j.pool.refCount++
	return &JEM{
		DB:   j.DB.clone(),
		pool: j.pool,
	}
}

func (j *JEM) ControllerAdmin() params.User {
	return j.pool.config.ControllerAdmin
}

// Close closes the JEM instance. This should be called when
// the JEM instance is finished with.
func (j *JEM) Close() {
	j.pool.mu.Lock()
	defer j.pool.mu.Unlock()
	if j.closed {
		return
	}
	j.closed = true
	j.DB.Session.Close()
	j.DB = nil
	j.pool.decRef()
}

// ErrAPIConnection is returned by OpenAPI, OpenAPIFromDoc and
// OpenModelAPI when the API connection cannot be made.
//
// Note that it is defined as an ErrorCode so that Database.checkError
// does not treat it as a mongo-connection-broken error.
var ErrAPIConnection params.ErrorCode = "cannot connect to API"

// OpenAPI opens an API connection to the controller with the given path
// and returns it along with the information used to connect. If the
// controller does not exist, the error will have a cause of
// params.ErrNotFound.
//
// If the controller API connection could not be made, the error will
// have a cause of ErrAPIConnection.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPI(ctx context.Context, path params.EntityPath) (_ *apiconn.Conn, err error) {
	defer j.DB.checkError(ctx, &err)
	ctl, err := j.DB.Controller(ctx, path)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot get controller", errgo.Is(params.ErrNotFound))
	}
	return j.OpenAPIFromDoc(ctx, ctl)
}

// OpenAPIFromDoc returns an API connection to the controller held in the
// given document. This can be useful when we want to connect to a
// controller before it's added to the database. Note that a successful
// return from this function does not necessarily mean that the
// credentials or API addresses in the docs actually work, as it's
// possible that there's already a cached connection for the given
// controller.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenAPIFromDoc(ctx context.Context, ctl *mongodoc.Controller) (*apiconn.Conn, error) {
	return j.pool.connCache.OpenAPI(ctx, ctl.UUID, func() (api.Connection, *api.Info, error) {
		info := apiInfoFromDoc(ctl)
		zapctx.Debug(ctx, "open API", zap.Any("api-info", info))
		conn, err := api.Open(info, apiDialOpts())
		if err != nil {
			return nil, nil, errgo.WithCausef(err, ErrAPIConnection, "")
		}
		return conn, info, nil
	})
}

func apiDialOpts() api.DialOpts {
	return api.DialOpts{
		Timeout:    APIOpenTimeout,
		RetryDelay: 500 * time.Millisecond,
	}
}

func apiInfoFromDoc(ctl *mongodoc.Controller) *api.Info {
	return &api.Info{
		Addrs:    mongodoc.Addresses(ctl.HostPorts),
		CACert:   ctl.CACert,
		Tag:      names.NewUserTag(ctl.AdminUser),
		Password: ctl.AdminPassword,
	}
}

// OpenModelAPI opens an API connection to the model with the given path
// and returns it along with the information used to connect. If the
// model does not exist, the error will have a cause of
// params.ErrNotFound.
//
// If the model API connection could not be made, the error will have a
// cause of ErrAPIConnection.
//
// The returned connection must be closed when finished with.
func (j *JEM) OpenModelAPI(ctx context.Context, path params.EntityPath) (_ *apiconn.Conn, err error) {
	defer j.DB.checkError(ctx, &err)
	m, err := j.DB.Model(ctx, path)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot get model", errgo.Is(params.ErrNotFound))
	}
	ctl, err := j.DB.Controller(ctx, m.Controller)
	if err != nil {
		return nil, errgo.Notef(err, "cannot get controller")
	}
	return j.openModelAPIFromDocs(ctx, ctl, m)
}

// openModelAPIFromDocs returns an API connection to the model held in the
// given documents.
//
// The returned connection must be closed when finished with.
func (j *JEM) openModelAPIFromDocs(ctx context.Context, ctl *mongodoc.Controller, m *mongodoc.Model) (*apiconn.Conn, error) {
	return j.pool.connCache.OpenAPI(ctx, m.UUID, func() (api.Connection, *api.Info, error) {
		info := apiInfoFromDocs(ctl, m)
		zapctx.Debug(ctx, "open API", zap.Any("api-info", info))
		conn, err := api.Open(info, apiDialOpts())
		if err != nil {
			zapctx.Info(ctx, "failed to open connection", zaputil.Error(err), zap.Any("api-info", info))
			return nil, nil, errgo.WithCausef(err, ErrAPIConnection, "")
		}
		return conn, info, nil
	})
}

func apiInfoFromDocs(ctl *mongodoc.Controller, m *mongodoc.Model) *api.Info {
	return &api.Info{
		Addrs:    mongodoc.Addresses(ctl.HostPorts),
		CACert:   ctl.CACert,
		ModelTag: names.NewModelTag(m.UUID),
		Tag:      names.NewUserTag(ctl.AdminUser),
		Password: ctl.AdminPassword,
	}
}

// Controller retrieves the given controller from the database,
// validating that the current user is allowed to read the controller.
func (j *JEM) Controller(ctx context.Context, path params.EntityPath) (*mongodoc.Controller, error) {
	if err := j.DB.CheckReadACL(ctx, j.DB.Controllers(), path); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	ctl, err := j.DB.Controller(ctx, path)
	return ctl, errgo.Mask(err, errgo.Is(params.ErrNotFound))
}

// Credential retrieves the given credential from the database,
// validating that the current user is allowed to read the credential.
func (j *JEM) Credential(ctx context.Context, path params.CredentialPath) (*mongodoc.Credential, error) {
	cred, err := j.DB.Credential(ctx, path)
	if err != nil {
		if errgo.Cause(err) == params.ErrNotFound {
			// We return an authorization error for all attempts to retrieve credentials
			// from any other user's space.
			if aerr := auth.CheckIsUser(ctx, path.User); aerr != nil {
				err = aerr
			}
		}
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := auth.CheckCanRead(ctx, cred); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	return cred, nil
}

// CreateModelParams specifies the parameters needed to create a new
// model using CreateModel.
type CreateModelParams struct {
	// Path contains the path of the new model.
	Path params.EntityPath

	// ControllerPath contains the path of the owning
	// controller.
	ControllerPath params.EntityPath

	// Credential contains the name of the credential to use to
	// create the model.
	Credential params.CredentialPath

	// Cloud contains the name of the cloud in which the
	// model will be created.
	Cloud params.Cloud

	// Region contains the name of the region in which the model will
	// be created. This may be empty if the cloud does not support
	// regions.
	Region string

	// Attributes contains the attributes to assign to the new model.
	Attributes map[string]interface{}
}

// CreateModel creates a new model as specified by p.
func (j *JEM) CreateModel(ctx context.Context, p CreateModelParams) (_ *mongodoc.Model, err error) {
	// Only the owner can create a new model in their namespace.
	if err := auth.CheckIsUser(ctx, p.Path.User); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}

	var usageSenderCredentials []byte
	if j.usageSenderAuthorizationClient != nil {
		usageSenderCredentials, err = j.usageSenderAuthorizationClient.GetCredentials(
			ctx,
			string(p.Path.User))
		if err != nil {
			return nil, errgo.Mask(err)
		}
	}

	var cred *mongodoc.Credential
	cred, err = j.selectCredential(ctx, p.Credential, p.Path.User, p.Cloud)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrAmbiguousChoice))
	}

	controllers, err := j.possibleControllers(ctx, p.ControllerPath, p.Cloud, p.Region)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}

	// Create the model record in the database before actually
	// creating the model on the controller. It will have an invalid
	// UUID because it doesn't exist but that's better than creating
	// a model that we can't add locally because the name
	// already exists.
	modelDoc := &mongodoc.Model{
		Path:                   p.Path,
		CreationTime:           wallClock.Now(),
		Creator:                auth.Username(ctx),
		UsageSenderCredentials: usageSenderCredentials,
		// Use a temporary UUID so that we can create two at the
		// same time, because the uuid field must always be
		// unique.
		UUID: fmt.Sprintf("creating-%x", j.pool.uuidGenerator.Next()),
	}
	if cred != nil {
		modelDoc.Credential = cred.Path
	}
	if err := j.DB.AddModel(ctx, modelDoc); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrAlreadyExists))
	}

	defer func() {
		if err == nil {
			return
		}

		// We're returning an error, so remove the model from the
		// database. Note that this might leave the model around
		// in the controller, but this should be rare and we can
		// deal with it at model creation time later (see TODO below).
		if err := j.DB.DeleteModel(ctx, modelDoc.Path); err != nil {
			zapctx.Error(ctx, "cannot remove model from database after error; leaked model", zaputil.Error(err))
		}
	}()

	var ctlPath params.EntityPath
	var modelInfo base.ModelInfo
	for _, controller := range controllers {
		var err error
		modelInfo, err = j.createModelOnController(ctx, controller, p, cred)
		if err == nil {
			ctlPath = controller
			break
		}
		if errgo.Cause(err) == errInvalidModelParams {
			return nil, errgo.Notef(err, "cannot create model")
		}
		zapctx.Error(ctx, "cannot create model on controller", zaputil.Error(err), zap.String("controller", controller.String()))
	}

	if ctlPath.Name == "" {
		return nil, errgo.New("cannot find suitable controller")
	}

	// Now set the UUID to that of the actually created model,
	// and update other attributes from the response too.
	// Use Apply so that we can return a result that's consistent
	// with Database.Model.
	info := mongodoc.ModelInfo{
		Life: string(modelInfo.Life),
		Status: mongodoc.ModelStatus{
			Status:  string(modelInfo.Status.Status),
			Message: modelInfo.Status.Info,
			Data:    modelInfo.Status.Data,
		},
	}
	if modelInfo.Status.Since != nil {
		info.Status.Since = *modelInfo.Status.Since
	}
	if modelInfo.AgentVersion != nil {
		info.Config = map[string]interface{}{
			config.AgentVersionKey: modelInfo.AgentVersion.String(),
		}
	}
	if _, err := j.DB.Models().FindId(modelDoc.Id).Apply(mgo.Change{
		Update: bson.D{{"$set", bson.D{
			{"uuid", modelInfo.UUID},
			{"controller", ctlPath},
			{"cloud", modelInfo.Cloud},
			{"cloudregion", modelInfo.CloudRegion},
			{"defaultseries", modelInfo.DefaultSeries},
			{"info", info},
			{"type", modelInfo.Type},
			{"providertype", modelInfo.ProviderType},
		}}},
		ReturnNew: true,
	}, &modelDoc); err != nil {
		j.DB.checkError(ctx, &err)
		return nil, errgo.Notef(err, "cannot update model %s in database", modelInfo.UUID)
	}

	if err := j.DB.AppendAudit(ctx, params.AuditModelCreated{
		ID:             modelDoc.Id,
		UUID:           modelInfo.UUID,
		Owner:          string(modelDoc.Owner()),
		Creator:        modelDoc.Creator,
		ControllerPath: ctlPath.String(),
		Cloud:          string(modelDoc.Cloud),
		Region:         modelDoc.CloudRegion,
		AuditEntryCommon: params.AuditEntryCommon{
			Type_:    params.AuditLogType(params.AuditModelCreated{}),
			Created_: time.Now(),
		},
	}); err != nil {
		zapctx.Error(ctx, "cannot add audit log for model creation", zaputil.Error(err))
	}

	return modelDoc, nil
}

func (j *JEM) possibleControllers(ctx context.Context, ctlPath params.EntityPath, cloud params.Cloud, region string) ([]params.EntityPath, error) {
	if ctlPath.Name != "" {
		return []params.EntityPath{ctlPath}, nil
	}
	cloudRegion, err := j.DB.CloudRegion(ctx, cloud, region)
	if err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	if err := auth.CheckCanRead(ctx, cloudRegion); err != nil {
		return nil, errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	controllers := cloudRegion.PrimaryControllers
	if len(controllers) == 0 {
		controllers = cloudRegion.SecondaryControllers
	}
	shuffle(len(controllers), func(i, j int) { controllers[i], controllers[j] = controllers[j], controllers[i] })
	return controllers, nil
}

// shuffle is used to randomize the order in which possible controllers
// are tried. It is a variable so it can be replaced in tests.
var shuffle func(int, func(int, int)) = rand.Shuffle

const errInvalidModelParams params.ErrorCode = "invalid CreateModel request"

func (j *JEM) createModelOnController(ctx context.Context, ctlPath params.EntityPath, p CreateModelParams, cred *mongodoc.Credential) (base.ModelInfo, error) {
	ctl, err := j.Controller(ctx, ctlPath)
	if err != nil {
		return base.ModelInfo{}, errgo.Notef(err, "cannot get controller document")
	}
	if ctl.Deprecated {
		return base.ModelInfo{}, errgo.Notef(err, "controller deprecated")
	}
	conn, err := j.OpenAPIFromDoc(ctx, ctl)
	if err != nil {
		return base.ModelInfo{}, errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()

	var credTag names.CloudCredentialTag
	if cred != nil {
		if err := j.updateControllerCredential(ctx, ctlPath, cred.Path, conn, cred); err != nil {
			return base.ModelInfo{}, errgo.Notef(err, "cannot add credential")
		}
		if err := j.DB.credentialAddController(ctx, cred.Path, ctlPath); err != nil {
			return base.ModelInfo{}, errgo.Notef(err, "cannot add credential")
		}
		credTag = CloudCredentialTag(cred.Path)
	}

	mmClient := modelmanager.NewClient(conn.Connection)
	m, err := mmClient.CreateModel(
		string(p.Path.Name),
		UserTag(p.Path.User).Id(),
		string(p.Cloud),
		p.Region,
		credTag,
		p.Attributes,
	)
	if err != nil {
		switch jujuparams.ErrCode(err) {
		case jujuparams.CodeAlreadyExists:
			// The model already exists in the controller but it didn't
			// exist in the database. This probably means that it's
			// been abortively created previously, but left around because
			// of connection failure.
			// TODO initiate cleanup of the model, first checking that
			// it's empty, but return an error to the user because
			// the operation to delete a model isn't synchronous even
			// for empty models. We could also have a worker that deletes
			// empty models that don't appear in the database.
			return base.ModelInfo{}, errgo.Notef(err, "model name in use")
		case jujuparams.CodeUpgradeInProgress:
			return base.ModelInfo{}, errgo.Notef(err, "upgrade in progress")
		default:
			// The model couldn't be created because of an
			// error in the request, don't try another
			// controller.
			return base.ModelInfo{}, errgo.WithCausef(err, errInvalidModelParams, "")
		}
	}
	// TODO should we try to delete the model from the controller
	// on error here?

	// Grant JIMM admin access to the model. Note that if this fails,
	// the local database entry will be deleted but the model
	// will remain on the controller and will trigger the "already exists
	// in the backend controller" message above when the user
	// attempts to create a model with the same name again.
	if err := mmClient.GrantModel(conn.Info.Tag.(names.UserTag).Id(), "admin", m.UUID); err != nil {
		// TODO (mhilton) ensure that this is flagged in some admin interface somewhere.
		zapctx.Error(ctx, "leaked model", zap.String("controller", ctlPath.String()), zap.String("model", p.Path.String()), zaputil.Error(err), zap.String("model-uuid", m.UUID))
		return base.ModelInfo{}, errgo.Notef(err, "cannot grant model access")
	}
	return m, nil
}

// UpdateCredential updates the specified credential in the
// local database and then updates it on all controllers to which it is
// deployed.
func (j *JEM) UpdateCredential(ctx context.Context, cred *mongodoc.Credential) (err error) {
	if err := j.DB.updateCredential(ctx, cred); err != nil {
		return errgo.Notef(err, "cannot update local database")
	}
	c, err := j.DB.Credential(ctx, cred.Path)
	if err != nil {
		return errgo.Mask(err)
	}
	// Mark in the local database that an update is required for all controllers
	if err := j.DB.setCredentialUpdates(ctx, cred.Controllers, cred.Path); err != nil {
		// Log the error, but press on hoping to update the controllers anyway.
		zapctx.Error(ctx,
			"cannot update controllers with updated credential",
			zap.String("cred", c.Path.String()),
			zaputil.Error(err),
		)
	}
	// Attempt to update all controllers to which the credential is
	// deployed. If these fail they will be updated by the monitor.
	n := len(c.Controllers)
	// Make the channel buffered so we don't leak go-routines
	ch := make(chan struct{}, n)
	for _, ctlPath := range c.Controllers {
		go func(j *JEM, ctlPath params.EntityPath) {
			defer func() {
				ch <- struct{}{}
			}()
			defer j.Close()
			if err := j.updateControllerCredential(ctx, ctlPath, cred.Path, nil, c); err != nil {
				zapctx.Warn(ctx,
					"cannot update credential",
					zap.String("cred", c.Path.String()),
					zap.String("controller", ctlPath.String()),
					zaputil.Error(err),
				)
				return
			}
		}(j.Clone(), ctlPath)
	}
	// Only wait for as along as the context allows for the updates to finish.
	for n > 0 {
		select {
		case <-ch:
			n--
		case <-ctx.Done():
		}
	}
	return nil
}

// ControllerUpdateCredentials updates the given controller by updating
// all outstanding UpdateCredentials.
func (j *JEM) ControllerUpdateCredentials(ctx context.Context, ctlPath params.EntityPath) error {
	ctl, err := j.DB.Controller(ctx, ctlPath)
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	conn, err := j.OpenAPIFromDoc(ctx, ctl)
	if err != nil {
		return errgo.Notef(err, "cannot connect to controller")
	}
	defer conn.Close()
	for _, credPath := range ctl.UpdateCredentials {
		if err := j.updateControllerCredential(ctx, ctl.Path, credPath, conn, nil); err != nil {
			zapctx.Warn(ctx,
				"cannot update credential",
				zap.Stringer("cred", credPath),
				zap.Stringer("controller", ctl.Path),
				zaputil.Error(err),
			)
		}
	}
	return nil
}

// updateControllerCredential updates the given credential on the given
// controller. If conn is non-nil then it will be used to communicate
// with the controller. If cred is non-nil then those credentials will be
// updated on the controller.
func (j *JEM) updateControllerCredential(
	ctx context.Context,
	ctlPath params.EntityPath,
	credPath params.CredentialPath,
	conn *apiconn.Conn,
	cred *mongodoc.Credential,
) error {
	var err error
	if conn == nil {
		conn, err = j.OpenAPI(ctx, ctlPath)
		if err != nil {
			return errgo.Mask(err)
		}
		defer conn.Close()
	}
	if cred == nil {
		cred, err = j.DB.Credential(ctx, credPath)
		if err != nil {
			return errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
	}
	cloudCredentialTag := CloudCredentialTag(credPath)
	cloudClient := cloudapi.NewClient(conn)
	if cred.Revoked {
		err = cloudClient.RevokeCredential(cloudCredentialTag)
	} else {
		err = cloudClient.UpdateCredential(
			cloudCredentialTag,
			jujucloud.NewCredential(jujucloud.AuthType(cred.Type), cred.Attributes),
		)
	}
	if err != nil {
		return errgo.Notef(err, "cannot update credentials")
	}
	if err := j.DB.clearCredentialUpdate(ctx, ctlPath, credPath); err != nil {
		zapctx.Error(ctx,
			"failed to update controller after successfully updating credential",
			zap.Stringer("cred", credPath),
			zap.Stringer("controller", ctlPath),
			zaputil.Error(err),
		)
	}
	return nil
}

// GrantModel grants the given access for the given user on the given model and updates the JEM database.
func (j *JEM) GrantModel(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model, user params.User, access string) error {
	if err := j.DB.GrantModel(ctx, model.Path, user, access); err != nil {
		return errgo.Mask(err)
	}
	client := modelmanager.NewClient(conn)
	if err := client.GrantModel(UserTag(user).Id(), access, model.UUID); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// RevokeModel revokes the given access for the given user on the given model and updates the JEM database.
func (j *JEM) RevokeModel(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model, user params.User, access string) error {
	if err := j.DB.RevokeModel(ctx, model.Path, user, access); err != nil {
		return errgo.Mask(err)
	}
	client := modelmanager.NewClient(conn)
	if err := client.RevokeModel(UserTag(user).Id(), access, model.UUID); err != nil {
		// TODO (mhilton) What should be done with the changes already made to JEM.
		return errgo.Mask(err)
	}
	return nil
}

// DestroyModel destroys the specified model. The model will have its
// Life set to dying, but won't be removed until it is removed from the
// controller.
func (j *JEM) DestroyModel(ctx context.Context, conn *apiconn.Conn, model *mongodoc.Model, destroyStorage *bool) error {
	client := modelmanager.NewClient(conn)
	if err := client.DestroyModel(names.NewModelTag(model.UUID), destroyStorage); err != nil {
		return errgo.Mask(err, jujuparams.IsCodeHasPersistentStorage)
	}
	if err := j.DB.SetModelLife(ctx, model.Controller, model.UUID, "dying"); err != nil {
		// If this update fails then don't worry as the watcher
		// will detect the state change and update as appropriate.
		zapctx.Warn(ctx, "error updating model life", zap.Error(err), zap.String("model", model.UUID))
	}
	if err := j.DB.AppendAudit(ctx, params.AuditModelDestroyed{
		ID:   model.Id,
		UUID: model.UUID,
		AuditEntryCommon: params.AuditEntryCommon{
			Type_:    params.AuditLogType(params.AuditModelDestroyed{}),
			Created_: time.Now(),
		},
	}); err != nil {
		zapctx.Error(ctx, "cannot add audit log for model destruction", zaputil.Error(err))
	}
	return nil
}

// EarliestControllerVersion returns the earliest agent version
// that any of the available public controllers is known to be running.
// If there are no available controllers or none of their versions are
// known, it returns the zero version.
func (j *JEM) EarliestControllerVersion(ctx context.Context) (version.Number, error) {
	// TOD(rog) cache the result of this for a while, as it changes only rarely
	// and we don't really need to make this extra round trip every
	// time a user connects to the API?
	var v *version.Number
	if err := j.DoControllers(ctx, func(c *mongodoc.Controller) error {
		zapctx.Debug(ctx, "in EarliestControllerVersion", zap.Stringer("controller", c.Path), zap.Stringer("version", c.Version))
		if c.Version == nil {
			return nil
		}
		if v == nil || c.Version.Compare(*v) < 0 {
			v = c.Version
		}
		return nil
	}); err != nil {
		return version.Number{}, errgo.Mask(err)
	}
	if v == nil {
		return version.Number{}, nil
	}
	return *v, nil
}

// DoControllers calls the given function for each controller that
// can be read by the current user that matches the given attributes.
// If the function returns an error, the iteration stops and
// DoControllers returns the error with the same cause.
//
// Note that the same pointer is passed to the do function on
// each iteration. It is the responsibility of the do function to
// copy it if needed.
func (j *JEM) DoControllers(ctx context.Context, do func(c *mongodoc.Controller) error) error {
	// Query all the controllers that match the attributes, building
	// up all the possible values.
	q := j.DB.Controllers().Find(bson.D{{"unavailablesince", notExistsQuery}, {"public", true}})
	// Sort by _id so that we can make easily reproducible tests.
	iter := j.DB.NewCanReadIter(ctx, q.Sort("_id").Iter())
	var ctl mongodoc.Controller
	for iter.Next(&ctl) {
		if err := do(&ctl); err != nil {
			iter.Close()
			return errgo.Mask(err, errgo.Any)
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot query")
	}
	return nil
}

// selectCredential chooses a credential appropriate for the given user that can
// be used when starting a model in the given cloud.
//
// If there's more than one such credential, it returns a params.ErrAmbiguousChoice error.
//
// If there are no credentials found, a zero credential path is returned.
func (j *JEM) selectCredential(ctx context.Context, path params.CredentialPath, user params.User, cloud params.Cloud) (*mongodoc.Credential, error) {
	query := bson.D{{"path", path}}
	if path.IsZero() {
		query = bson.D{
			{"path.entitypath.user", user},
			{"path.cloud", cloud},
		}
	}
	var creds []mongodoc.Credential
	iter := j.DB.NewCanReadIter(ctx, j.DB.Credentials().Find(query).Iter())
	var cred mongodoc.Credential
	for iter.Next(&cred) {
		creds = append(creds, cred)
	}
	if err := iter.Err(); err != nil {
		return nil, errgo.Notef(err, "cannot query credentials")
	}
	switch len(creds) {
	case 0:
		var err error
		if !path.IsZero() {
			err = errgo.WithCausef(nil, params.ErrNotFound, "credential %q not found", path)
		}
		return nil, err
	case 1:
		return &creds[0], nil
	default:
		return nil, errgo.WithCausef(nil, params.ErrAmbiguousChoice, "more than one possible credential to use")
	}
}

// selectRandomController chooses a random controller that you have access to.
func (j *JEM) selectRandomController(ctx context.Context) (params.EntityPath, error) {
	// Choose a random controller.
	// TODO select a controller more intelligently, for example
	// by choosing the most lightly loaded controller
	var controllers []mongodoc.Controller
	if err := j.DoControllers(ctx, func(c *mongodoc.Controller) error {
		controllers = append(controllers, *c)
		return nil
	}); err != nil {
		return params.EntityPath{}, errgo.Mask(err)
	}
	if len(controllers) == 0 {
		return params.EntityPath{}, errgo.Newf("cannot find a suitable controller")
	}
	n := randIntn(len(controllers))
	return controllers[n].Path, nil
}

// UpdateMachineInfo updates the information associated with a machine.
func (j *JEM) UpdateMachineInfo(ctx context.Context, ctlPath params.EntityPath, info *multiwatcher.MachineInfo) error {
	cloud, region, err := j.modelRegion(ctx, ctlPath, info.ModelUUID)
	if errgo.Cause(err) == params.ErrNotFound {
		// If the model isn't found then it is not controlled by
		// JIMM and we aren't interested in it.
		return nil
	}
	if err != nil {
		return errgo.Notef(err, "cannot find region for model %s:%s", ctlPath, info.ModelUUID)
	}
	return errgo.Mask(j.DB.UpdateMachineInfo(ctx, &mongodoc.Machine{
		Controller: ctlPath.String(),
		Cloud:      cloud,
		Region:     region,
		Info:       info,
	}))
}

// UpdateApplicationInfo updates the information associated with an application.
func (j *JEM) UpdateApplicationInfo(ctx context.Context, ctlPath params.EntityPath, info *multiwatcher.ApplicationInfo) error {
	cloud, region, err := j.modelRegion(ctx, ctlPath, info.ModelUUID)
	if errgo.Cause(err) == params.ErrNotFound {
		// If the model isn't found then it is not controlled by
		// JIMM and we aren't interested in it.
		return nil
	}
	if err != nil {
		return errgo.Notef(err, "cannot find region for model %s:%s", ctlPath, info.ModelUUID)
	}
	app := &mongodoc.Application{
		Controller: ctlPath.String(),
		Cloud:      cloud,
		Region:     region,
	}
	if info != nil {
		app.Info = &mongodoc.ApplicationInfo{
			ModelUUID:       info.ModelUUID,
			Name:            info.Name,
			Exposed:         info.Exposed,
			CharmURL:        info.CharmURL,
			OwnerTag:        info.OwnerTag,
			Life:            info.Life,
			Subordinate:     info.Subordinate,
			Status:          info.Status,
			WorkloadVersion: info.WorkloadVersion,
		}
	}
	return errgo.Mask(j.DB.UpdateApplicationInfo(ctx, app))
}

// modelRegion determines the cloud and region in which a model is contained.
func (j *JEM) modelRegion(ctx context.Context, ctlPath params.EntityPath, uuid string) (params.Cloud, string, error) {
	type cloudRegion struct {
		cloud  params.Cloud
		region string
	}
	key := fmt.Sprintf("%s %s", ctlPath, uuid)
	r, err := j.pool.regionCache.Get(key, func() (interface{}, error) {
		m, err := j.DB.modelFromControllerAndUUID(ctx, ctlPath, uuid)
		if err != nil {
			return nil, errgo.Mask(err, errgo.Is(params.ErrNotFound))
		}
		return cloudRegion{
			cloud:  m.Cloud,
			region: m.CloudRegion,
		}, nil
	})
	if err != nil {
		return "", "", errgo.Mask(err, errgo.Is(params.ErrNotFound))
	}
	cr := r.(cloudRegion)
	return cr.cloud, cr.region, nil
}

// CreateCloud creates a new cloud in the database and adds it to a
// controller. The slice of regions must contain at least a region for
// the cloud as a whole (no region name) which must be first. If the
// cloud name already exists then an error with a cause of
// params.ErrAlreadyExists will be returned.
func (j *JEM) CreateCloud(ctx context.Context, cloud mongodoc.CloudRegion, regions []mongodoc.CloudRegion) error {
	// TODO(mhilton) check the cloud name isn't reserved.

	// Attempt to insert the document for the cloud and fail early if
	// such a cloud exists.
	if err := j.DB.InsertCloudRegion(ctx, &cloud); err != nil {
		if errgo.Cause(err) == params.ErrAlreadyExists {
			return errgo.WithCausef(nil, params.ErrAlreadyExists, "cloud %q already exists", cloud.Cloud)
		}
		return errgo.Mask(err)
	}
	jcloud := jujucloud.Cloud{
		Name:             string(cloud.Cloud),
		Type:             cloud.ProviderType,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		CACertificates:   cloud.CACertificates,
	}
	for _, authType := range cloud.AuthTypes {
		jcloud.AuthTypes = append(jcloud.AuthTypes, jujucloud.AuthType(authType))
	}
	for _, reg := range regions {
		jcloud.Regions = append(jcloud.Regions, jujucloud.Region{
			Name:             reg.Region,
			Endpoint:         reg.Endpoint,
			IdentityEndpoint: reg.IdentityEndpoint,
			StorageEndpoint:  reg.StorageEndpoint,
		})
	}
	ctlPath, err := j.createCloud(ctx, jcloud)
	if err != nil {
		if err := j.DB.RemoveCloudRegion(ctx, cloud.Cloud, ""); err != nil {
			zapctx.Warn(ctx, "cannot remove cloud that failed to deploy", zaputil.Error(err))
		}
		return errgo.Mask(err)
	}
	cloud.PrimaryControllers = []params.EntityPath{ctlPath}
	for i := range regions {
		regions[i].PrimaryControllers = []params.EntityPath{ctlPath}
	}
	return errgo.Mask(j.DB.UpdateCloudRegions(ctx, append(regions, cloud)))
}

func (j *JEM) createCloud(ctx context.Context, cloud jujucloud.Cloud) (params.EntityPath, error) {
	// Pick a random public controller.
	// TODO(mhilton) find a better way to choose a controller for the
	// cloud (presumably based on IP address magic).
	ctlPath, err := j.selectRandomController(ctx)
	if err != nil {
		return params.EntityPath{}, errgo.Mask(err)
	}
	conn, err := j.OpenAPI(ctx, ctlPath)
	if err != nil {
		// TODO(mhilton) if this controller fails try another?
		return params.EntityPath{}, errgo.Mask(err)
	}
	defer conn.Close()
	if err := cloudapi.NewClient(conn).AddCloud(cloud); err != nil {
		// TODO(mhilton) if this controller fails try another?
		return params.EntityPath{}, errgo.Mask(err)
	}
	return ctlPath, nil
}

// RemoveCloud removes the given cloud, so long as no models are using it.
func (j *JEM) RemoveCloud(ctx context.Context, cloud params.Cloud) (err error) {
	cr, err := j.DB.CloudRegion(ctx, cloud, "")
	if err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrNotFound), errgo.Is(params.ErrUnauthorized))
	}
	if err := auth.CheckACL(ctx, cr.ACL.Admin); err != nil {
		return errgo.Mask(err, errgo.Is(params.ErrUnauthorized))
	}
	// This check is technically redundant as we can't know whether
	// the cloud is in use by any models at the moment we remove it from a controller
	// (remember that only one of the primary controllers might be using it).
	// However we like the error message and it's usually going to be OK,
	// so we'll do the advance check anyway.
	if n, err := j.DB.Models().Find(bson.D{{"cloud", cloud}}).Count(); n > 0 || err != nil {
		if err != nil {
			return errgo.Mask(err)
		}
		return errgo.Newf("cloud is used by %d model%s", n, plural(n))
	}
	// TODO delete the cloud from the controllers in parallel
	// (although currently there is only ever one anyway).
	for _, ctl := range cr.PrimaryControllers {
		conn, err := j.OpenAPI(ctx, ctl)
		if err != nil {
			return errgo.Mask(err)
		}
		defer conn.Close()
		if err := cloudapi.NewClient(conn).RemoveCloud(string(cloud)); err != nil {
			return errgo.Notef(err, "cannot remove cloud from controller %s", ctl)
		}
	}
	if err := j.DB.RemoveCloud(ctx, cloud); err != nil {
		return errgo.Mask(err)
	}
	// TODO (mhilton) Audit cloud removals.
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// UserTag creates a juju user tag from a params.User
func UserTag(u params.User) names.UserTag {
	tag := names.NewUserTag(string(u))
	if tag.IsLocal() {
		tag = tag.WithDomain("external")
	}
	return tag
}

// CloudTag creates a juju cloud tag from a params.Cloud
func CloudTag(c params.Cloud) names.CloudTag {
	return names.NewCloudTag(string(c))
}

// CloudCredentialTag creates a juju cloud credential tag from the given
// CredentialPath.
func CloudCredentialTag(p params.CredentialPath) names.CloudCredentialTag {
	if p.IsZero() {
		return names.CloudCredentialTag{}
	}
	user := UserTag(p.User)
	return names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", p.Cloud, user.Id(), p.Name))
}
