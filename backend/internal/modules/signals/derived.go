// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

// Signals a producer derived rather than a human filed.
//
// The table has carried a `derived` source_channel since 0047 and nothing ever
// wrote one: the only entry point was POST /signals, which is human-only. This
// is the writer, and it stays in THIS module because the module owns the table
// — a producer computes WHICH accounts to raise, and hands the finding here to
// be written.
//
// A derived signal is an OBSERVATION. It is written directly, attributed to the
// producer that drew it, and carries the evidence a reader can open. What
// follows from one — a lifecycle change, a deal, a task — is a structural claim
// and stages for a human instead.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// resolutionResolved is a derived signal's resolution state: a producer that
// computed a finding about a KNOWN account has already resolved it — the
// resolver exists for raw signals that arrive naming nobody.
const resolutionResolved = "resolved"

// DerivedSignal is one finding a producer computed, in the words the reader
// will see. The producer owns the RULE; this module owns the row.
type DerivedSignal struct {
	Kind string
	// CompanyID is the account the finding is attributed to
	// (resolved_company_id): the company page's signal strip and the account's
	// signal list read a signal through it, whatever the subject below.
	CompanyID ids.UUID
	// ProjectID, when set, makes the PROJECT the signal's subject rather than
	// the account itself — a finding about one body of work at the company,
	// which still belongs to the company's page because the project does.
	ProjectID ids.UUID
	Summary   string
	// SummaryParams are the parts Summary was worded from, so a reader in
	// another language reads it in theirs. Nil when it was not worded from parts.
	SummaryParams json.RawMessage
	Severity      string
	// Fingerprint identifies the finding by what it fired ON, so a producer
	// that runs hourly raises nothing new on an unchanged account and a
	// dismissal survives every later pass.
	Fingerprint string
	// Evidence is the records the finding was drawn from, each openable.
	Evidence []DerivedEvidence
	// Audit carries whatever the producer wants recorded about the derivation
	// beyond the row itself.
	Audit map[string]any
	// Channel is WHERE the finding came from, in the signal table's own
	// vocabulary. Empty means `derived` — a finding computed from what this
	// workspace already holds, which is what every producer was until a
	// company's own newsroom became one.
	Channel string
	// Source names the producer for the audit trail. Empty means the signal
	// scan, for the same reason.
	Source string
	// PrivateTo is the one reader this finding answers to, or the zero value
	// when the whole workspace may read it.
	//
	// A finding computed from metadata — who spoke last, how long ago — states
	// nothing the account's own readers could not count for themselves, and is
	// shared. A finding drawn from what a message SAYS is only as shareable as
	// that message: capture files mail against contacts it auto-creates
	// owner-private, and a summary of a private message discloses the message.
	// The producer knows which of the two it made; this is where it says so.
	PrivateTo ids.UUID
}

// visibilityOwner marks a signal its owner alone may read, matching the value
// contact and company carry for the same reason.
const visibilityOwner = "owner"

// visibilityWorkspace marks a signal every seat that can see its subject may
// read, which is what a signal has always been.
const visibilityWorkspace = "workspace"

// nullableOwner renders the zero id as SQL NULL, so a shared signal names no
// owner rather than naming the nil one.
func nullableOwner(owner ids.UUID) *ids.UUID {
	if owner == (ids.UUID{}) {
		return nil
	}
	return &owner
}

// DerivedEvidence is one citable record behind a derived signal.
//
// The two source shapes are exclusive and both are cited the same way: an
// ACTIVITY the finding was drawn from, or a PAGE it was read on. A web finding
// carries the second because there is no activity to open — what a reader wants
// is the article, and storing the article's text instead of its address is the
// thing this product does not do.
type DerivedEvidence struct {
	SourceQuote    *values.QuoteProof
	LifecycleScope string
	Workstream     string
	BuyerContactID *ids.UUID
	CommercialRole string
	Snippet        string
	ActivityID     ids.UUID
	SourceURL      string
}

// RecordDerived writes one derived signal inside the caller's transaction, or
// reports that the same finding was already raised.
//
// It returns false rather than an error on a repeat: a producer pass over an
// unchanged account is the normal case, not a failure, and the fingerprint
// index covers dismissed rows so a dismissal is never undone by the next pass.
func RecordDerived(ctx context.Context, tx pgx.Tx, in DerivedSignal, detectedAt time.Time) (bool, error) {
	if !in.PrivateTo.IsZero() {
		owner, found, err := auth.ConsolidatedMemberTx(ctx, tx, in.PrivateTo)
		if err != nil {
			return false, err
		}
		if !found {
			return false, nil
		}
		in.PrivateTo = owner
	}
	evidence, err := json.Marshal(derivedEvidenceRows(in.Evidence))
	if err != nil {
		return false, fmt.Errorf("encode derived signal evidence: %w", err)
	}
	visibility := visibilityWorkspace
	if in.PrivateTo != (ids.UUID{}) {
		visibility = visibilityOwner
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	subjectType, subjectID := in.subject()
	channel, source := in.channelAndSource()
	// Every placeholder is derived from the argument slice, so a column added
	// to this statement cannot bind to its neighbour's value.
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	var signalID ids.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO signal
		  (kind, entity_type, entity_id, resolved_company_id, summary, summary_params,
		   evidence, fingerprint, source_channel, resolution_state, severity,
		   status, detected_at, source, captured_by, visibility, owner_id)
		VALUES (`+arg(in.Kind)+`, `+arg(subjectType)+`, `+arg(subjectID)+`, `+arg(in.CompanyID)+`, `+arg(in.Summary)+`, `+arg(in.SummaryParams)+`,
		        `+arg(evidence)+`, `+arg(in.Fingerprint)+`, `+arg(channel)+`, 'resolved', `+arg(in.Severity)+`,
		        'open', `+arg(detectedAt)+`, `+arg(source)+`, `+arg(by)+`, `+arg(visibility)+`, `+arg(nullableOwner(in.PrivateTo))+`)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		args...).Scan(&signalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("write derived signal: %w", err)
	}

	// The write shape, in the caller's one transaction. The audit names the
	// SIGNAL: the account is its subject, not the record that changed.
	auditID, err := storekit.Audit(ctx, tx, "create", "signal", signalID, nil, in.Audit)
	if err != nil {
		return false, fmt.Errorf("audit derived signal: %w", err)
	}
	eventSubjectID := openapi_types.UUID(subjectID)
	if err := storekit.EmitEvent(ctx, tx, auditID, signalID, crmcontracts.PublicEventSignalDetected{
		SignalId:        openapi_types.UUID(signalID),
		Kind:            in.Kind,
		SourceChannel:   channel,
		ResolutionState: resolutionResolved,
		Severity:        in.Severity,
		// The signal's SUBJECT. The envelope's own entity is the signal.
		SubjectEntityType: &subjectType,
		SubjectEntityId:   &eventSubjectID,
	}); err != nil {
		return false, fmt.Errorf("emit signal.detected: %w", err)
	}
	return true, nil
}

// channelAndSource fills in what a producer left unsaid. The defaults are what
// every producer meant before there was a second kind: a finding computed from
// this workspace's own records, by the signal scan.
func (in DerivedSignal) channelAndSource() (channel, source string) {
	channel, source = in.Channel, in.Source
	if channel == "" {
		channel = "derived"
	}
	if source == "" {
		source = scanSource
	}
	return channel, source
}

// subject is the (entity_type, entity_id) pair the row carries: the project
// when the finding is about one, the account otherwise.
func (in DerivedSignal) subject() (string, ids.UUID) {
	if in.ProjectID != (ids.UUID{}) {
		return "project", in.ProjectID
	}
	return "company", in.CompanyID
}

// derivedEvidenceRows renders evidence in the per-claim shape SIG-DDL-1 fixes,
// so a derived signal's evidence reads the same as a human-filed one's.
func derivedEvidenceRows(in []DerivedEvidence) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, cited := range in {
		row := map[string]any{"snippet": cited.Snippet}
		if cited.SourceQuote != nil {
			row["source_quote"] = cited.SourceQuote
		}
		if cited.LifecycleScope != "" {
			row["lifecycle_scope"] = cited.LifecycleScope
		}
		if cited.Workstream != "" {
			row["workstream"] = cited.Workstream
		}
		if cited.BuyerContactID != nil {
			row["buyer_contact_id"] = *cited.BuyerContactID
		}
		if cited.CommercialRole != "" {
			row["commercial_role"] = cited.CommercialRole
		}
		if cited.SourceURL != "" {
			// `page` is the contract's own word for a cited web page
			// (SignalEvidence.source_type). Inventing a second one here would
			// write rows the published enum refuses.
			row["source_type"] = "page"
			row["source_id"] = cited.SourceURL
		} else {
			row["source_type"] = "activity"
			row["source_id"] = cited.ActivityID.String()
		}
		out = append(out, row)
	}
	return out
}

// columnStatus and statusOpen are the one spelling of the triage column and
// the state this transition leaves: the CAS predicate and the audit's
// before-image must agree, and two literals would drift.
const (
	columnStatus = "status"
	statusOpen   = "open"
)

// The deciders AcknowledgeTx records on the outcome row.
const (
	ResolutionSourceApproval  = "approval"
	ResolutionSourceDealScout = "deal_scout"
	// ResolutionSourceCommitmentRule settles a commitment signal written before
	// commitments had a rule, once its message has been read through the rule.
	ResolutionSourceCommitmentRule = "commitment_rule"
)

// DismissedTx reports whether a human dismissed the live signal a fingerprint
// names. A producer that does more than file a signal asks it, so a finding a
// human threw away is not acted on again under another name. The live rows are
// share-locked, so a dismissal racing the producer waits for it to commit.
func DismissedTx(ctx context.Context, tx pgx.Tx, fingerprint string) (bool, error) {
	var dismissed bool
	if err := tx.QueryRow(ctx, `
		SELECT coalesce(bool_or(status = 'dismissed'), false)
		  FROM (SELECT status FROM signal
		         WHERE fingerprint = $1 AND archived_at IS NULL
		         FOR SHARE) live`,
		fingerprint).Scan(&dismissed); err != nil {
		return false, fmt.Errorf("read whether the signal was dismissed: %w", err)
	}
	return dismissed, nil
}

// AcknowledgeTx shares the consequence's transaction and preserves an earlier human decision.
func AcknowledgeTx(ctx context.Context, tx pgx.Tx, signalID ids.UUID, source string) (bool, error) {
	return settleSignalTx(ctx, tx, signalID, source, "acknowledged")
}

// DismissTx preserves earlier decisions while settling the authorized signal.
func DismissTx(ctx context.Context, tx pgx.Tx, signalID ids.UUID, source string) (bool, error) {
	if err := auth.Require(ctx, "signal", principal.ActionUpdate); err != nil {
		return false, err
	}
	if err := auth.EnsureSignalVisibleLive(ctx, tx, signalID); err != nil {
		return false, err
	}
	return settleSignalTx(ctx, tx, signalID, source, "dismissed")
}

func settleSignalTx(ctx context.Context, tx pgx.Tx, signalID ids.UUID, source, outcome string) (bool, error) {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return false, err
	}
	// The status predicate IS the CAS: a signal a human already triaged has
	// left 'open' behind, the update matches nothing, and the false below says
	// their judgement stands.
	var args []any
	arg := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	tag, err := tx.Exec(ctx, `
 UPDATE signal SET status = `+arg(outcome)+` WHERE id = `+arg(signalID)+` AND status = 'open' AND archived_at IS NULL`, args...)
	if err != nil {
		return false, fmt.Errorf("settle the signal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// The append-only human-outcome row (data-model §12.5), the same one the
	// triage endpoint writes — the outcome is a human's, whoever's hands the
	// write passed through.
	args = nil
	if _, err := tx.Exec(ctx,
		`INSERT INTO signal_resolution (id, signal_id, outcome, resolved_by, source, captured_by)
 VALUES (`+arg(ids.NewV7())+`, `+arg(signalID)+`, `+arg(outcome)+`, `+arg(storekit.UUIDOrNil(actor.UserID))+`, `+arg(source)+`, `+arg(actor.ID)+`)`, args...); err != nil {
		return false, fmt.Errorf("append the signal outcome: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "update", "signal", signalID,
		map[string]any{columnStatus: statusOpen},
		map[string]any{columnStatus: outcome}); err != nil {
		return false, fmt.Errorf("audit the signal decision: %w", err)
	}
	return true, nil
}

// AcknowledgeCoveredForCompanyTx preserves signals outside the decision's evidence set.
// The decider bounds visibility; ctx carries the writer's provenance.
func AcknowledgeCoveredForCompanyTx(
	ctx context.Context, decider context.Context, tx pgx.Tx, companyID ids.UUID, kind string, covered []ids.UUID,
) (int, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	companyPos, kindPos, coveredPos := arg(companyID), arg(kind), arg(covered)
	scope, err := auth.SignalScopeClause(decider, "signal", arg)
	if err != nil {
		return 0, err
	}
	if scope == "" {
		// Unbounded on every subject type, so there is nothing to narrow.
		scope = "true"
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id FROM signal
		 WHERE resolved_company_id = $%d AND kind = $%d AND id = ANY($%d::uuid[])
		   AND status = 'open' AND archived_at IS NULL
		   AND %s
		 ORDER BY detected_at`, companyPos, kindPos, coveredPos, scope), args...)
	if err != nil {
		return 0, fmt.Errorf("list the account's open signals: %w", err)
	}
	named, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("read the account's open signals: %w", err)
	}
	settled := 0
	for _, signalID := range named {
		// Each through the same CAS the single-signal path takes, so a row a
		// human triaged between the read and this write keeps their outcome.
		moved, err := AcknowledgeTx(ctx, tx, signalID, ResolutionSourceApproval)
		if err != nil {
			return settled, err
		}
		if moved {
			settled++
		}
	}
	return settled, nil
}

// CitesActivity reports whether any live signal was raised citing this message.
//
// The question the pipeline trace asks after a conversation has been read: the
// reading either produced a finding that names this message or it did not, and
// the two are different answers to a member.
//
// Evidence is jsonb and the containment test is over one element, so a signal
// citing several messages answers true for each of them. Archived signals are
// excluded: a finding somebody removed is not one the message produced any
// more, and reporting it would explain a card the reader cannot open.
//
// It takes the caller's transaction rather than opening one, because the trace
// assembles several reads into one answer and a second connection would let
// this one see a state the others did not.
func CitesActivity(ctx context.Context, tx pgx.Tx, activityID ids.UUID) (bool, error) {
	var cited bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM signal
		   WHERE archived_at IS NULL
		     AND evidence @> jsonb_build_array(
		           jsonb_build_object('source_type', 'activity', 'source_id', $1::text)))`,
		activityID.String()).Scan(&cited)
	if err != nil {
		return false, fmt.Errorf("signals: reading whether a signal cites this message: %w", err)
	}
	return cited, nil
}
