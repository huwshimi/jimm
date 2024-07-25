// Copyright 2024 Canonical Ltd.

package rebac_admin

import (
	"net/http"

	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/jimm"
	rebac_handlers "github.com/canonical/rebac-admin-ui-handlers/v1"
)

func AuthenticateMiddleware(next http.Handler, jimm *jimm.JIMM) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := jimm.OAuthAuthenticator.AuthenticateBrowserSession(r.Context(), w, r)
		if err != nil {
			zapctx.Error(ctx, "failed to authenticate", zap.Error(err))
			http.Error(w, "failed to authenticate", http.StatusUnauthorized)
			return
		}

		identity := auth.SessionIdentityFromContext(ctx)
		if identity == "" {
			zapctx.Error(ctx, "no identity found in session")
			http.Error(w, "internal authentication error", http.StatusInternalServerError)
			return
		}

		user, err := jimm.GetOpenFGAUserAndAuthorise(ctx, identity)
		if err != nil {
			zapctx.Error(ctx, "failed to get openfga user", zap.Error(err))
			http.Error(w, "internal authentication error", http.StatusInternalServerError)
			return
		}

		ctx = rebac_handlers.ContextWithIdentity(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
