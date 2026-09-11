// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Collecting the logo objects nothing references any more. Every write path in
// the logo lane ends here — the mark a write superseded, the bytes an attempt
// stored for a write that did not happen, the mark a confirmation declined —
// and the one rule that binds them all is that the collection runs DETACHED
// from the context whose expiry most likely caused it.

import (
	"context"
	"log/slog"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reclaimParkedLogo collects the marks a read parked and can no longer hand to
// anybody. The onboarding lane stores its bytes while the page is still in
// hand, long before the confirmation that would adopt them exists; a read that
// ends without a dossier never reaches that confirmation, and the reference on
// the dossier row is the only thing that can still find the object.
//
// Best-effort like the rest of the lane, and for a sharper reason here: the
// read has already failed, and storage is not worth failing it a second time.
// The store answers only with a key no record names, so nothing on this path
// can delete bytes a company wears.
func (w *siteDeepReadWorker) reclaimParkedLogo(ctx context.Context, readID ids.UUID) {
	if w.blob == nil {
		return
	}
	keys, err := w.contacts.DiscardSiteReadLogo(ctx, readID)
	if err != nil {
		w.log.WarnContext(ctx, "dropping the logo parked on a read that ended without a company failed",
			"read", readID.String(), "err", err)
		return
	}
	for _, key := range keys {
		w.reclaimLogoObject(ctx, readID, &key)
	}
}

// reclaimLogoObject deletes an object nothing references any more: the mark a
// successful write superseded, this attempt's own bytes when the write did not
// happen, or the mark a confirmation declined to adopt. Best-effort like the
// rest of the lane — a failure here costs storage, never correctness, so it is
// logged and the caller carries on.
//
// It runs on a DETACHED context, for the same reason finish() does: this is
// the answer to work that has already happened, and the most likely reason to
// be reclaiming at all is that the work ran out of time. Reusing the context
// that just expired would skip exactly the deletes that matter — and an
// object at a per-attempt key that no row ever named is one nothing else can
// find to collect later.
//
// A nil store is a role that holds no objects; it never reaches a key worth
// collecting, because the row-writing calls that report one are guarded by the
// same fact.
//
// `subject` names what the collection belongs to — the dossier a resolve ran
// for, or the company a contact's own write superseded a mark on. It is a
// caller's string rather than a read id because the upload path has no read:
// the detached-context rule above is the invariant, and a second copy of it
// spelled for uploads is how one of the two paths quietly loses it.
func deleteUnreferencedLogo(ctx context.Context, blob blobstore.Store, log *slog.Logger, subject string, key *string) {
	if blob == nil || key == nil || *key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), logoReclaimBudget)
	defer cancel()
	if err := blob.Delete(ctx, *key); err != nil {
		log.WarnContext(ctx, "reclaiming an unreferenced logo object failed",
			"subject", subject, "key", *key, "err", err)
	}
}

// reclaimLogoObject binds the worker's own object store and logger to the
// collection every write path in this lane ends with.
func (w *siteDeepReadWorker) reclaimLogoObject(ctx context.Context, readID ids.UUID, key *string) {
	deleteUnreferencedLogo(ctx, w.blob, w.log, "read "+readID.String(), key)
}
