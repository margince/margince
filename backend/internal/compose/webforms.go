// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The anonymous website-form edge: /v1/public/forms/{form_key} is posted to by
// a visitor's browser or a form builder's webhook, with no session. This
// middleware meters it, answers the browser's CORS preflight (the contract
// router has no OPTIONS route), and binds a system principal confined to the
// intake. The form's own admission rule, in the webforms module, decides
// whether a post is taken.

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const publicFormsPrefix = "/v1/public/forms/"

// newWebFormHandlers builds the forms surface over the real lead and activity
// writers. The secret store arrives later, through WithKeyvault.
func newWebFormHandlers(pool *pgxpool.Pool, log *slog.Logger) webforms.Handlers {
	store := webforms.NewStore(InstallationDB(pool), log)
	catalog := customfields.NewService(pool, nil)
	lander := webFormLeads{
		leads: newContactsStore(InstallationDB(pool)).WithFieldCatalog(catalog).
			WithLeadArrivals(newLeadWorkTasks(InstallationDB(pool), log)),
		activities: activities.NewStore(InstallationDB(pool)),
		catalog:    catalog,
	}
	return webforms.NewHandlers(store, webforms.NewIntake(store, lander), time.Now)
}

// publicFormLimiters brake the anonymous intake. Per IP stops one source
// filling the CRM. Per form is sized for a whole website's traffic, which
// every visitor and a builder's few sending addresses share.
type publicFormLimiters struct {
	perIP   *ratelimit.Limiter
	perForm *ratelimit.Limiter
}

func newPublicFormLimiters() publicFormLimiters {
	return publicFormLimiters{
		perIP:   ratelimit.New("public-forms/per-ip", ratelimit.FailClosed, 60, time.Minute),
		perForm: ratelimit.New("public-forms/per-form", ratelimit.FailClosed, 600, time.Minute),
	}
}

func publicForms(forms webforms.Handlers, limits publicFormLimiters) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, publicFormsPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cache-Control", "no-store")
			key := strings.TrimPrefix(r.URL.Path, publicFormsPrefix)
			// The key is checked before it meters anything: a made-up key
			// must not become a limiter key of its own.
			if !webforms.ValidFormKey(key) {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			if !limits.perIP.Allow(httpserver.ClientIP(r)) || !limits.perForm.Allow(key) {
				httperr.Write(w, r, apperrors.ErrBudgetExceeded)
				return
			}
			// The workspace is already bound: the identity middleware binds the
			// installation's into every request, public paths included.
			ctx := principal.WithActor(r.Context(), principal.Principal{
				Type: principal.PrincipalSystem,
				ID:   "system:public_web_form",
			})
			if r.Method == http.MethodOptions {
				forms.ServePreflight(w, r.WithContext(ctx), key)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
