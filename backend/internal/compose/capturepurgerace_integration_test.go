// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A colleague files a tag, a list membership or a note on a contact while a
// sender purge is deciding about it. Whichever commits first wins: work that
// lands first keeps the contact, and work that comes second finds it gone.
// Never both, which would erase a record somebody had just worked on.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// colleagueWork writes one piece of work on a contact inside tx, through the
// production writer.
type colleagueWork func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error

// colleagueWorkKinds prepares each kind of work the purge's recheck reads.
var colleagueWorkKinds = map[string]func(t *testing.T, e *integration.Env) colleagueWork{
	"a tag": func(t *testing.T, e *integration.Env) colleagueWork {
		store := collections.NewStore(e.DB())
		tag, err := store.CreateTag(ownerCtx(e), "Mandanten", nil, nil)
		if err != nil {
			t.Fatalf("creating the tag: %v", err)
		}
		return func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
			_, err := store.ApplyTagTx(ctx, tx, tag.ID, "contact", contact)
			return err
		}
	},
	"a list membership": func(t *testing.T, e *integration.Env) colleagueWork {
		store := collections.NewStore(e.DB())
		list, err := store.CreateList(ownerCtx(e), collections.CreateListInput{Name: "Kanzleien", EntityType: "contact"})
		if err != nil {
			t.Fatalf("creating the list: %v", err)
		}
		return func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
			_, err := store.AddMemberTx(ctx, tx, list.ID,
				collections.MemberChange{EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen})
			return err
		}
	},
	"a note": func(t *testing.T, e *integration.Env) colleagueWork {
		return func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
			body := "Rückruf vereinbart"
			_, _, err := e.Activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
				Kind: "note", Body: &body, Source: "manual",
				Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
			})
			return err
		}
	},
}

// The colleague's transaction is open, with the work written, when the purge
// reaches the contact. The purge waits for it, sees the work, and keeps the
// contact.
func TestASenderPurgeWaitsForWorkInFlightAndKeepsTheContact(t *testing.T) {
	for name, prepare := range colleagueWorkKinds {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			connectRep1Mailbox(t, e)
			minted := mintThroughCapture(t, e, "inflight")
			work := prepare(t, e)
			rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)

			var outcome PurgeOutcome
			purged := make(chan error, 1)
			ctx := ownerCtx(e)
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				if err := work(ctx, tx, minted); err != nil {
					return err
				}
				go func() {
					var err error
					outcome, err = purgerFor(t, e).Purge(purgeCtx(e, e.Rep1), rule, false)
					purged <- err
				}()
				waitForBackendBlockedBy(t, tx, backendPID(t, tx), purged)
				return nil
			}); err != nil {
				t.Fatalf("writing %s on the contact: %v", name, err)
			}
			if err := within(t, purged); err != nil {
				t.Fatalf("purging: %v", err)
			}
			if outcome.Anonymised != 0 {
				t.Fatalf("anonymised %d, want 0: %s committed before the purge could decide", outcome.Anonymised, name)
			}
			if _, archived := contactNameAndArchived(t, e, minted); archived {
				t.Fatalf("the contact was erased although %s on it committed first", name)
			}
		})
	}
}

// The purge has decided and is scrubbing when the colleague writes. The write
// waits for the scrub, then finds the contact gone and is refused.
func TestWorkWrittenDuringASenderPurgesScrubWaitsAndIsRefused(t *testing.T) {
	for name, prepare := range colleagueWorkKinds {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			connectRep1Mailbox(t, e)
			minted := mintThroughCapture(t, e, "scrub")
			work := prepare(t, e)
			ruleID := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)
			purger := purgerFor(t, e)
			ctx := purgeCtx(e, e.Rep1)
			rule, _, err := purger.purgeableRule(ctx, ruleID, e.Rep1)
			if err != nil {
				t.Fatalf("reading the rule: %v", err)
			}
			chosen, err := purger.purgeableContacts(ctx, e.Rep1, rule)
			if err != nil || len(chosen) != 1 || chosen[0] != minted {
				t.Fatalf("the purge chose %v (err=%v), want the captured contact %s", chosen, err, minted)
			}

			recheck := purger.stillPurgeable(e.Rep1, rule)
			written := make(chan error, 1)
			// The recheck has answered under the contact's lock; the colleague
			// writes now, before the scrub that follows it commits.
			writeDuringScrub := func(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
				ok, err := recheck(ctx, tx, id)
				if err != nil || !ok {
					return ok, err
				}
				go func() {
					wctx := ownerCtx(e)
					written <- database.WithWorkspaceTx(wctx, e.Pool, func(wtx pgx.Tx) error {
						return work(wctx, wtx, id)
					})
				}()
				waitForBackendBlockedBy(t, tx, backendPID(t, tx), written)
				return true, nil
			}
			anonymised, err := purger.carryOut(ctx, capture.PurgeSubject{}, chosen, writeDuringScrub,
				e.Rep1, privacy.PurgeOwnerRule)
			if err != nil || anonymised != 1 {
				t.Fatalf("anonymised %d (err=%v), want 1: nothing had worked on the contact when the purge decided",
					anonymised, err)
			}
			if err := within(t, written); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("writing %s after the scrub answered %v, want not found: the contact is gone", name, err)
			}
		})
	}
}

func backendPID(t *testing.T, tx pgx.Tx) int {
	t.Helper()
	var pid int
	if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the backend pid: %v", err)
	}
	return pid
}
