// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// setLeadContactFields patches the fields that say who the lead is and how to
// reach them, each parsed the way its create path parses it.
func setLeadContactFields(p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) error {
	if in.FullName != nil {
		name, err := httperr.RequireNonBlank("full_name", *in.FullName)
		if err != nil {
			return err
		}
		p.Set("full_name", current.FullName, name)
	}
	if in.Email != nil {
		parsed, err := values.ParseEmail(*in.Email)
		if err != nil {
			return err
		}
		p.Set("email", current.Email, parsed.String())
	}
	if in.Phone != nil {
		parsed, err := values.ParsePhone(*in.Phone)
		if err != nil {
			return err
		}
		p.Set(leadPhoneColumn, current.Phone, parsed.String())
	}
	if in.Title != nil {
		p.Set("title", current.Title, *in.Title)
	}
	if in.CompanyName != nil {
		p.Set("company_name", current.CompanyName, *in.CompanyName)
	}
	if in.Industry != nil {
		industry, err := leadIndustryWrite(in.Industry)
		if err != nil {
			return err
		}
		p.Set(leadIndustryField, current.Industry, industry)
	}
	if in.LinkedInURL != nil {
		normalized, err := leadLinkedInForUpdate(*in.LinkedInURL, current.LinkedinUrl)
		if err != nil {
			return err
		}
		p.Set(linkedinURLField, current.LinkedinUrl, normalized)
	}
	return nil
}

// buildLeadPatch folds the caller's sparse update onto the current lead as a
// field patch. It reports whether the caller must resume recompute, which a
// cleared score override asks for. A score override is sticky: setting a score
// demands a written reason and keeps the machine value.
func buildLeadPatch(current crmcontracts.Lead, in UpdateLeadInput, now time.Time) (*storekit.Patch, bool, error) {
	p := storekit.NewPatch()
	clears, scoreNamed := withoutScoreOverrideClears(in.Clear)
	in.ClearScoreOverride = in.ClearScoreOverride || scoreNamed
	if err := storekit.ApplyClears(p, clears, clearableLeadColumns(current)); err != nil {
		return nil, false, err
	}
	if err := setLeadContactFields(p, current, in); err != nil {
		return nil, false, err
	}
	setLeadReferences(p, current, in)
	if in.Status != nil {
		status, err := parseWritableLeadStatus(*in.Status)
		if err != nil {
			return nil, false, err
		}
		p.Set(leadStatusColumn, current.Status, string(status))
	}
	// Margince writes how a row arrived. A client may echo the stored value
	// back; any other value would let a caller rewrite "Added via".
	if in.Source != nil && strings.TrimSpace(*in.Source) != current.Source {
		return nil, false, &values.ParseError{
			Field: "source", Code: "read_only",
			Message: "source records how the lead reached Margince and cannot be changed; set origin to say where it came from",
		}
	}
	if err := setLeadOrigin(p, current, in); err != nil {
		return nil, false, err
	}
	resumeRecompute, err := applyScoreOverride(p, current, in)
	if err != nil {
		return nil, false, err
	}
	if in.OwnerID != nil {
		p.Set("owner_id", current.OwnerId, *in.OwnerID)
		// The first owner of an unowned lead starts its response clock, because
		// the clock reads COALESCE(routed_at, created_at). Without the stamp, the
		// new owner inherits a breach earned while the lead had no owner. A
		// reassignment keeps the clock. The stamp rides this patch, so the audit
		// image of the change records it.
		if current.RoutedAt == nil && current.OwnerId == nil {
			stampArrival(p, *in.OwnerID, now)
		}
	}
	return p, resumeRecompute, nil
}
