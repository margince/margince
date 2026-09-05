// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The dossier reporting itself to the AI-activity projection.
//
// A website read is a durable carrier: it owns a row that is queued before any
// worker sees it, claimed, sometimes handed back, and closed from its own
// outcome — so it can say `queued` and `running`, which the router that
// announces the crawl's individual model calls never can. Every status write of
// site_read funnels through emitSiteReadActivity, and the payload is derived
// from the row the write RETURNED, so no call site can announce a state it did
// not just commit.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SiteReadActivitySource names this carrier to the projection. Identity, not
// display: two sources must never collide on one occurrence key.
const SiteReadActivitySource = "site_read"

// SiteReadActivityKind is the display kind a website read reports under — the
// AiActivityKind the contract carries and the rail keys its copy on. Exported
// so the root-package parity gate can hold the contract's enum to what this
// carrier announces. Not an api/ai-tasks.yaml task: a read runs several model
// tasks and is an occurrence of none of them, so the payload names no ai_task.
const SiteReadActivityKind = "site_read"

// siteReadQueuedLease is how long a queued read stays believable before the
// projection reports it stalled. Two deep-read workers per process each hold a
// crawl for up to the job's own wall, so a read waiting behind them is ordinary
// for a while and suspicious after this — the same judgement the extraction
// lease makes for a document. A claimed read carries the reclaim window its
// claim was made with instead, because that is the instant a replacement may
// take it.
const siteReadQueuedLease = 20 * time.Minute

// The contract's caps on the two free-text fields this carrier sends, applied
// before the wire: the projection stores what it is handed.
const (
	siteReadDegradeReasonBound = 200
	siteReadSubjectLabelBound  = 120
)

// claim is what BeginSiteRead hands the worker, read off the claimed row.
func (sr SiteRead) claim() SiteReadClaim {
	var claimedAt time.Time
	if sr.StartedAt != nil {
		claimedAt = *sr.StartedAt
	}
	var orgID *ids.UUID
	if sr.OrganizationID != nil {
		id := sr.OrganizationID.UUID
		orgID = &id
	}
	return SiteReadClaim{
		OrganizationID: orgID, TargetKind: sr.TargetKind, SeedURL: sr.SeedURL,
		RequestedBy: sr.RequestedBy, ClaimedAt: claimedAt,
	}
}

// activityStateDegraded is the projection's word for a settled occurrence that
// kept partial state. It is the one answer below that no site_read status
// spells, so it is named rather than repeated.
const activityStateDegraded = "degraded"

// The dossier's own statuses in the projection's vocabulary.
//
// `degraded` is not a synonym for "not done": the projection's fault arm is
// bounded on it, and the rail holds the newest unacknowledged one on the orb
// until a reader opens the panel. So the question each arm answers is whether
// a person has to be TOLD, and `partial` splits on exactly that. A crawl that
// ran into one of the ceilings this product sets for it — the page cap, the
// byte cap, the wall clock — did what it was configured to do: it read the
// site down to the budget it was given and staged everything it found, and the
// dossier carries where it stopped. A crawl whose extraction lost a lane, or
// whose workspace ran out of AI credit, is short of what it was ASKED for.
// Only the second is somebody's to act on; the first is true of almost every
// ordinary company site, whose pages outnumber the cap, and an orb amber on
// every one of those is a signal nobody reads any more.
//
// deferred settles rather than staying live: the read is not being worked, the
// budget it waits on may return hours later, and an orb lit that whole time
// would report a crawl that is not happening. It reports `degraded` because it
// kept what it had read, and the next claim reopens the occurrence under a new
// attempt — which is exactly the reopening the projection's guard admits. A
// retryable failure is the same shape: `failed` now, reopened if claimed again.
// cancelled is a read withdrawn by a decision rather than a fault, and it
// reports `failed` because the vocabulary has no third settled word for "did
// not happen" — the reason says which it was.
func (sr SiteRead) activityState() string {
	switch sr.Status {
	case siteReadStatusQueued, siteReadStatusRunning, siteReadStatusDone, siteReadStatusFailed:
		return sr.Status
	case siteReadStatusPartial:
		if sr.stoppedAtOwnCeiling() {
			return siteReadStatusDone
		}
		return activityStateDegraded
	case siteReadStatusDeferred:
		return activityStateDegraded
	case siteReadStatusCancelled:
		return siteReadStatusFailed
	}
	return sr.Status
}

// stoppedAtOwnCeiling answers whether this partial is a crawl that filled its
// budget and nothing else.
//
// Both halves are required, and the second is the one that is easy to forget:
// a run can hit the page cap AND lose an extraction lane, and the row spells
// that with the cap's own stop reason, indistinguishable from a clean one.
// SiteReadPartialExtractionWarning is what the worker writes when a lane died,
// so its presence is what keeps that read out of the healthy answer.
func (sr SiteRead) stoppedAtOwnCeiling() bool {
	if sr.StoppedReason == nil || !siteReadOwnCeiling[*sr.StoppedReason] {
		return false
	}
	return !slices.Contains(sr.Warnings, SiteReadPartialExtractionWarning)
}

// siteReadOwnCeiling is the half of the stop vocabulary that names a bound
// this product chose. `budget` is deliberately absent: the workspace running
// out of AI credit is a condition an operator repairs, not a ceiling the crawl
// was built to fill.
var siteReadOwnCeiling = map[string]bool{
	siteReadStopPageCap:  true,
	siteReadStopByteCap:  true,
	siteReadStopDeadline: true,
}

// The stop reasons as prose. Server-authored and closed — never a provider's
// words — which is the condition on reaching a rail an ordinary rep reads.
var siteReadStopSaid = map[string]string{
	siteReadStopBudget:   "The read stopped at the AI budget's limit.",
	siteReadStopPageCap:  "The read stopped at its page limit.",
	siteReadStopByteCap:  "The read stopped at its size limit.",
	siteReadStopDeadline: "The read stopped at its time limit.",
}

const (
	siteReadPartialSaid   = "The read stopped before the whole site was read."
	siteReadCancelledSaid = "The read was withdrawn before it ran."
)

// activityDegradeReason is the one sentence a settled-short read carries. Every
// value is authored in this tree: status_detail comes from the closed failure
// classifier or the budget deferral, so no site's and no model's words reach
// the rail through it.
func (sr SiteRead) activityDegradeReason() string {
	var said string
	switch sr.Status {
	case siteReadStatusFailed, siteReadStatusDeferred:
		if sr.StatusDetail != nil {
			said = *sr.StatusDetail
		}
	case siteReadStatusPartial:
		said = siteReadPartialSaid
		if sr.StoppedReason != nil {
			if stop, known := siteReadStopSaid[*sr.StoppedReason]; known {
				said = stop
			}
		}
	case siteReadStatusCancelled:
		said = siteReadCancelledSaid
	}
	return boundedRunes(said, siteReadDegradeReasonBound)
}

// activitySettledAt is when this attempt stopped being live. finished_at is stamped by
// the terminal writes only; a deferral leaves it NULL because the read is not
// over, yet the attempt IS, and the projection requires a settled state to say
// when — so the deferral's own write instant stands in.
func (sr SiteRead) activitySettledAt() *time.Time {
	if sr.FinishedAt != nil {
		return sr.FinishedAt
	}
	if state := sr.activityState(); state == siteReadStatusQueued || state == siteReadStatusRunning {
		return nil
	}
	settled := sr.UpdatedAt
	return &settled
}

// emitSiteReadActivity publishes one dossier's current state against an
// existing ledger row.
//
// lease is how long a LIVE state stays believable and is ignored for a settled
// one: a closed occurrence is not claiming to work, so it has nothing to go
// stale.
func emitSiteReadActivity(ctx context.Context, tx pgx.Tx, ledgerID ids.UUID, sr SiteRead, lease time.Duration) error {
	payload := crmcontracts.InternalEventAiTaskStateChanged{
		Source: SiteReadActivitySource,
		// The dossier's own id. One company is read many times over its life
		// and each read is its own occurrence, so the key is the read.
		OccurrenceKey: sr.ID.String(),
		Kind:          SiteReadActivityKind,
		Attempt:       sr.Attempt,
		State:         sr.activityState(),
		// The instant THIS attempt became current, not the read's creation: a
		// live row ages from here, and a read claimed again hours later would
		// otherwise be past its lease before the worker fetched a page.
		QueuedAt:   sr.AttemptAt,
		StartedAt:  sr.StartedAt,
		FinishedAt: sr.activitySettledAt(),
	}
	if lease > 0 && payload.FinishedAt == nil {
		seconds := int(lease.Seconds())
		payload.LeaseSeconds = &seconds
	}
	if reason := sr.activityDegradeReason(); reason != "" {
		payload.DegradeReason = &reason
	}
	if sr.OrganizationID != nil {
		label, err := siteReadSubjectLabel(ctx, tx, sr.OrganizationID.UUID)
		if err != nil {
			return err
		}
		subject := openapi_types.UUID(sr.OrganizationID.UUID)
		subjectType := entityOrganization
		payload.SubjectType = &subjectType
		payload.SubjectId = &subject
		if label != "" {
			payload.SubjectLabel = &label
		}
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, ledgerID, payload); err != nil {
		return fmt.Errorf("publish website read activity: %w", err)
	}
	return nil
}

// siteReadSubjectLabel is what the read is ABOUT, named the way the product
// titles the company everywhere else, so the rail and the record never call one
// company two names. A read whose company is gone — an erasure can take the
// organization while its read is still settling — goes out unnamed, and the
// rail draws its generic sentence.
func siteReadSubjectLabel(ctx context.Context, tx pgx.Tx, orgID ids.UUID) (string, error) {
	var label string
	err := tx.QueryRow(ctx, `SELECT left(display_name, $2) FROM organization WHERE id = $1`,
		orgID, siteReadSubjectLabelBound).Scan(&label)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read website read subject label: %w", err)
	}
	return label, nil
}

// logSiteReadActivity writes the ledger row a transition needs when it has no
// audit row of its own — a claim, a deferral, a close — and announces against
// it. Every event carries a ledger trace link, and the system_log row is what
// keeps a worker's transition attributable without an entity ref.
func logSiteReadActivity(ctx context.Context, tx pgx.Tx, sr SiteRead, lease time.Duration) error {
	ledgerID, err := storekit.LogSystem(ctx, tx, "ai_task.state_changed", map[string]any{
		auditKeySource: SiteReadActivitySource, "occurrence_key": sr.ID.String(),
		"state": sr.Status, "attempt": sr.Attempt,
	})
	if err != nil {
		return fmt.Errorf("log website read state change: %w", err)
	}
	return emitSiteReadActivity(ctx, tx, ledgerID, sr, lease)
}

// boundedRunes cuts a sentence to the contract's cap without splitting a
// character, the way the read side's left() would.
func boundedRunes(s string, bound int) string {
	runes := []rune(s)
	if len(runes) <= bound {
		return s
	}
	return string(runes[:bound])
}
