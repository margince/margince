// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package licenseentitlement renders the license posture onto the wire: the
// entitlement every license read answers, and who holds the license.
package licenseentitlement

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/licensecheck"
)

// renewalWindow is how long before expiry the surface asks for a renewal.
//
// It matches the module's 90-day grace period. An installation is warned for as
// long before expiry as it keeps working after it. The two are not one constant:
// the module owns its grace and does not report it. If upstream changes that
// window, this stops matching without a sign.
const renewalWindow = 90 * 24 * time.Hour

// Entitlement renders one posture and one count onto the wire.
//
// The reason a rejection carries is left out. It is the module's text about the
// installation's own configuration, and it quotes token content the module has
// not verified. It belongs in the boot error and the process log, never in a
// response.
func Entitlement(posture licensecheck.Posture, used int) crmcontracts.LicenseEntitlement {
	out := crmcontracts.LicenseEntitlement{
		State:     crmcontracts.LicenseEntitlementState(posture.State),
		SeatsUsed: used,
		CheckedAt: posture.CheckedAt,
	}
	if posture.State == licensecheck.StateValid {
		holder := Holder(posture.License, posture.CheckedAt)
		out.License = &holder
	}
	// The ceiling a new seat is held to. A refused license keeps the last valid
	// one, and the screen shows the number an invite is refused against.
	granted, capped := posture.SeatCeiling()
	if !capped {
		// Absent, never zero. A client rendering a missing cap as 0 would tell an
		// admin their license permits nobody. `over_limit` stays false because
		// there is no limit to be over, not because the installation is inside one.
		return out
	}
	out.SeatsGranted = &granted
	out.OverLimit = used > granted
	return out
}

// Holder renders who holds the license and how long it lasts.
//
// Every optional claim is rendered only when the token carries it. A license
// issued before those claims existed verifies like any other. A client shows the
// rows it has rather than placeholders for the rest.
//
// The token never appears here. It is a credential and this response reaches a
// browser.
func Holder(license licensecheck.License, now time.Time) crmcontracts.LicenseHolder {
	holder := crmcontracts.LicenseHolder{
		Id:      license.ID,
		Subject: license.Subject,
		Expiry:  license.Expiry,
		InGrace: license.InGrace,
		// True whenever the license is already on its grace period, so a client
		// that reads only this one still warns.
		RenewalDue: license.InGrace || !license.Expiry.After(now.Add(renewalWindow)),
	}
	if license.Company != "" {
		holder.Company = &license.Company
	}
	if license.ContactName != "" {
		holder.ContactName = &license.ContactName
	}
	if license.ContactEmail != "" {
		holder.ContactEmail = &license.ContactEmail
	}
	return holder
}
