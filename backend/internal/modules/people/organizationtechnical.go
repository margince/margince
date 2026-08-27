// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// companySourceTechnical is the provenance a technical lookup writes.
//
// Its own value rather than site_read's: a rep asking "how do you know they
// run Microsoft 365?" is owed "because their MX record says so", and a source
// that claimed the website said it would be false. The DDL binds this value to
// an evidence obligation of its own for the same reason.
const companySourceTechnical = "technical_lookup"

// technicalCapturedBy attributes the write to the lookup rather than to the
// person who pressed the button.
//
// The `agent:` prefix is load-bearing, not decoration: the fact upsert's
// precedence guard tests `captured_by NOT LIKE 'human:%'`, so a technical row
// stamped as a human would make itself un-refreshable and would also survive a
// correction it should have yielded to.
const technicalCapturedBy = "agent:technical-lookup"

// TechnicalLane names one public source. The lanes fail independently, which
// is the whole reason they are named: a certificate log being down must never
// be recorded as "this company operates no services".
type TechnicalLane string

// The three public sources, and the whole vocabulary: DNS records, the
// certificate-transparency log, and the company's own homepage.
const (
	LaneDNS      TechnicalLane = "dns"
	LaneCertLog  TechnicalLane = "certlog"
	LaneHomepage TechnicalLane = "homepage"
)

// technicalLanes is every lane, so a reader asking "has this company been
// fully looked at?" counts against the real set rather than a number written
// down twice. Derived from laneFields, which is where a new lane is added.
//
// Held by: TestEveryTechnicalLaneIsDerivedFromItsFields (backend/gates/technicaldomain_test.go)
var technicalLanes = func() []TechnicalLane {
	lanes := make([]TechnicalLane, 0, len(laneFields))
	for lane := range laneFields {
		lanes = append(lanes, lane)
	}
	sort.Slice(lanes, func(i, j int) bool { return lanes[i] < lanes[j] })
	return lanes
}()

// laneFields names the fact fields each lane is authoritative for. A lane that
// completed replaces exactly these; a lane that failed touches none of them.
var laneFields = map[TechnicalLane][]string{
	LaneDNS:      {FactMailProvider, FactEmailSecurity, FactHostingProvider},
	LaneCertLog:  {FactOperatedService},
	LaneHomepage: {FactTechnology},
}

// LaneOwningField reports which public source is authoritative for a fact
// field, and the zero lane when none is.
//
// Derived from laneFields rather than restated, so a lane that gains a field
// cannot end up with a caller still attributing that field to the old one.
func LaneOwningField(field string) TechnicalLane {
	for lane, fields := range laneFields {
		for _, owned := range fields {
			if owned == field {
				return lane
			}
		}
	}
	return ""
}

// TechnicalObservation is one thing a lookup read about a company.
type TechnicalObservation struct {
	Field    string
	ValueKey string
	Value    string
	// Evidence is the public record that proves it — the MX host, the
	// certificate hostname, the matched marker.
	Evidence string
	// SourceURL names where it was read: `dns:example.de`, the certificate
	// log query, or the homepage URL.
	SourceURL string
}

// TechnicalEnrichment is one completed run over one company.
type TechnicalEnrichment struct {
	OrganizationID ids.OrganizationID
	// Completed names the lanes that finished. ONLY these reconcile: a lane
	// absent here keeps whatever it wrote last time, because "the log did not
	// answer" and "the company has nothing" are different facts and only one
	// of them belongs on the record.
	Completed []TechnicalLane
	// Observations are what the completed lanes read. A completed lane with no
	// observations is an authoritative empty answer and clears its rows.
	Observations []TechnicalObservation
	// ObservedAt stamps retrieved_at — when the sources were actually read.
	ObservedAt time.Time
}

// TechnicalChange is one difference between what the record held and what the
// lookup just read. It is what becomes a company event.
type TechnicalChange struct {
	OrganizationID ids.OrganizationID
	Field          string
	ValueKey       string
	Value          string
	// Previous is what the record held before, empty when this is the first
	// time the field was read.
	Previous string
	// Kind says which way it moved.
	Kind TechnicalChangeKind
	// Evidence is the record that proves the new state.
	Evidence string
}

// TechnicalChangeKind distinguishes the three ways a technical picture moves.
type TechnicalChangeKind string

const (
	// TechnicalAppeared is a signal the company did not publish before — a new
	// careers subdomain, a shop that went live.
	TechnicalAppeared TechnicalChangeKind = "appeared"
	// TechnicalMoved is a single-valued signal whose value changed — mail
	// moving from Google Workspace to Microsoft 365.
	TechnicalMoved TechnicalChangeKind = "moved"
	// TechnicalGone is a signal the company no longer publishes.
	TechnicalGone TechnicalChangeKind = "gone"
)

// TechnicalChangeRecorder is told about each change inside the apply's own
// transaction, so the record and the company event commit together.
//
// It is a function rather than a signals-module call because this module never
// imports a sibling: compose owns the edge and translates a change into
// whatever a signal needs. Implementations MUST use the transaction they are
// given — a second connection would commit a company event for a record change
// that later rolled back.
type TechnicalChangeRecorder func(ctx context.Context, tx pgx.Tx, change TechnicalChange, at time.Time) error

// ApplyTechnicalEnrichment writes what a technical lookup read, reconciles
// each completed lane against what the record held, and reports every change
// to the recorder — all in ONE transaction with one audit row and one
// organization.updated event.
func (s *Store) ApplyTechnicalEnrichment(
	ctx context.Context, in TechnicalEnrichment, record TechnicalChangeRecorder,
) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return s.ApplyTechnicalEnrichmentTx(ctx, tx, in, record)
	})
}

// ApplyTechnicalEnrichmentTx applies through a caller-owned transaction.
//
// Each COMPLETED lane is authoritative for its own fields: rows it no longer
// observes are removed, so a company that moves from Google Workspace to
// Microsoft 365 ends with one mail provider rather than two. A lane that did
// not complete is skipped entirely.
//
// A row a human has claimed is never touched in either direction — not
// refreshed, not removed. The correction outranks the lookup, and a
// reconciliation that deleted it would undo a person's decision on the next
// scheduled pass.
func (s *Store) ApplyTechnicalEnrichmentTx(
	ctx context.Context, tx pgx.Tx, in TechnicalEnrichment, record TechnicalChangeRecorder,
) error {
	if err := auth.Require(ctx, "organization", principal.ActionUpdate); err != nil {
		return err
	}
	if err := validateTechnicalEnrichment(in); err != nil {
		return err
	}
	// The target is a KNOWN row; row-scope is re-checked here so a leaked org
	// id buys nothing (existence-hiding 404). LIVE, because a scheduled run
	// reaches this with no human in the loop to notice an archived company.
	if err := auth.EnsureWritableLive(ctx, tx, "organization", in.OrganizationID.UUID); err != nil {
		return err
	}
	fields := reconciledFields(in.Completed)
	if len(fields) == 0 {
		// Every lane failed. Nothing is authoritative, so nothing changes —
		// and this is not an error: the next pass asks again.
		return nil
	}
	held, err := readTechnicalFacts(ctx, tx, in.OrganizationID, fields)
	if err != nil {
		return err
	}
	written, err := writeTechnicalFacts(ctx, tx, in)
	if err != nil {
		return err
	}
	removed, err := removeUnobservedTechnicalFacts(ctx, tx, in, fields)
	if err != nil {
		return err
	}
	changes := technicalChanges(in, held, removed)
	if record != nil {
		for _, change := range changes {
			if err := record(ctx, tx, change, in.ObservedAt); err != nil {
				return fmt.Errorf("record technical change %s: %w", change.Field, err)
			}
		}
	}
	return s.auditTechnicalEnrichment(ctx, tx, in, written, removed, changes)
}

// auditTechnicalEnrichment commits the audit row and the paired outbox event.
//
// Through the OCCURRENCE door rather than the update door, and the distinction
// is real: what a company publicly runs lands in organization_fact and no
// column of the organization moves, so there is no field whose prior value a
// before-image could name. What was replaced is still recorded, and more
// precisely than an image would — the evidence carries the rows written AND
// the rows the reconciliation removed, which is what makes a mail provider
// moving to Microsoft 365 readable as a move rather than as an arrival.
func (s *Store) auditTechnicalEnrichment(
	ctx context.Context, tx pgx.Tx, in TechnicalEnrichment,
	written, removed []map[string]any, changes []TechnicalChange,
) error {
	lanes := make([]string, 0, len(in.Completed))
	for _, lane := range in.Completed {
		lanes = append(lanes, string(lane))
	}
	delta := map[string]any{
		"written": written, "removed": removed,
		"lanes": lanes, "changes": len(changes),
	}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "organization", in.OrganizationID.UUID,
		nil, map[string]any{
			auditKeySource: companySourceTechnical,
			auditKeyFacts:  written,
			"technical":    delta,
		})
	if err != nil {
		return fmt.Errorf("audit technical enrichment: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, in.OrganizationID.UUID,
		crmcontracts.PublicEventOrganizationUpdated{
			ChangedFields: map[string]any{
				eventKeyDelta:  delta,
				auditKeySource: companySourceTechnical,
			},
		}); err != nil {
		return fmt.Errorf("emit organization.updated: %w", err)
	}
	return nil
}

// validateTechnicalEnrichment refuses an enrichment that cannot be stored,
// here rather than at the DDL, so the caller gets a name instead of a
// constraint violation.
func validateTechnicalEnrichment(in TechnicalEnrichment) error {
	if in.OrganizationID.IsZero() {
		return fmt.Errorf("technical enrichment: no organization")
	}
	if in.ObservedAt.IsZero() {
		return fmt.Errorf("technical enrichment: no observation time")
	}
	authoritative := map[string]bool{}
	for _, field := range reconciledFields(in.Completed) {
		authoritative[field] = true
	}
	for _, observation := range in.Observations {
		switch {
		case observation.ValueKey == "":
			return fmt.Errorf("technical enrichment: %s carries no value key", observation.Field)
		case observation.Evidence == "" || observation.SourceURL == "":
			// The DDL binds this too. Refusing here names the field.
			return fmt.Errorf("technical enrichment: %s names nothing that proves it", observation.Field)
		case !authoritative[observation.Field]:
			// An observation from a lane that did not report completing is a
			// caller bug, and silently writing it would put an unreconciled
			// row on the record forever.
			return fmt.Errorf("technical enrichment: %s was observed by no completed lane", observation.Field)
		}
	}
	return nil
}

// reconciledFields is the set of fact fields the completed lanes own.
func reconciledFields(completed []TechnicalLane) []string {
	var fields []string
	for _, lane := range completed {
		fields = append(fields, laneFields[lane]...)
	}
	return fields
}
