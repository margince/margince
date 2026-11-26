// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The entitlement surface: what the license grants, and how much of it is used.
//
// Composed here because the answer has two halves that live in different layers
// and must not import each other — the posture is platform's (resolved from the
// deployment file against the bundled validation module) and the seat count is
// identity's (rows in app_user). Neither half is a sensible home for the other,
// and the verdict that pairs them — whether the installation is over its limit —
// belongs where both are already in scope.

import (
	"context"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/installtrial"
	"github.com/margince/margince/backend/internal/compose/licenseentitlement"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type licenseHandlers struct {
	seats *identity.SeatUsageStore
	// posture answers what this process last resolved. A function, not a value:
	// the watcher re-checks while the process runs, so a screen opened on day
	// thirty reports the license as it stands and not as it booted.
	posture func() licensecheck.Posture
	// recheck is the watcher's re-check, set by WithLicenseRecheck.
	recheck func(context.Context) error
	// trial is set by WithInstallationTrial; nil reports none.
	trial *installtrial.Trial
}

// withPosture adds the license half to a handler that already carries the seat
// count, and is the ONLY way the entitlement surface is completed.
//
// It adds to the receiver rather than rebuilding it. A literal restating the
// count and the posture could drop either silently. Held by: TestWithLicensePostureKeepsTheSeatCountItWasGiven, which fails when
// this rebuilds the struct instead of adding to it.
func (h licenseHandlers) withPosture(posture func() licensecheck.Posture) licenseHandlers {
	h.posture = posture
	return h
}

// GetLicenseEntitlement answers the entitlement and the usage measured against
// it.
//
// The seat count comes first because it carries the RBAC gate: the store refuses
// a caller without the `license` read grant, so a role that may not see the
// installation's commercial standing never reaches the posture either.
func (h licenseHandlers) GetLicenseEntitlement(w http.ResponseWriter, r *http.Request) {
	if h.seats == nil || h.posture == nil {
		httperr.NotImplemented(w, r, "GetLicenseEntitlement")
		return
	}
	// Human-only (x-agent-access), like every sibling governance read: what an
	// installation is entitled to is not reconnaissance to hand an agent, even
	// one carrying an admin's passport.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	used, err := h.seats.FullSeatsInUse(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeEntitlement(w, r, used)
}

// writeEntitlement answers the posture as it stands now, with any trial.
func (h licenseHandlers) writeEntitlement(w http.ResponseWriter, r *http.Request, used int) {
	trial, err := h.trial.Contract(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := licenseentitlement.Entitlement(h.posture(), used)
	out.Trial = trial
	httperr.WriteJSON(w, http.StatusOK, out)
}

// CheckLicenseNow runs the watcher's daily re-check at once and answers the
// entitlement it resolved. It writes nothing and takes no token, so there is no
// row to audit. Both gates run before the re-check.
func (h licenseHandlers) CheckLicenseNow(w http.ResponseWriter, r *http.Request) {
	if h.seats == nil || h.posture == nil || h.recheck == nil {
		httperr.NotImplemented(w, r, "CheckLicenseNow")
		return
	}
	ctx := r.Context()
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(ctx, licenseCheckObject, principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The seat count carries the `license` read gate, as on the GET.
	used, err := h.seats.FullSeatsInUse(ctx)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := h.recheck(ctx); err != nil {
		// Logged by the watcher; the cause can quote a path, so it stays out of the response.
		httperr.ServiceUnavailable(w, r, "the license could not be re-read; the last result stands — see the server log")
		return
	}
	h.writeEntitlement(w, r, used)
}

// licenseCheckObject is the grant "Check now" asks for (update; admin and ops).
// No role holds a `license` write grant, and a re-check changes no license.
const licenseCheckObject = "installation_settings"

// GetSeatUsage answers the seat count without the entitlement beside it.
//
// The posture is not read here: a reader who may see capacity is not handed
// what the installation pays. The store gates it on `seat_usage`.
func (h licenseHandlers) GetSeatUsage(w http.ResponseWriter, r *http.Request) {
	if h.seats == nil {
		httperr.NotImplemented(w, r, "GetSeatUsage")
		return
	}
	// Human-only for the same reason the entitlement is: how much of an
	// installation is in use is capacity planning, not something an agent asks
	// for, even one carrying its holder's passport.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	used, err := h.seats.SeatsInUse(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.SeatUsage{SeatsUsed: used})
}
