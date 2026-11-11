// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestIdentityHolderPlansReadOnlyTheRequestedMailboxes(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	var wanted ids.ContactID
	var wantedLead ids.LeadID
	for i := range 128 {
		id := e.plainContact(ctx, t, fmt.Sprintf("Lookup Contact %d", i), fmt.Sprintf("lookup-%d@bounded.test", i))
		lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new(fmt.Sprintf("Lookup Lead %d", i)), Email: new(fmt.Sprintf("lead-%d@bounded.test", i)), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 64 {
			wanted = ids.From[ids.ContactKind](id)
			wantedLead = ids.From[ids.LeadKind](ids.UUID(lead.Id))
		}
	}
	// Planner statistics survive resets; measure the population this fixture seeded.
	analyzeAsOwner(t, "contact_email", "contact", "lead")
	config := e.store.db.Pool().Config()
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	config.MaxConns = 1
	pool, err := testdb.OwnPoolFromConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cases := []struct {
		email   string
		contact ids.ContactID
		lead    ids.LeadID
	}{
		{"LOOKUP-64@bounded.test", wanted, ids.LeadID{}},
		{"lead-64@bounded.test", ids.ContactID{}, wantedLead},
		{"absent@bounded.test", ids.ContactID{}, ids.LeadID{}},
	}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.email, func(t *testing.T) {
					keys := storekit.IdentityKeys{Emails: []string{tc.email, tc.email}}
					err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
						if _, err := tx.Exec(ctx, `SELECT set_config('plan_cache_mode', @mode, true)`, pgx.NamedArgs{"mode": mode}); err != nil {
							return err
						}
						// A few hundred rows read faster in a sequential scan once autovacuum has
						// counted them. The check asks for the plan a full mailbox gets.
						if _, err := tx.Exec(ctx, `SELECT set_config('enable_seqscan', 'off', true)`); err != nil {
							return err
						}
						holders, err := storekit.IdentityHoldersOf(ctx, tx, keys)
						if err != nil {
							return err
						}
						want := []ids.ContactID{}
						wantLeads := []ids.LeadID{}
						if tc.contact != (ids.ContactID{}) {
							want = append(want, tc.contact)
						}
						if tc.lead != (ids.LeadID{}) {
							wantLeads = append(wantLeads, tc.lead)
						}
						if !slices.Equal(holders.Contacts, want) || !slices.Equal(holders.Leads, wantLeads) {
							return fmt.Errorf("identity holders=%+v, want contacts=%v and leads=%v", holders, want, wantLeads)
						}
						var statement string
						if err := tx.QueryRow(ctx, `SELECT name FROM pg_prepared_statements
						 WHERE statement LIKE '%WITH emails AS%' AND statement NOT LIKE '%pg_prepared_statements%'`).Scan(&statement); err != nil {
							return err
						}
						values := []any{storekit.EmailLookups(keys.Emails...), keys.LinkedInURLs}
						query := `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) EXECUTE ` + pgx.Identifier{statement}.Sanitize() + `(` + storekit.Placeholders(values) + `)`
						var raw []byte
						if err := tx.QueryRow(ctx, query, append([]any{pgx.QueryExecModeSimpleProtocol}, values...)...).Scan(&raw); err != nil {
							return err
						}
						var plans []struct{ Plan mailboxPlanNode }
						if err := json.Unmarshal(raw, &plans); err != nil {
							return err
						}
						if len(plans) != 1 || !identityMailPlanIsBounded(plans[0].Plan, len(want), len(wantLeads)) {
							return fmt.Errorf("identity lookup reads beyond its matching mailbox: %s", raw)
						}
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func identityMailPlanIsBounded(node mailboxPlanNode, matches, leads int) bool {
	// An address is looked up by its dedupe key or by its mailbox base, so
	// either index on contact_email reads only the matching mailbox.
	indexes := map[string][]string{
		"contact": {"contact_pkey"}, "contact_email": {"uq_contact_email_dedupe", "idx_contact_email_mailbox_base"},
		"lead": {"uq_lead_email_dedupe"},
	}
	limit := matches
	if node.RelationName == "lead" {
		limit = leads
	}
	if index, bounded := indexes[node.RelationName]; bounded && node.ActualLoops > 0 {
		if !slices.ContainsFunc(index, node.usesIndex) || node.ActualRows*node.ActualLoops > limit {
			return false
		}
	}
	if node.RelationName == "contact_social" && node.ActualLoops != 0 {
		return false
	}
	for _, child := range node.Plans {
		if !identityMailPlanIsBounded(child, matches, leads) {
			return false
		}
	}
	return true
}

func TestIdentityHoldersKeepBothLanesAndExcludeArchivedContacts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const profile = "https://www.linkedin.com/in/shared-holder"
	one := e.createWithLinkedIn(ctx, t, "First Holder", "first@holder.test", profile)
	two := e.createWithLinkedIn(ctx, t, "Second Holder", "second@holder.test", profile)
	const leadProfile = "https://www.linkedin.com/in/lead-holder"
	lead, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: new("Lead Holder"), Email: new("lead@holder.test"), LinkedInURL: new(leadProfile), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	keys := storekit.IdentityKeys{
		Emails:       []string{" FIRST@holder.test ", "second@holder.test", "lead@holder.test", "first@holder.test"},
		LinkedInURLs: []string{profile, leadProfile, profile},
	}
	check := func(want []ids.ContactID) {
		t.Helper()
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			holders, err := storekit.IdentityHoldersOf(ctx, tx, keys)
			if err != nil {
				return err
			}
			if !slices.Equal(holders.Contacts, want) || len(holders.Leads) != 1 || holders.Leads[0].UUID != ids.UUID(lead.Id) {
				return fmt.Errorf("holders=%+v, want contacts=%v and lead=%v", holders, want, lead.Id)
			}
			if holders.ContactHoldsEmail("second@holder.test") != slices.Contains(want, two) {
				return fmt.Errorf("second address survived a contact archive: %+v", holders)
			}
			if !holders.ContactHoldsEmail("first@holder.test") || !holders.ContactHoldsLinkedIn(profile) || !holders.LeadHoldsEmail("lead@holder.test") || !holders.LeadHoldsLinkedIn(leadProfile) {
				return fmt.Errorf("identity lanes lost a requested key: %+v", holders)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	check([]ids.ContactID{one, two})
	if _, err := e.store.ArchiveContact(ctx, two, nil); err != nil {
		t.Fatal(err)
	}
	check([]ids.ContactID{one})
}

func TestIdentityHoldersFoldDomainsWithoutRoutingGmailReviewPairs(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	international := ids.From[ids.ContactKind](e.plainContact(ctx, t, "Domain Holder", "pat@xn--bcher-kva.example"))
	dotted := ids.From[ids.ContactKind](e.plainContact(ctx, t, "Dotted Holder", "a.b@gmail.com"))
	plain := ids.From[ids.ContactKind](e.plainContact(ctx, t, "Plain Holder", "ab@gmail.com"))
	for _, tc := range []struct {
		asked string
		want  ids.ContactID
	}{
		{" PAT@BÜCHER.EXAMPLE ", international},
		{"A.B@GMAIL.COM", dotted},
		{"ab@gmail.com", plain},
	} {
		t.Run(tc.asked, func(t *testing.T) {
			if err := e.store.tx(ctx, func(tx pgx.Tx) error {
				holders, err := storekit.IdentityHoldersOf(ctx, tx, storekit.IdentityKeys{Emails: []string{tc.asked}})
				if err != nil {
					return err
				}
				if !slices.Equal(holders.Contacts, []ids.ContactID{tc.want}) || len(holders.Leads) != 0 || !holders.ContactHoldsEmail(tc.asked) {
					return fmt.Errorf("holders=%+v, want only %v", holders, tc.want)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
