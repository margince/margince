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
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/inquiries"
	"github.com/margince/margince/backend/internal/compose/webformlead"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

const publicFormsPrefix = "/v1/public/forms/"

// newWebFormHandlers builds the forms surface over the real lead and activity
// writers. The secret store arrives later, through WithKeyvault.
func newWebFormHandlers(pool *pgxpool.Pool, log *slog.Logger) webforms.Handlers {
	db := InstallationDB(pool)
	store := webforms.NewStore(db, log).WithHandlingFence(newWebFormFence(db))
	store = store.WithInquirySettings(newInquirySettings(db, store))
	forms := webforms.NewHandlers(store, webforms.NewIntake(store, newWebFormLander(db, pool, log)), time.Now)
	return forms.WithThrottle(webforms.NewThrottle(webforms.DefaultThrottleLimits(), time.Now, log, forms.ListLiveForms, forms.LookupForm))
}

// newWebFormLander is the lead and activity writers a submission lands through.
func newWebFormLander(db *database.DB, pool *pgxpool.Pool, log *slog.Logger) webformlead.Lander {
	catalog := customfields.NewService(pool, nil)
	leads := newContactsStore(db).WithFieldCatalog(catalog).WithLeadArrivals(newLeadWorkTasks(db, log))
	return webformlead.New(leads, activities.NewStore(db), catalog, automation.NewAutomationStore(db), webFormFieldValue)
}

// webFormFieldValue reads a form field as the import reads a cell. The visitor
// sees no refusal, so the refusal's wording is dropped.
//
//craft:ignore naked-any the value is a lead CustomFields entry, shaped by the column type
func webFormFieldValue(name string, column fieldcatalog.Column, text string) (any, bool) {
	value, problem := checkCell(name, column, text, false)
	return value, problem == nil
}

// WithWebFormThrottle sets the intake throttle's limits from the deployment's
// settings, for the edge and for the state each form's settings show.
func WithWebFormThrottle(limits webforms.ThrottleLimits) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.webFormsHandlers = s.WithThrottle(webforms.NewThrottle(limits, time.Now, s.log, s.ListLiveForms, s.LookupForm))
	}
}

func publicForms(forms webforms.Handlers) func(http.Handler) http.Handler {
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
			// The workspace is already bound: the identity middleware binds the
			// installation's into every request, public paths included.
			ctx := principal.WithActor(r.Context(), principal.Principal{
				Type: principal.PrincipalSystem,
				ID:   "system:public_web_form",
			})
			workspace, _ := principal.WorkspaceID(ctx)
			address := httpserver.ClientIP(r)
			// A preflight has its own ceiling, so it spends no form's budget.
			if r.Method == http.MethodOptions {
				form, v := forms.Throttle().Preflight(ctx, workspace.String(), key, address)
				if v.Refused {
					writeIntakeRefused(w, r, v)
					return
				}
				forms.ServePreflight(w, r.WithContext(ctx), form)
				return
			}
			v := forms.Throttle().Check(ctx, workspace.String(), key, address)
			switch {
			case v.Refused:
				writeIntakeRefused(w, r, v)
				return
			case v.Unknown:
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeIntakeRefused(w http.ResponseWriter, r *http.Request, v webforms.ThrottleVerdict) {
	wait := int(math.Ceil(v.RetryAfter.Seconds()))
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusTooManyRequests, Code: "rate_limited", RetryAfterSeconds: wait,
		Detail: fmt.Sprintf("This form receives too many submissions right now. Wait %d seconds and send it again.", wait),
	})
}

type inquiryWork = inquiries.Work

func newInquiryRouting(db *database.DB) inquiries.Routing {
	return inquiries.NewRouting(db, newWebFormFence(db), NewProviderFor(db))
}

func newInquiryAssignment(db *database.DB) inquiries.Assignment {
	return inquiries.NewAssignment(db, newContactsStore(db), newInquiryRouting(db))
}

func newInquirySettings(db *database.DB, forms *webforms.Store) inquiries.Settings {
	ex := automation.Executors{Provider: NewProviderFor(db), Authority: identity.NewService(db.Pool())}
	return inquiries.NewSettings(db, forms, newInquiryRouting(db), ex, func() *automation.WorkflowEngine { return NewWorkflowEngine(db) })
}

func newInquiryWork(db *database.DB) *inquiries.Work {
	return inquiries.NewWork(db, newInquiryRouting(db))
}

func newInquiryFiringFence(db *database.DB) *inquiries.FiringFence {
	return inquiries.NewFiringFence(db, newWebFormFence(db))
}

func newWebFormFence(db *database.DB) *inquiries.WebFormFence {
	return inquiries.NewWebFormFence(db, boundBySessionSlots)
}

// boundBySessionSlots keeps a budget the context already carries, such as a worker's.
func boundBySessionSlots(ctx context.Context, slots chan struct{}) context.Context {
	if _, bounded := ctx.Value(sessionBudgetKey{}).(chan struct{}); !bounded {
		ctx = context.WithValue(ctx, sessionBudgetKey{}, slots)
	}
	return database.WithSessionBudget(ctx, takeSessionSlot)
}
