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
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
)

type licenseHandlers struct {
	seats *identity.SeatUsageStore
	// posture answers what this process last resolved. A function, not a value:
	// the watcher re-checks while the process runs, so a screen opened on day
	// thirty reports the license as it stands and not as it booted.
	posture func() licensecheck.Posture
}

// withPosture adds the license half to a handler that already carries the seat
// count, and is the ONLY way the entitlement surface is completed.
//
// It takes the receiver's store rather than building a second one. The two
// halves genuinely come from different places — the count from the assembly,
// because app_user rows exist whether or not a license was configured; the
// posture from the option, because only that caller has one — and a struct
// literal restating both would let an edit to either drop the other silently.
//
// Held by: TestWithLicensePostureKeepsTheSeatCountItWasGiven, which fails when
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
	httperr.WriteJSON(w, http.StatusOK, toContractLicenseEntitlement(h.posture(), used))
}

// GetSeatUsage answers the seat count without the entitlement beside it.
//
// The posture is not read here at all, and that is the split: this surface
// exists so a reader who may see capacity is not thereby handed what the
// installation pays. Its gate is the store's — `seat_usage` rather than
// `license` — so the two surfaces differ in exactly one thing, which grant
// opens them.
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

// renewalWindow is how long before expiry the surface asks for a renewal.
//
// It is the same 90 days the module's grace period runs for, on purpose: an
// installation is warned for as long before expiry as it keeps working after it.
// The two are NOT one constant — the module owns its grace and does not report
// it — so if upstream ever changes that window, this stops matching silently.
// Asking for a `grace_until` is tracked on issue #1190.
const renewalWindow = 90 * 24 * time.Hour

// toContractLicenseEntitlement renders one posture and one count onto the wire.
//
// The reason a rejection carries is deliberately NOT here. It is the module's
// text about the installation's own configuration — and the module quotes token
// content it has not verified — so it belongs in the boot error and the process
// log, never in a response.
func toContractLicenseEntitlement(posture licensecheck.Posture, used int) crmcontracts.LicenseEntitlement {
	out := crmcontracts.LicenseEntitlement{
		State:     crmcontracts.LicenseEntitlementState(posture.State),
		SeatsUsed: used,
		CheckedAt: posture.CheckedAt,
	}
	if posture.State == licensecheck.StateValid {
		holder := toContractLicenseHolder(posture.License, posture.CheckedAt)
		out.License = &holder
	}
	granted, capped := posture.Seats()
	if !capped {
		// Absent, never zero. A client rendering a missing cap as 0 would tell an
		// admin their license permits nobody, and `over_limit` stays false because
		// there is no limit to be over — not because the installation is inside one.
		return out
	}
	out.SeatsGranted = &granted
	out.OverLimit = used > granted
	return out
}

// toContractLicenseHolder renders who holds the license and how long it lasts.
//
// Every optional claim is rendered only when the token carries it. A license
// issued before those claims existed verifies exactly like any other, and a
// client should show the rows it has rather than placeholders for the rest.
//
// The token never appears here. It is a credential and this response reaches a
// browser.
func toContractLicenseHolder(license licensecheck.License, now time.Time) crmcontracts.LicenseHolder {
	holder := crmcontracts.LicenseHolder{
		Id:      license.ID,
		Subject: license.Subject,
		Expiry:  license.Expiry,
		InGrace: license.InGrace,
		// True whenever the license is already on its grace period, so a client
		// that reads only this one still warns.
		RenewalDue: license.InGrace || !license.Expiry.After(now.Add(renewalWindow)),
	}
	if license.Org != "" {
		holder.Org = &license.Org
	}
	if license.ContactName != "" {
		holder.ContactName = &license.ContactName
	}
	if license.ContactEmail != "" {
		holder.ContactEmail = &license.ContactEmail
	}
	return holder
}
