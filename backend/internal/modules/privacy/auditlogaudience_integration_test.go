// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// The AUDIENCE boundary on the compliance log, for the four collateral types.
//
// Its sibling next door (auditlogboundary_integration_test.go) proves the
// ERASURE boundary: a tombstone withholds the images written before it. This
// one proves the other rule on the same rows — an image describing an activity
// the reader is outside the audience of is withheld even though nothing was
// erased.
//
// Both are needed because they fail independently. Erasure keys on a tombstone
// that mostly does not exist; the audience keys on a row that always does. The
// four collateral types passed the erasure test for a year while every one of
// their images was legible to an admin who was not on the mail.
//
// Seeded through the owner connection rather than through each module's writer:
// the subject here is what THIS read does with rows that exist, and reaching
// four modules' stores to place them would test their write paths instead.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestACollateralImageFollowsItsActivitysAudience is the leak this closes: an
// admin outside a held thread's audience reading a filename off the audit log.
func TestACollateralImageFollowsItsActivitysAudience(t *testing.T) {
	for _, tc := range []struct {
		entityType string
		// seed places the collateral row and returns the id the audit row is
		// written against.
		seed func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID
	}{{
		entityType: "attachment",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			// The capture path's shape: entity pair AND activity_id.
			return e.seedID(t, `
				INSERT INTO attachment (id, entity_type, entity_id, activity_id, filename,
				                        storage_key, source, captured_by)
				VALUES ($1, 'activity', $2, $2, 'Kuendigung.pdf', 'blob/k', 'gmail',
				        'connector:gmail:'||$3::text)`, activity, e.other)
		},
	}, {
		entityType: "attachment",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			// The manual-upload shape: the polymorphic pair alone, activity_id
			// NULL. A route reading only activity_id would leave this row
			// unjoined, and an unjoined row is withheld — safe, but it would
			// destroy the governance record for every uploaded file, so the
			// coalesce is what this case holds.
			return e.seedID(t, `
				INSERT INTO attachment (id, entity_type, entity_id, filename,
				                        storage_key, source, captured_by)
				VALUES ($1, 'activity', $2, 'Kuendigung.pdf', 'blob/k2', 'upload',
				        'human:'||$3::text)`, activity, e.other)
		},
	}, {
		entityType: "attachment_extraction",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			attachment := e.seedID(t, `
				INSERT INTO attachment (id, entity_type, entity_id, activity_id, filename,
				                        storage_key, source, captured_by)
				VALUES ($1, 'activity', $2, $2, 'Kuendigung.pdf', 'blob/k3', 'gmail',
				        'connector:gmail:'||$3::text)`, activity, e.other)
			return e.seedID(t, `
				INSERT INTO attachment_extraction (id, attachment_id, requested_by)
				VALUES ($1, $2, 'human:'||$3::text)`, attachment, e.other)
		},
	}, {
		entityType: "transcript_read",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			return e.seedID(t, `
				INSERT INTO transcript_read (id, activity_id, requested_by)
				VALUES ($1, $2, 'human:'||$3::text)`, activity, e.other)
		},
	}, {
		entityType: "scheduled_send",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			// A reply-origin send reaches its activity through
			// anchor_activity_id, which is the only route an unreleased send
			// has — its own activity_id is NULL until it is released.
			return e.seedID(t, `
				INSERT INTO scheduled_send (id, anchor_activity_id, origin_kind, status,
				                            scheduled_at, scheduled_tz, payload,
				                            scheduled_by, principal_kind)
				VALUES ($1, $2, 'reply', 'scheduled', now(), 'Europe/Berlin',
				        '{"subject":"Re: Aufhebungsvertrag"}'::jsonb, $3, 'human')`, activity, e.other)
		},
	}} {
		t.Run(tc.entityType, func(t *testing.T) {
			e := setupAuditBoundary(t)
			activity := e.seedHeldActivity(t)
			id := tc.seed(t, e, activity)
			e.seedCollateralAudit(t, tc.entityType, id)

			limit := 50
			page, err := ListAuditLog(e.ctx, e.db,
				AuditFilter{EntityType: &tc.entityType, EntityID: &id, Limit: &limit})
			if err != nil {
				t.Fatalf("ListAuditLog: %v", err)
			}
			// The row must be PRESENT and withheld. Asserting absence alone
			// passes on an empty page, which is the shape a broken filter and a
			// working boundary share.
			found := false
			for _, entry := range page.Entries {
				if entry.Action != "create" {
					continue
				}
				found = true
				if strings.Contains(string(entry.After), "Kuendigung") ||
					strings.Contains(string(entry.After), "Aufhebungsvertrag") {
					t.Errorf("an admin outside the audience read the %s's content: %s",
						tc.entityType, entry.After)
				}
			}
			if !found {
				t.Fatalf("the %s's create row is absent from the page, so this proved nothing",
					tc.entityType)
			}
		})
	}
}

// TestAnExtractionRequestKeepsItsGovernanceKeysWhenWithheld is the other
// direction, and the one a redaction gets wrong by being too eager: withholding
// the extraction image WHOLE destroys "who asked to read this file", which is
// the record a compliance reader opened this log for.
func TestAnExtractionRequestKeepsItsGovernanceKeysWhenWithheld(t *testing.T) {
	e := setupAuditBoundary(t)
	activity := e.seedHeldActivity(t)
	attachment := e.seedID(t, `
		INSERT INTO attachment (id, entity_type, entity_id, activity_id, filename,
		                        storage_key, source, captured_by)
		VALUES ($1, 'activity', $2, $2, 'Kuendigung.pdf', 'blob/g', 'gmail',
		        'connector:gmail:'||$3::text)`, activity, e.other)
	extraction := e.seedID(t, `
		INSERT INTO attachment_extraction (id, attachment_id, requested_by)
		VALUES ($1, $2, 'human:'||$3::text)`, attachment, e.other)
	e.seedCollateralAudit(t, "attachment_extraction", extraction)

	entityType, limit := "attachment_extraction", 50
	page, err := ListAuditLog(e.ctx, e.db,
		AuditFilter{EntityType: &entityType, EntityID: &extraction, Limit: &limit})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Action != "create" {
			continue
		}
		found = true
		for _, key := range []string{"attachment_id", "requested_by"} {
			if !strings.Contains(string(entry.After), key) {
				t.Errorf("%q was redacted away; the audience has no claim over the record of "+
					"who read the file: %s", key, entry.After)
			}
		}
	}
	if !found {
		t.Fatal("the extraction's create row is absent from the page, so this proved nothing")
	}
}

// TestAnUploadedFileOnAHeldThreadIsWithheld holds the polymorphic half of the
// attachment route.
//
// A manual upload writes only the (entity_type, entity_id) pair and leaves
// activity_id NULL, so a route reading activity_id alone never resolves this
// row's activity. It is the case the capture-path fixtures cannot reach, and it
// is a real disclosure rather than a safe miss: the predicate admits a row whose
// entity type is governed only when the activity resolves AND the audience
// holds, so a NULL route withholds — but only because the type is governed. The
// pair of them is what this seeds.
func TestAnUploadedFileOnAHeldThreadIsWithheld(t *testing.T) {
	e := setupAuditBoundary(t)
	activity := e.seedHeldActivity(t)
	attachment := e.seedID(t, `
		INSERT INTO attachment (id, entity_type, entity_id, filename,
		                        storage_key, source, captured_by)
		VALUES ($1, 'activity', $2, 'Kuendigung.pdf', 'blob/u', 'upload',
		        'human:'||$3::text)`, activity, e.other)
	e.seedCollateralAudit(t, "attachment", attachment)

	entityType, limit := "attachment", 50
	page, err := ListAuditLog(e.ctx, e.db,
		AuditFilter{EntityType: &entityType, EntityID: &attachment, Limit: &limit})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Action != "create" {
			continue
		}
		found = true
		if strings.Contains(string(entry.After), "Kuendigung") {
			t.Errorf("an admin outside the audience read the uploaded file's name: %s", entry.After)
		}
	}
	if !found {
		t.Fatal("the attachment's create row is absent from the page, so this proved nothing")
	}
}

// TestAnUploadedFileOnAnOpenThreadKeepsItsName is the coalesce's own case, and
// the only direction that can catch it.
//
// Dropping the polymorphic half does not disclose anything — an unresolved row
// is withheld, which every disclosure test reads as correct. What it does is
// withhold EVERY manually uploaded file's image, including the ones on threads
// the reader may read in full. So the mutation is visible only from a workspace
// activity, where the right answer is that nothing is redacted at all.
func TestAnUploadedFileOnAnOpenThreadKeepsItsName(t *testing.T) {
	e := setupAuditBoundary(t)
	activity := e.seedID(t, `
		INSERT INTO activity (id, kind, audience, source, captured_by, occurred_at)
		VALUES ($1, 'email', 'workspace', 'gmail', 'connector:gmail:'||$2::text, $3)`,
		e.other, boundaryEarlier)
	attachment := e.seedID(t, `
		INSERT INTO attachment (id, entity_type, entity_id, filename,
		                        storage_key, source, captured_by)
		VALUES ($1, 'activity', $2, 'Kuendigung.pdf', 'blob/o', 'upload',
		        'human:'||$3::text)`, activity, e.other)
	e.seedCollateralAudit(t, "attachment", attachment)

	entityType, limit := "attachment", 50
	page, err := ListAuditLog(e.ctx, e.db,
		AuditFilter{EntityType: &entityType, EntityID: &attachment, Limit: &limit})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Action != "create" {
			continue
		}
		found = true
		if !strings.Contains(string(entry.After), "Kuendigung") {
			t.Errorf("an uploaded file on a workspace-visible thread was redacted; its activity "+
				"never resolved, so every manually uploaded file's image is withheld: %s", entry.After)
		}
	}
	if !found {
		t.Fatal("the attachment's create row is absent from the page, so this proved nothing")
	}
}

// TestADocumentOnAPersonKeepsItsName holds the other six parent types.
//
// An attachment hangs off person, organization, deal, lead, activity, project or
// relationship. Only the activity parent has an audience, so treating every
// attachment row as governed would withhold the filename of a contract filed on
// a deal from the compliance log — audit data destroyed to protect an audience
// that does not exist. The route resolves nothing for those six, and a row that
// resolves nothing is not governed.
func TestADocumentOnAPersonKeepsItsName(t *testing.T) {
	e := setupAuditBoundary(t)
	attachment := e.seedID(t, `
		INSERT INTO attachment (id, entity_type, entity_id, filename,
		                        storage_key, source, captured_by)
		VALUES ($1, 'person', $2, 'Kuendigung.pdf', 'blob/p', 'upload',
		        'human:'||$3::text)`, e.person, e.other)
	e.seedCollateralAudit(t, "attachment", attachment)

	entityType, limit := "attachment", 50
	page, err := ListAuditLog(e.ctx, e.db,
		AuditFilter{EntityType: &entityType, EntityID: &attachment, Limit: &limit})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Action != "create" {
			continue
		}
		found = true
		if !strings.Contains(string(entry.After), "Kuendigung") {
			t.Errorf("a document filed on a person was redacted; there is no activity audience "+
				"to enforce, so this is audit data destroyed for nothing: %s", entry.After)
		}
	}
	if !found {
		t.Fatal("the attachment's create row is absent from the page, so this proved nothing")
	}
}

// TestEachRouteResolvesItsOwnActivity is the test the disclosure cases cannot
// be: a route that returns NULL, or resolves the WRONG activity, withholds — and
// withholding is what every "was content disclosed?" assertion reads as correct.
//
// So each route is exercised from an activity the reader MAY read. A working
// route resolves that activity, the audience admits it, and nothing is redacted.
// A broken one resolves NULL and the content vanishes.
func TestEachRouteResolvesItsOwnActivity(t *testing.T) {
	for _, tc := range []struct {
		entityType string
		seed       func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID
	}{{
		entityType: "attachment",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			return e.seedID(t, `
				INSERT INTO attachment (id, entity_type, entity_id, activity_id, filename,
				                        storage_key, source, captured_by)
				VALUES ($1, 'activity', $2, $2, 'Kuendigung.pdf', 'blob/r1', 'gmail',
				        'connector:gmail:'||$3::text)`, activity, e.other)
		},
	}, {
		entityType: "attachment_extraction",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			attachment := e.seedID(t, `
				INSERT INTO attachment (id, entity_type, entity_id, activity_id, filename,
				                        storage_key, source, captured_by)
				VALUES ($1, 'activity', $2, $2, 'Kuendigung.pdf', 'blob/r2', 'gmail',
				        'connector:gmail:'||$3::text)`, activity, e.other)
			return e.seedID(t, `
				INSERT INTO attachment_extraction (id, attachment_id, requested_by)
				VALUES ($1, $2, 'human:'||$3::text)`, attachment, e.other)
		},
	}, {
		entityType: "transcript_read",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			return e.seedID(t, `
				INSERT INTO transcript_read (id, activity_id, requested_by)
				VALUES ($1, $2, 'human:'||$3::text)`, activity, e.other)
		},
	}, {
		entityType: "scheduled_send",
		seed: func(t *testing.T, e *auditBoundaryEnv, activity ids.UUID) ids.UUID {
			return e.seedID(t, `
				INSERT INTO scheduled_send (id, anchor_activity_id, origin_kind, status,
				                            scheduled_at, scheduled_tz, payload,
				                            scheduled_by, principal_kind)
				VALUES ($1, $2, 'reply', 'scheduled', now(), 'Europe/Berlin',
				        '{"subject":"Re: Aufhebungsvertrag"}'::jsonb, $3, 'human')`, activity, e.other)
		},
	}} {
		t.Run(tc.entityType, func(t *testing.T) {
			e := setupAuditBoundary(t)
			// Workspace, not participants: the audience admits the reader, so
			// anything redacted here was redacted because the ROUTE failed.
			activity := e.seedID(t, `
				INSERT INTO activity (id, kind, audience, source, captured_by, occurred_at)
				VALUES ($1, 'email', 'workspace', 'gmail', 'connector:gmail:'||$2::text, $3)`,
				e.other, boundaryEarlier)
			id := tc.seed(t, e, activity)
			e.seedCollateralAudit(t, tc.entityType, id)

			limit := 50
			page, err := ListAuditLog(e.ctx, e.db,
				AuditFilter{EntityType: &tc.entityType, EntityID: &id, Limit: &limit})
			if err != nil {
				t.Fatalf("ListAuditLog: %v", err)
			}
			found := false
			for _, entry := range page.Entries {
				if entry.Action != "create" {
					continue
				}
				found = true
				if !strings.Contains(string(entry.After), "Kuendigung") {
					t.Errorf("the %s's image was redacted on an activity the reader may read in "+
						"full, so its route resolved NULL or the wrong row: %s",
						tc.entityType, entry.After)
				}
			}
			if !found {
				t.Fatalf("the %s's create row is absent from the page, so this proved nothing",
					tc.entityType)
			}
		})
	}
}

// TestAScheduledSendWithNoActivityIsWithheld pins the fail-closed direction. An
// account-origin send that has not been released has neither activity_id nor
// anchor_activity_id, by its own CHECK constraint, so the route resolves NULL
// and there is no audience to consult. Answering "readable" for exactly the rows
// whose audience cannot be checked is how the disclosure returns.
func TestAScheduledSendWithNoActivityIsWithheld(t *testing.T) {
	e := setupAuditBoundary(t)
	send := e.seedID(t, `
		INSERT INTO scheduled_send (id, origin_kind, origin_links, status,
		                            scheduled_at, scheduled_tz, payload,
		                            scheduled_by, principal_kind)
		VALUES ($1, 'account', '[]'::jsonb, 'scheduled', now(), 'Europe/Berlin',
		        '{"subject":"Re: Aufhebungsvertrag"}'::jsonb, $2, 'human')`, e.other)
	e.seedCollateralAudit(t, "scheduled_send", send)

	entityType, limit := "scheduled_send", 50
	page, err := ListAuditLog(e.ctx, e.db,
		AuditFilter{EntityType: &entityType, EntityID: &send, Limit: &limit})
	if err != nil {
		t.Fatalf("ListAuditLog: %v", err)
	}
	found := false
	for _, entry := range page.Entries {
		if entry.Action != "create" {
			continue
		}
		found = true
		if strings.Contains(string(entry.After), "Aufhebungsvertrag") {
			t.Errorf("a send with no resolvable activity was disclosed rather than withheld: %s",
				entry.After)
		}
	}
	if !found {
		t.Fatal("the send's create row is absent from the page, so this proved nothing")
	}
}

// seedHeldActivity writes a participants-limited activity captured by SOMEBODY
// ELSE.
//
// Both halves matter. `participants` is what the audience arm tests, and a
// different capturer is what stops the arm's own `captured_by LIKE '%:<uuid>'`
// clause admitting the reader — an activity this admin captured is one they may
// read, and a fixture built that way would pass whatever the redaction did.
func (e *auditBoundaryEnv) seedHeldActivity(t *testing.T) ids.UUID {
	t.Helper()
	return e.seedID(t, `
		INSERT INTO activity (id, kind, audience, source, captured_by, occurred_at)
		VALUES ($1, 'email', 'participants', 'gmail', 'connector:gmail:'||$2::text, $3)`,
		e.other, boundaryEarlier)
}

// seedCollateralAudit writes the create row whose image carries the content.
func (e *auditBoundaryEnv) seedCollateralAudit(t *testing.T, entityType string, id ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, after, occurred_at)
		VALUES ($1, 'human', 'user:'||$2::text, 'create', $3, $4,
		        jsonb_build_object(
		          'filename', 'Kuendigung.pdf',
		          'subject', 'Re: Aufhebungsvertrag',
		          'attachment_id', 'a-uuid',
		          'requested_by', 'human:x',
		          'scheduled_at', '2026-03-01T09:00:00Z'), $5)`,
		ids.NewV7(), e.other, entityType, id, boundaryEarlier); err != nil {
		t.Fatalf("seeding the %s audit image: %v", entityType, err)
	}
}

// seedID runs one seed statement whose first placeholder is a fresh id, and
// returns that id.
func (e *auditBoundaryEnv) seedID(t *testing.T, statement string, args ...any) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), statement, append([]any{id}, args...)...); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	return id
}
