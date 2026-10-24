// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// How a lead's origin and its note are written: admitted on create, folded onto
// the patch on update, and filled from an importer's wording.

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// cleanOriginDetail trims the note and, when asked, cuts it to the column's
// bound. A long unmatched value is then cut here and not refused by the check.
func cleanOriginDetail(detail string, bound bool) string {
	detail = strings.TrimSpace(detail)
	if bound {
		detail = textcut.Runes(detail, LeadOriginDetailMax)
	}
	return detail
}

// noteWithHint keeps unresolved importer wording in the note, in front of a
// note the same row supplied, so neither is lost. An importer's wording
// is cut to the bound rather than refusing the row.
func noteWithHint(hint string, detail *string) string {
	if detail == nil || strings.TrimSpace(*detail) == "" {
		return cleanOriginDetail(hint, true)
	}
	return cleanOriginDetail(hint+": "+strings.TrimSpace(*detail), true)
}

// admitLeadOrigin settles a new lead's origin and detail before the score is
// derived from them. A key a human, agent or connector named must be an active
// catalog key. An importer's free text goes through ResolveOrigin, and when it
// does not resolve the wording is kept in the detail with origin left NULL.
func admitLeadOrigin(ctx context.Context, tx pgx.Tx, in CreateLeadInput) (CreateLeadInput, error) {
	if in.Origin != nil {
		named, err := assignableOrigin(ctx, tx, *in.Origin)
		if err != nil {
			return in, err
		}
		in.Origin = named
	} else if hint := strings.TrimSpace(in.OriginHint); hint != "" {
		var err error
		if in, err = admitOriginHint(ctx, tx, in, hint); err != nil {
			return in, err
		}
	}
	if in.OriginDetail != nil {
		detail, err := checkOriginDetail(*in.OriginDetail)
		if err != nil {
			return in, err
		}
		in.OriginDetail = nil
		if detail != "" {
			in.OriginDetail = &detail
		}
	}
	return in, nil
}

// setLeadOrigin folds the origin and its note onto the patch. A blank origin is
// refused rather than stored: clearing is the explicit null, which reaches the
// patch through Clear.
func setLeadOrigin(p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) error {
	if in.Origin != nil {
		origin := strings.TrimSpace(*in.Origin)
		if origin == "" {
			return &values.ParseError{Field: leadOriginField, Code: codeRequired, Message: "origin must not be empty; pass null to clear it"}
		}
		p.Set(leadOriginField, current.Origin, origin)
	}
	if in.OriginDetail != nil {
		detail, err := checkOriginDetail(*in.OriginDetail)
		if err != nil {
			return err
		}
		p.Set(leadOriginDetailField, current.OriginDetail, detail)
	}
	return nil
}

// settleOriginHint turns an importer's wording into an origin or a note, but
// only for a lead with no origin. A human may have set it, and the file knows no
// better. This runs under the row lock, so a human's edit that lands between the
// import's read and its write still wins. Wording that does not resolve becomes
// the note, and never replaces one.
func settleOriginHint(ctx context.Context, tx pgx.Tx, current crmcontracts.Lead, in UpdateLeadInput) (UpdateLeadInput, error) {
	hint := strings.TrimSpace(in.OriginHint)
	if hint == "" || current.Origin != nil || in.Origin != nil {
		return in, nil
	}
	key, ok, err := ResolveOrigin(ctx, tx, hint)
	if err != nil {
		return in, err
	}
	switch {
	case ok:
		in.Origin = &key
	case current.OriginDetail == nil:
		detail := noteWithHint(hint, in.OriginDetail)
		in.OriginDetail = &detail
	}
	return in, nil
}

// checkOriginDetail cleans a note and refuses one past the bound.
func checkOriginDetail(raw string) (string, error) {
	detail := cleanOriginDetail(raw, false)
	if utf8.RuneCountInString(detail) > LeadOriginDetailMax {
		return "", &values.ParseError{
			Field: leadOriginDetailField, Code: codeTooLong,
			Message: fmt.Sprintf("origin_detail is at most %d characters", LeadOriginDetailMax),
		}
	}
	return detail, nil
}

// assignableOrigin returns the trimmed key when it is an active catalog key,
// and nil for a blank one.
func assignableOrigin(ctx context.Context, tx pgx.Tx, raw string) (*string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return nil, nil //nolint:nilnil // a blank origin is "no origin", not a failure
	}
	if err := ensureAssignableOrigin(ctx, tx, key, nil); err != nil {
		return nil, err
	}
	return &key, nil
}

// admitOriginHint resolves an importer's wording to a catalog key, or keeps it
// in the detail when no key matches.
func admitOriginHint(ctx context.Context, tx pgx.Tx, in CreateLeadInput, hint string) (CreateLeadInput, error) {
	key, ok, err := ResolveOrigin(ctx, tx, hint)
	if err != nil {
		return in, err
	}
	if ok {
		in.Origin = &key
		return in, nil
	}
	detail := noteWithHint(hint, in.OriginDetail)
	in.OriginDetail = &detail
	return in, nil
}
