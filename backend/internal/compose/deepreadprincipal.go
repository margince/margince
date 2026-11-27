// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who a deep read acts as. The dossier row is the authority on who asked, not
// the job payload that travelled beside it. A read a colleague asked for writes
// its findings as that colleague's agent, under their live grants. A read
// Margince started itself runs as system:deep_read. Either way the dossier
// row's own bookkeeping is system:deep_read.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// run is the whole deep read, River-agnostic so tests drive it directly.
// Retry semantics rest on BeginSiteRead's CAS: any terminal outcome
// (done, partial, failed, cancelled) leaves the dossier past "queued", so a River
// retry — including one after a recorded failure — CAS-misses and
// no-ops. One honest outcome per dossier, no zombie re-crawls; reading
// the site again is a human's next start, never an automatic retry.
// deepReadWorkerCtx attaches the worker's principal, workspace and correlation
// onto the job context — the values every store write (and terminalCtx) needs.
//
// The requester it carries comes from the job payload, because the claim that
// yields the authoritative one needs a context to be made at all. It is
// replaced by withClaimedRequester the moment the row is in hand, so nothing
// the worker writes is attributed on the payload's word.
func deepReadWorkerCtx(ctx context.Context, args SiteDeepReadArgs) context.Context {
	return withClaimedRequester(
		principal.WithWorkspaceID(ctx, args.Workspace), args.RequestedBy, args.SiteReadID)
}

// withClaimedRequester stamps the principal every store write is attributed to.
// The human named here owns what this read creates, so it must be the one the
// DOSSIER ROW records: a payload that disagreed would hang another contact's
// name on the rows, which no later gate would catch — provenance is written
// once and never re-derived.
func withClaimedRequester(ctx context.Context, requestedBy string, readID ids.UUID) context.Context {
	requester := requestedByUserID(requestedBy)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type:       principal.PrincipalSystem,
		ID:         deepReadSystemID,
		UserID:     requester,
		OnBehalfOf: requester,
	})
	if readID.IsZero() {
		return principal.WithCorrelationID(ctx, ids.NewV7())
	}
	return principal.WithCorrelationID(ctx, readID)
}

// deepReader is a deep read's two principals. The agent reads the site and
// writes the findings for the colleague who asked. The system keeps the dossier
// row, and runs a read Margince started itself.
var deepReader = colleagueJob{work: "agent:deepread", books: deepReadSystemID}

// deepReadSystemID keeps every dossier row and runs the reads Margince starts.
const deepReadSystemID = "system:deep_read"

// readPrincipals binds the claimed read's work onto ctx, with its bookkeeping
// carried beside it (runBooks). A read nobody asked for, or one whose
// requester names no colleague, is the system's throughout. A requester who left
// or lost their seat answers workRefused.
func readPrincipals(ctx context.Context, users *identity.Service, requestedBy string, readID ids.UUID) (context.Context, error) {
	books := withClaimedRequester(ctx, requestedBy, readID)
	requester := requestedByUserID(requestedBy)
	if isSystemRead(requestedBy) || requester.IsZero() {
		return books, nil
	}
	_, work, err := forColleague(books, users, requester, deepReader)
	if err != nil {
		return books, err
	}
	return withRunBooks(work, books), nil
}

// requestedByUserID recovers the human uuid behind a "human:<uuid>"
// requested_by so the staged proposal carries OnBehalfOf. A requester
// without a recoverable uuid yields the zero uuid — the approval's
// on_behalf_of is then honestly NULL rather than the read failing over
// provenance.
func requestedByUserID(requestedBy string) ids.UUID {
	// Only a HUMAN requester can be a human owner, which is exactly what
	// principal.HumanUserID answers: a system namespace naming a uuid is not a
	// contact, and attributing it to one is the provenance mistake this whole
	// path exists to avoid.
	id, ok := principal.HumanUserID(requestedBy)
	if !ok {
		return ids.UUID{}
	}
	return id
}
