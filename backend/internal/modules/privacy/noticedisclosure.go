// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What a privacy notice page tells one subject about their own data: the kinds
// held, the uses on record, and the retention that reaches them. It reads the
// subject-access export's own sections, so the page never claims less or more
// than the export would hand over.

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// NoticeSubject is the contact or the website lead a notice link names.
type NoticeSubject struct {
	ContactID ids.ContactID
	LeadID    ids.LeadID
}

// NoticeUse is one use of the subject's data on record, with its basis. That is
// "consent" for a granted purpose, else the kind of a live communication basis.
type NoticeUse struct {
	PurposeKey string
	Basis      string
}

// NoticeRetention is one retention rule that acts on the subject's data.
// KeptByLaw is the correspondence floor that holds some of that data past
// the rule's window, when the rule destroys and the floor binds its scope.
type NoticeRetention struct {
	Scope      string
	RetainDays int
	Action     string
	KeptByLaw  *jurisdiction.RetentionClass
}

// NoticeDisclosure is the page's answer about one subject.
type NoticeDisclosure struct {
	Categories []string
	Uses       []NoticeUse
	Retention  []NoticeRetention
}

// businessCorrespondence is the seeded purpose a communication basis serves.
const businessCorrespondence = "business_correspondence"

// AssembleNoticeDisclosure reads what the privacy page owes one subject.
//
// System principal only: the public confirm edge resolves the link and binds
// one, and the link is the authority. Unlike AssembleSAR it writes no export
// audit row, because it hands over kinds and codes, never a value.
func AssembleNoticeDisclosure(
	ctx context.Context, db *database.DB, country CountryReader, subject NoticeSubject,
) (NoticeDisclosure, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalSystem {
		return NoticeDisclosure{}, apperrors.ErrPermissionDenied
	}
	out := NoticeDisclosure{Categories: []string{}, Uses: []NoticeUse{}, Retention: []NoticeRetention{}}
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		var scopes []string
		var err error
		if !subject.ContactID.IsZero() {
			scopes, err = readContactNotice(ctx, tx, subject.ContactID, &out)
		} else {
			scopes, err = readLeadNotice(ctx, tx, subject.LeadID, &out)
		}
		if err != nil {
			return err
		}
		out.Retention, err = retentionReaching(ctx, tx, country, scopes)
		return err
	})
	return out, err
}

// readContactNotice fills the categories and uses from the export's sections,
// and answers the retention scopes the contact has data in.
func readContactNotice(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, out *NoticeDisclosure) ([]string, error) {
	var pkg SARPackage
	emails, leads, identities, err := subjectReach(ctx, tx, contactID)
	if err != nil {
		return nil, err
	}
	wanted := []*[]map[string]any{
		&pkg.Emails, &pkg.Phones, &pkg.ChannelIdentities, &pkg.LinkedInConnections, &pkg.Relationships,
		&pkg.Deals, &pkg.Activities, &pkg.RawCapture, &pkg.Consent, &pkg.CommunicationBases,
	}
	for _, section := range sarSections(&pkg, contactID, emails, leads, identities, nil) {
		if !slices.Contains(wanted, section.dest) {
			continue
		}
		args := section.args
		if args == nil {
			args = []any{contactID}
		}
		if *section.dest, err = rowMaps(ctx, tx, section.query, args...); err != nil {
			return nil, fmt.Errorf("privacy: reading what a notice discloses: %w", err)
		}
	}
	held, err := rowMaps(ctx, tx, sarSubjectQuery, contactID)
	if err != nil || len(held) == 0 {
		return nil, notFoundIfEmpty(err)
	}
	pkg.Subject = held[0]
	if err := appendSubjectCustomValues(ctx, tx, contactID, pkg.Subject); err != nil {
		return nil, err
	}
	out.Categories = contactCategories(pkg)
	out.Uses = recordedUses(pkg.Consent, pkg.CommunicationBases)
	return contactScopes(pkg), nil
}

func notFoundIfEmpty(err error) error {
	if err != nil {
		return err
	}
	return apperrors.ErrNotFound
}

// contactCategories names the kinds of data the export holds for a contact.
func contactCategories(pkg SARPackage) []string {
	subject := pkg.Subject
	present := func(columns ...string) bool {
		return slices.ContainsFunc(columns, func(column string) bool { return filled(subject[column]) })
	}
	custom := false
	for column, value := range subject {
		custom = custom || (strings.HasPrefix(column, "cf_") && filled(value))
	}
	employed := slices.ContainsFunc(pkg.Relationships, func(row map[string]any) bool { return row["kind"] == "employment" })
	return held([]heldKind{
		{"name", present("full_name", "first_name", "last_name")},
		{"job_title", present("title")},
		{"employer", employed},
		{"email", len(pkg.Emails) > 0},
		{"phone", len(pkg.Phones) > 0},
		{"postal_address", present("address_line1", "address_line2", "address_city", "address_region", "address_postal_code", "address_country")},
		{"social_profile", present("social") || len(pkg.LinkedInConnections) > 0 || len(pkg.ChannelIdentities) > 0},
		{"custom_fields", custom},
		{"correspondence", len(pkg.Activities) > 0},
		{"deals", len(pkg.Deals) > 0},
		{"preferences", len(pkg.Consent) > 0},
	})
}

type heldKind struct {
	code    string
	present bool
}

func held(kinds []heldKind) []string {
	out := []string{}
	for _, kind := range kinds {
		if kind.present {
			out = append(out, kind.code)
		}
	}
	return out
}

// filled reports whether a column holds something a reader would call data.
//
//craft:ignore naked-any an export column value is any by construction: rowMaps returns pgx's rows.Values().
func filled(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case map[string]any:
		return len(v) > 0
	}
	return true
}

// recordedUses is each granted purpose, on consent, and each live
// communication basis, on its own kind, once each.
func recordedUses(consent, bases []map[string]any) []NoticeUse {
	out := []NoticeUse{}
	add := func(use NoticeUse) {
		if !slices.Contains(out, use) {
			out = append(out, use)
		}
	}
	for _, row := range consent {
		if key, ok := row["purpose"].(string); ok && row["state"] == "granted" {
			add(NoticeUse{PurposeKey: key, Basis: "consent"})
		}
	}
	now := time.Now()
	for _, row := range bases {
		kind, ok := row["kind"].(string)
		if !ok || row["revoked_at"] != nil || endedBy(row["valid_until"], now) {
			continue
		}
		add(NoticeUse{PurposeKey: businessCorrespondence, Basis: kind})
	}
	return out
}

// endedBy reports whether a basis' valid_until has passed.
//
//craft:ignore naked-any an export column value is any by construction: rowMaps returns pgx's rows.Values().
func endedBy(until any, now time.Time) bool {
	at, ok := until.(time.Time)
	return ok && !at.After(now)
}

// contactScopes is the retention scopes a contact's export shows data in.
func contactScopes(pkg SARPackage) []string {
	scopes := []string{"contact/no_consent_no_deal"}
	if len(pkg.Activities) > 0 {
		scopes = append(scopes, "activity")
	}
	if slices.ContainsFunc(pkg.Activities, func(row map[string]any) bool { return row["source_system"] == "transcript" }) {
		scopes = append(scopes, "activity/transcript")
	}
	if len(pkg.RawCapture) > 0 {
		scopes = append(scopes, "raw_capture")
	}
	return scopes
}
