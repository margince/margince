// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// What the technical lookup writes, over a real Postgres.
//
// Four of the five guarantees below are invisible to a unit test because they
// are SQL: the reconciliation is a DELETE with a NOT LIKE guard, the human
// precedence is a WHERE clause on an upsert, the evidence obligation is a CHECK
// constraint, and the audit-plus-outbox pairing is two rows in one transaction.
// Each was written to be provable here rather than asserted about in prose.
//
// The fifth is the one that would be easiest to get wrong and never notice: a
// lane that FAILED must change nothing. An upsert-only writer passes every
// other test in this file and still leaves a company carrying two mail
// providers after it moves.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// observedAt is fixed so retrieved_at is assertable: the write stamps what it
// is given, and a real clock would make the assertion about time rather than
// about the write.
var technicalObservedAt = time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)

// seedTechnicalOrg creates a company for the lookup to write onto.
func seedTechnicalOrg(ctx context.Context, t *testing.T, e *dedupeEnv, name, domain string) ids.OrganizationID {
	t.Helper()
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: name, Source: "manual",
		Domains: []OrgDomainInput{{Domain: domain, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seed org %s: %v", name, err)
	}
	return ids.From[ids.OrganizationKind](ids.UUID(org.Id))
}

// technicalFactsOf reads back what the record holds, keyed field→value_key.
func technicalFactsOf(ctx context.Context, t *testing.T, e *dedupeEnv, orgID ids.OrganizationID) map[string][]string {
	t.Helper()
	held := map[string][]string{}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT field, value_key FROM organization_fact
			 WHERE organization_id = $1 AND category = 'signal'
			 ORDER BY field, value_key`, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var field, valueKey string
			if err := rows.Scan(&field, &valueKey); err != nil {
				return err
			}
			held[field] = append(held[field], valueKey)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("read back the technical facts: %v", err)
	}
	return held
}

// observation builds one well-formed reading, which always names what proved it
// — the DDL refuses a technical fact that does not.
func observation(field, valueKey, value string) TechnicalObservation {
	return TechnicalObservation{
		Field: field, ValueKey: valueKey, Value: value,
		Evidence: "aspmx.l.google.com", SourceURL: "dns:example.de",
	}
}

// TestACompletedLaneReplacesItsOwnRows is the reconciliation, which is what
// makes a completed lane authoritative rather than additive.
//
// Without it a company that moves from Google Workspace to Microsoft 365 keeps
// both providers forever, and a rep reading the record cannot tell which one
// answers their mail today.
func TestACompletedLaneReplacesItsOwnRows(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Beispiel GmbH", "beispiel.de")

	first := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations:   []TechnicalObservation{observation(FactMailProvider, "google_workspace", "Google Workspace")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, first, nil); err != nil {
		t.Fatalf("apply the first reading: %v", err)
	}

	moved := first
	moved.Observations = []TechnicalObservation{observation(FactMailProvider, "microsoft365", "Microsoft 365")}
	if err := e.store.ApplyTechnicalEnrichment(ctx, moved, nil); err != nil {
		t.Fatalf("apply the reading after the move: %v", err)
	}

	held := technicalFactsOf(ctx, t, e, orgID)
	providers := held[FactMailProvider]
	if len(providers) != 1 || providers[0] != "microsoft365" {
		t.Errorf("the record holds mail providers %v, want exactly [microsoft365] — a company has one "+
			"mail system, and keeping the one it left is a claim that is no longer true", providers)
	}
}

// TestAFailedLaneChangesNothing is the guarantee an upsert-only writer would
// pass every other test without holding.
//
// A certificate log being down is the common case. Recording that as "this
// company operates no services" deletes a webshop signal a rep was about to act
// on, and the next run puts it back — so the record flickers with the log's
// uptime rather than with what the company runs.
func TestAFailedLaneChangesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Laden GmbH", "laden.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneCertLog},
		Observations:   []TechnicalObservation{observation(FactOperatedService, "webshop", "Webshop")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the first reading: %v", err)
	}

	// The next run: the certificate log did not answer, so its lane is absent
	// from Completed and carries no observations.
	outage := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations:   []TechnicalObservation{observation(FactMailProvider, "self_hosted", "Eigener Mailserver")},
		ObservedAt:     technicalObservedAt.Add(time.Hour),
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, outage, nil); err != nil {
		t.Fatalf("apply the reading taken during the outage: %v", err)
	}

	held := technicalFactsOf(ctx, t, e, orgID)
	if len(held[FactOperatedService]) != 1 {
		t.Errorf("the webshop is gone from the record after a lane that never answered: %v. "+
			"'The log did not answer' and 'the company has nothing' are different facts", held)
	}
	if len(held[FactMailProvider]) != 1 {
		t.Errorf("the lane that DID complete wrote nothing: %v", held)
	}
}

// TestAHumanCorrectionSurvivesEveryLaterLookup holds the precedence rule.
//
// The lookup runs weekly and forever. A correction it could overwrite is a
// correction that lasts until the next pass, which is worse than not offering
// the correction at all.
func TestAHumanCorrectionSurvivesEveryLaterLookup(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Korrektur GmbH", "korrektur.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations:   []TechnicalObservation{observation(FactMailProvider, "other", "Anderer Anbieter")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the machine reading: %v", err)
	}

	// A person corrects the VALUE of the row the lookup wrote, keeping its
	// value_key — which is what a correction is. The row is now theirs, marked
	// by captured_by, the column the precedence guard tests.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE organization_fact
			   SET value = 'Eigener Mailserver',
			       captured_by = 'human:' || $2, source = 'human'
			 WHERE organization_id = $1 AND field = $3`,
			orgID, e.rep.String(), FactMailProvider)
		return err
	}); err != nil {
		t.Fatalf("record the human's correction: %v", err)
	}

	// The weekly pass runs again and reads what it read before.
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the later reading: %v", err)
	}

	var value, capturedBy string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT value, captured_by FROM organization_fact
			 WHERE organization_id = $1 AND field = $2`, orgID, FactMailProvider).Scan(&value, &capturedBy)
	}); err != nil {
		t.Fatalf("read back the corrected fact: %v", err)
	}
	if value != "Eigener Mailserver" {
		t.Errorf("the lookup overwrote a person's correction with %q", value)
	}
	if capturedBy == technicalCapturedBy {
		t.Error("the lookup reclaimed a row a human had corrected — the next pass would overwrite it again")
	}
}

// TestAHumanHeldRowIsNeverReconciledAway is the other half of precedence, and
// the one the reconciliation could break on its own.
//
// The upsert declines to overwrite a human's row. A DELETE that did not carry
// the same guard would remove it instead, which is the same defect arriving
// through the other door.
func TestAHumanHeldRowIsNeverReconciledAway(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Bestand GmbH", "bestand.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneCertLog},
		Observations:   []TechnicalObservation{observation(FactOperatedService, "careers", "Karriereseite")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the first reading: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE organization_fact SET captured_by = 'human:' || $2, source = 'human'
			 WHERE organization_id = $1 AND field = $3`, orgID, e.rep.String(), FactOperatedService)
		return err
	}); err != nil {
		t.Fatalf("mark the row as a person's: %v", err)
	}

	// The lane completes and no longer sees the careers page.
	gone := read
	gone.Observations = nil
	gone.ObservedAt = technicalObservedAt.Add(time.Hour)
	if err := e.store.ApplyTechnicalEnrichment(ctx, gone, nil); err != nil {
		t.Fatalf("apply the reading that no longer sees it: %v", err)
	}

	held := technicalFactsOf(ctx, t, e, orgID)
	if len(held[FactOperatedService]) != 1 {
		t.Error("the reconciliation removed a row a person had claimed — the upsert's precedence " +
			"guard and the delete's must be the same rule, or a correction survives one pass and not the other")
	}
}

// TestACompletedLaneWithNothingClearsItsRows is the empty answer, which is a
// fact rather than a gap.
//
// A company that takes its webshop down publishes nothing where the shop was,
// and the record must say so — otherwise a rep calls about a shop that closed.
func TestACompletedLaneWithNothingClearsItsRows(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Leer GmbH", "leer.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneCertLog},
		Observations:   []TechnicalObservation{observation(FactOperatedService, "webshop", "Webshop")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the first reading: %v", err)
	}

	empty := read
	empty.Observations = nil
	empty.ObservedAt = technicalObservedAt.Add(time.Hour)
	if err := e.store.ApplyTechnicalEnrichment(ctx, empty, nil); err != nil {
		t.Fatalf("apply the authoritative empty reading: %v", err)
	}

	if held := technicalFactsOf(ctx, t, e, orgID); len(held[FactOperatedService]) != 0 {
		t.Errorf("the record still claims %v after the source answered that there is none", held)
	}
	// A technology LEAVING is a change, and the one a rep most wants to hear
	// about: the arrival announced, and so must the departure. Both events, not
	// one — the guard on the empty delta must not swallow this.
	var events int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->>'type' = 'organization.updated'
			   AND envelope->'entity'->>'id' = $1::text`, orgID.String()).Scan(&events)
	}); err != nil {
		t.Fatalf("read back the events: %v", err)
	}
	if events != 2 {
		t.Errorf("%d organization.updated events for an arrival and a departure, want 2 — a technology leaving is a change", events)
	}
}

// TestEveryTechnicalWriteCommitsItsAuditAndItsEvent holds the write shape.
func TestEveryTechnicalWriteCommitsItsAuditAndItsEvent(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Spur GmbH", "spur.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations:   []TechnicalObservation{observation(FactMailProvider, "microsoft365", "Microsoft 365")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the reading: %v", err)
	}

	var audits, events int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'organization' AND entity_id = $1
			   AND evidence->>'source' = $2`, orgID, companySourceTechnical).Scan(&audits); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->'entity'->>'id' = $1::text`, orgID.String()).Scan(&events)
	}); err != nil {
		t.Fatalf("read back the trail: %v", err)
	}
	if audits != 1 {
		t.Errorf("the write left %d audit rows naming the technical lookup, want 1", audits)
	}
	if events == 0 {
		t.Error("the write left an audit row with no outbox event — the two are one transaction")
	}
}

// TestALaneThatChangedNothingRecordsItAndAnnouncesNothing is the other half of
// the write shape.
//
// A lane that completed having found nothing is worth an AUDIT row: it is what
// says when the record was last looked at, and it is what lets a technology the
// company dropped leave. It is not an update. Nothing was written, nothing was
// removed, and `organization.updated` announcing an empty delta tells every
// subscriber a record moved when it did not.
//
// Most companies' sites declare no technology this build recognises, so the
// homepage lane hit this shape on nearly every site read in an installation —
// a no-op event each time, which every subscriber had to learn to ignore.
func TestALaneThatChangedNothingRecordsItAndAnnouncesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Leerlauf GmbH", "leerlauf.de")

	empty := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneHomepage},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, empty, nil); err != nil {
		t.Fatalf("apply the empty reading: %v", err)
	}

	var audits, events int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'organization' AND entity_id = $1
			   AND evidence->>'source' = $2`, orgID, companySourceTechnical).Scan(&audits); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->>'type' = 'organization.updated'
			   AND envelope->'entity'->>'id' = $1::text`, orgID.String()).Scan(&events)
	}); err != nil {
		t.Fatalf("read back the trail: %v", err)
	}
	if audits != 1 {
		t.Errorf("the lane left %d audit rows, want 1: a completed lane is worth recording even when it found nothing", audits)
	}
	if events != 0 {
		t.Errorf("the lane announced %d organization.updated events having changed nothing — a subscriber acting on one finds an identical record", events)
	}
}

// TestARefreshThatFoundTheSameStackAnnouncesNothing is the same rule on the
// other common shape.
//
// A lane re-reading a company whose stack has not moved UPSERTS every fact it
// found, so the write count is not zero — but nothing appeared, moved or went.
// Most companies that declare a recognisable technology declare the same one
// they declared yesterday, so this is the nightly case, not an edge.
func TestARefreshThatFoundTheSameStackAnnouncesNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Gleichstand GmbH", "gleichstand.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations:   []TechnicalObservation{observation(FactMailProvider, "microsoft365", "Microsoft 365")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the first reading: %v", err)
	}
	// The SAME answer again, which is what a nightly lane produces.
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the refresh: %v", err)
	}

	var audits, events int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'organization' AND entity_id = $1
			   AND evidence->>'source' = $2`, orgID, companySourceTechnical).Scan(&audits); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM event_outbox
			 WHERE envelope->>'type' = 'organization.updated'
			   AND envelope->'entity'->>'id' = $1::text`, orgID.String()).Scan(&events)
	}); err != nil {
		t.Fatalf("read back the trail: %v", err)
	}
	if audits != 2 {
		t.Errorf("the two lanes left %d audit rows, want 2: each one looked, and when it looked is the record", audits)
	}
	if events != 1 {
		t.Errorf("%d organization.updated events for one arrival and one refresh, want 1 — the refresh moved nothing", events)
	}
}

// TestATechnicalFactMustNameWhatProvedIt holds the evidence obligation the DDL
// carries, and it is the product promise in constraint form: a rep asking "how
// do you know they run Microsoft 365?" is owed the MX host.
func TestATechnicalFactMustNameWhatProvedIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Beweis GmbH", "beweis.de")

	unproven := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations: []TechnicalObservation{{
			Field: FactMailProvider, ValueKey: "microsoft365", Value: "Microsoft 365",
			// No evidence, no source: exactly what the constraint refuses.
		}},
		ObservedAt: technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, unproven, nil); err == nil {
		t.Error("a technical fact naming nothing that proved it was accepted")
	}
}

// TestTheObservationTimeIsWhatTheSourceWasRead holds retrieved_at, which is the
// column the record's freshness is read from.
func TestTheObservationTimeIsWhatTheSourceWasRead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Zeit GmbH", "zeit.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations:   []TechnicalObservation{observation(FactMailProvider, "google_workspace", "Google Workspace")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the reading: %v", err)
	}

	var retrievedAt *time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT retrieved_at FROM organization_fact
			 WHERE organization_id = $1 AND field = $2`, orgID, FactMailProvider).Scan(&retrievedAt)
	}); err != nil {
		t.Fatalf("read back the observation time: %v", err)
	}
	if retrievedAt == nil || !retrievedAt.UTC().Equal(technicalObservedAt) {
		t.Errorf("the fact says it was read at %v, want %v — a profile with no honest read time "+
			"cannot be shown as stale", retrievedAt, technicalObservedAt)
	}
}

// TestTheLaneLedgerRecordsEachSourceSeparately holds the per-lane ledger, which
// is what lets one source be stale while another is fresh.
func TestTheLaneLedgerRecordsEachSourceSeparately(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Buch GmbH", "buch.de")

	if err := e.store.RecordTechnicalLane(ctx, orgID, LaneDNS, TechnicalOutcomeApplied, technicalObservedAt); err != nil {
		t.Fatalf("record the DNS lane: %v", err)
	}
	if err := e.store.RecordTechnicalLane(ctx, orgID, LaneCertLog, TechnicalOutcomeFailed, technicalObservedAt); err != nil {
		t.Fatalf("record the certificate lane: %v", err)
	}

	lanes, err := e.store.TechnicalLaneState(ctx, orgID)
	if err != nil {
		t.Fatalf("read the ledger: %v", err)
	}
	if len(lanes) != 2 {
		t.Fatalf("the ledger holds %d lanes, want 2", len(lanes))
	}
	for _, lane := range lanes {
		switch lane.Lane {
		case string(LaneDNS):
			if lane.LastSuccessAt == nil {
				t.Error("a lane that applied its reading recorded no success")
			}
		case string(LaneCertLog):
			if lane.LastSuccessAt != nil {
				t.Error("a lane that failed recorded a success — its backoff would reset on a failure")
			}
		}
	}
}

// TestAHumanAnswerSettlesTheWholeSingleValuedField is the cross-key half of
// precedence, and the row-level guard does not cover it.
//
// A person who corrects the mail provider holds THAT row. A later reading of a
// different provider is a different value_key, so an upsert guarded only on
// the row would insert beside their answer — and the record would then claim
// two mail systems, one of them the one a person had just rejected.
func TestAHumanAnswerSettlesTheWholeSingleValuedField(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Entschieden GmbH", "entschieden.de")

	read := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations:   []TechnicalObservation{observation(FactMailProvider, "google_workspace", "Google Workspace")},
		ObservedAt:     technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, read, nil); err != nil {
		t.Fatalf("apply the machine reading: %v", err)
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE organization_fact
			   SET value = 'Eigener Mailserver', captured_by = 'human:' || $2, source = 'human'
			 WHERE organization_id = $1 AND field = $3`,
			orgID, e.rep.String(), FactMailProvider)
		return err
	}); err != nil {
		t.Fatalf("record the human's correction: %v", err)
	}

	// The next pass reads a DIFFERENT provider — a new value_key, so it misses
	// the row-level guard entirely.
	moved := read
	moved.Observations = []TechnicalObservation{observation(FactMailProvider, "microsoft365", "Microsoft 365")}
	moved.ObservedAt = technicalObservedAt.Add(time.Hour)
	if err := e.store.ApplyTechnicalEnrichment(ctx, moved, nil); err != nil {
		t.Fatalf("apply the later reading: %v", err)
	}

	held := technicalFactsOf(ctx, t, e, orgID)
	if len(held[FactMailProvider]) != 1 {
		t.Fatalf("the record claims %v mail providers — a company has one, and a person had already "+
			"said which", held[FactMailProvider])
	}
	var value string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT value FROM organization_fact
			 WHERE organization_id = $1 AND field = $2`, orgID, FactMailProvider).Scan(&value)
	}); err != nil {
		t.Fatalf("read back the surviving fact: %v", err)
	}
	if value != "Eigener Mailserver" {
		t.Errorf("the surviving mail provider is %q, want the person's answer", value)
	}
}

// TestAPartialDNSReadIsNotAuthoritative holds the other blocker's fix: a lane
// that read some of what it owns and not the rest must not reconcile.
//
// Reported here rather than only in the engine's own tests because the damage
// happens HERE: an incomplete lane marked complete deletes the mail posture it
// simply did not read this time.
func TestAPartialDNSReadIsNotAuthoritative(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	orgID := seedTechnicalOrg(ctx, t, e, "Teilweise GmbH", "teilweise.de")

	full := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      []TechnicalLane{LaneDNS},
		Observations: []TechnicalObservation{
			observation(FactMailProvider, "microsoft365", "Microsoft 365"),
			observation(FactEmailSecurity, "dmarc_reject", "DMARC durchgesetzt"),
		},
		ObservedAt: technicalObservedAt,
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, full, nil); err != nil {
		t.Fatalf("apply the full reading: %v", err)
	}

	// The engine reports the lane as NOT completed when a sub-lookup fails, so
	// the apply reconciles nothing at all.
	partial := TechnicalEnrichment{
		OrganizationID: orgID,
		Completed:      nil,
		ObservedAt:     technicalObservedAt.Add(time.Hour),
	}
	if err := e.store.ApplyTechnicalEnrichment(ctx, partial, nil); err != nil {
		t.Fatalf("apply the partial reading: %v", err)
	}

	held := technicalFactsOf(ctx, t, e, orgID)
	if len(held[FactEmailSecurity]) != 1 || len(held[FactMailProvider]) != 1 {
		t.Errorf("a read that could not complete removed what it did not see: %v", held)
	}
}
