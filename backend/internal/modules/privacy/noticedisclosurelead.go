// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// readLeadNotice is readContactNotice for a website lead, which the export does
// not reach. It reads the lead's own columns and the export's basis section,
// keyed on the lead.
func readLeadNotice(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, out *NoticeDisclosure) ([]string, error) {
	var kinds [7]bool
	var open bool
	err := tx.QueryRow(ctx, `
		SELECT btrim(coalesce(d.full_name, '')) <> '', btrim(coalesce(d.title, '')) <> '',
		       btrim(coalesce(d.company_name, '')) <> '' OR d.company_id IS NOT NULL,
		       btrim(coalesce(d.email, '')) <> '', btrim(coalesce(d.phone, '')) <> '',
		       btrim(coalesce(d.linkedin_url, '')) <> '',
		       EXISTS (SELECT 1 FROM activity_link l WHERE l.lead_id = d.id),
		       d.status IN ('new', 'contacted', 'engaged')
		  FROM lead d WHERE d.id = $1`, leadID).
		Scan(&kinds[0], &kinds[1], &kinds[2], &kinds[3], &kinds[4], &kinds[5], &kinds[6], &open)
	if err != nil {
		return nil, fmt.Errorf("privacy: reading which kinds of data this lead holds: %w", err)
	}
	out.Categories = held([]heldKind{
		{"name", kinds[0]}, {"job_title", kinds[1]}, {"employer", kinds[2]}, {"email", kinds[3]},
		{"phone", kinds[4]}, {"social_profile", kinds[5]}, {"correspondence", kinds[6]},
	})
	var pkg SARPackage
	for _, section := range sarCommunicationSections(&pkg, nil, []ids.UUID{leadID.UUID}, []ids.UUID{}) {
		if section.dest != &pkg.CommunicationBases {
			continue
		}
		if pkg.CommunicationBases, err = rowMaps(ctx, tx, section.query, section.args...); err != nil {
			return nil, fmt.Errorf("privacy: reading the bases on record for this lead: %w", err)
		}
	}
	out.Uses = recordedUses(nil, pkg.CommunicationBases)
	scopes, err := leadActivityScopes(ctx, tx, leadID)
	if open {
		scopes = append(scopes, "lead/unconverted")
	}
	return scopes, err
}

// leadActivityScopes is the activity scopes a lead's linked activities fall in.
func leadActivityScopes(ctx context.Context, tx pgx.Tx, leadID ids.LeadID) ([]string, error) {
	var activity, transcript, original bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity a JOIN activity_link l ON l.activity_id = a.id WHERE l.lead_id = $1),
		       EXISTS (SELECT 1 FROM activity a JOIN activity_link l ON l.activity_id = a.id
		                WHERE l.lead_id = $1 AND a.source_system = 'transcript'),
		       EXISTS (SELECT 1 FROM activity a JOIN activity_link l ON l.activity_id = a.id
		                JOIN raw_capture r ON `+storekit.StoredOriginalSQL("a", "r")+`
		                WHERE l.lead_id = $1)`, leadID).Scan(&activity, &transcript, &original)
	if err != nil {
		return nil, fmt.Errorf("privacy: reading which retention rules reach this lead: %w", err)
	}
	scopes := []string{}
	for scope, present := range map[string]bool{"activity": activity, "activity/transcript": transcript, "raw_capture": original} {
		if present {
			scopes = append(scopes, scope)
		}
	}
	return scopes, nil
}

// retentionReaching is the acting rules over the scopes the subject has data
// in. A rule that destroys data in a scope the correspondence floor shields
// names the floor, because the floor keeps that mail past the rule's window.
func retentionReaching(ctx context.Context, tx pgx.Tx, country CountryReader, scopes []string) ([]NoticeRetention, error) {
	retainOnly, err := retainOnlyTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, object_type, category, retain_days, action, lawful_basis, enabled
		FROM retention_policy ORDER BY object_type, category NULLS FIRST, retain_days`)
	if err != nil {
		return nil, err
	}
	policies, err := collectPolicies(rows, retainOnly)
	if err != nil {
		return nil, err
	}
	floor, err := ResolveFloorPolicy(ctx, tx, country)
	if err != nil {
		return nil, err
	}
	strictest := floor.StrictestClass(time.Now())
	out := []NoticeRetention{}
	for _, policy := range policies {
		if !policy.Enabled || policy.SuppressedByPosture || !slices.Contains(scopes, policy.Scope.String()) {
			continue
		}
		rule := NoticeRetention{Scope: policy.Scope.String(), RetainDays: policy.RetainDays, Action: policy.Action}
		if isDestructive(policy.Action) && floorShields(policy.Scope) && strictest.Keep != (jurisdiction.Period{}) {
			kept := strictest
			rule.KeptByLaw = &kept
		}
		out = append(out, rule)
	}
	return out, nil
}

// floorShields reports whether the evaluator's selector for this scope keeps
// what the correspondence floor shields, read off the selector itself.
func floorShields(scope RetentionScope) bool {
	selector, known := retentionSelectors[scope.selectorKey()]
	return known && strings.Contains(selector, correspondenceFloorPredicate(3, 4))
}
