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

// colleagueWork is one piece of work on a contact. write puts it there inside
// tx through the production writer, and landed reports whether it is there.
type colleagueWork struct {
	write  func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error
	landed func(t *testing.T) bool
}

// colleagueWorkKinds prepares each kind of work the purge's recheck reads.
var colleagueWorkKinds = map[string]func(t *testing.T, e *integration.Env, contact ids.UUID) colleagueWork{
	"a tag": func(t *testing.T, e *integration.Env, contact ids.UUID) colleagueWork {
		store := collections.NewStore(e.DB())
		tag, err := store.CreateTag(ownerCtx(e), "Mandanten", nil, nil)
		if err != nil {
			t.Fatalf("creating the tag: %v", err)
		}
		return colleagueWork{
			write: func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
				_, err := store.ApplyTagTx(ctx, tx, tag.ID, "contact", contact)
				return err
			},
			landed: func(t *testing.T) bool {
				return holds(t, e, `SELECT EXISTS (SELECT 1 FROM taggable WHERE tag_id = $1 AND entity_id = $2)`, tag.ID, contact)
			},
		}
	},
	"a list membership": func(t *testing.T, e *integration.Env, contact ids.UUID) colleagueWork {
		store := collections.NewStore(e.DB())
		list, err := store.CreateList(ownerCtx(e), collections.CreateListInput{Name: "Kanzleien", EntityType: "contact"})
		if err != nil {
			t.Fatalf("creating the list: %v", err)
		}
		return colleagueWork{
			write: func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
				_, err := store.AddMemberTx(ctx, tx, list.ID,
					collections.MemberChange{EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen})
				return err
			},
			landed: func(t *testing.T) bool {
				return holds(t, e, `SELECT EXISTS (SELECT 1 FROM list_member WHERE list_id = $1 AND entity_id = $2)`, list.ID, contact)
			},
		}
	},
	"a note": func(t *testing.T, e *integration.Env, contact ids.UUID) colleagueWork {
		return colleagueWork{
			write: func(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
				body := "Rückruf vereinbart"
				_, _, err := e.Activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
					Kind: "note", Body: &body, Source: "manual",
					Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
				})
				return err
			},
			landed: func(t *testing.T) bool {
				return holds(t, e, `SELECT EXISTS (SELECT 1 FROM activity_link l JOIN activity a ON a.id = l.activity_id
					 WHERE l.contact_id = $1 AND a.kind = 'note')`, contact)
			},
		}
	},
	// The send's own row absorbs the provider's captured copy, which capture
	// filed under the contact, and takes over its links.
	"a send absorbing its captured copy": func(t *testing.T, e *integration.Env, contact ids.UUID) colleagueWork {
		const staged, stamped, recipient = "staged-1@margince.example", "stamped-1@mail.gmail.example", "versand@elsewhere.example"
		send := seedRow(t, e, `
			INSERT INTO activity (kind, subject, body, direction, source_system, source_id, source, captured_by,
			                      thread_key, counterparty_email)
			VALUES ('email', 'Angebot', 'Anbei.', 'outbound', 'email', $1, 'manual', $2, $1, $3) RETURNING id`,
			staged, "human:"+e.Rep1.String(), recipient)
		echo := seedRow(t, e, `
			INSERT INTO activity (kind, subject, body, direction, source_system, source_id, source, captured_by,
			                      thread_key, counterparty_email)
			VALUES ('email', 'Angebot', 'Anbei.', 'outbound', 'email', $1, 'gmail:'||$1, 'connector:gmail', $1, $2)
			RETURNING id`, stamped, recipient)
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, echo, contact)
		return colleagueWork{
			write: func(ctx context.Context, tx pgx.Tx, _ ids.UUID) error {
				return e.Activities.ReconcileMessageIdentityTx(ctx, tx, ids.From[ids.ActivityKind](send), staged, stamped)
			},
			landed: func(t *testing.T) bool {
				return holds(t, e, `SELECT EXISTS (SELECT 1 FROM activity_link WHERE activity_id = $1 AND contact_id = $2)`,
					send, contact)
			},
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
			work := prepare(t, e, minted)
			rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, purgeDomain)

			var outcome PurgeOutcome
			purged := make(chan error, 1)
			ctx := ownerCtx(e)
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				if err := work.write(ctx, tx, minted); err != nil {
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
			if _, archived := contactNameAndArchived(t, e, minted); archived || !work.landed(t) {
				t.Fatalf("archived=%v and %s landed=%v, want a live contact that keeps it", archived, name, work.landed(t))
			}
		})
	}
}

// The purge has decided and is scrubbing when the colleague writes. The write
// waits for the scrub, then finds the contact gone: it is refused, or it leaves
// the contact out.
func TestWorkWrittenDuringASenderPurgesScrubWaitsAndMissesTheContact(t *testing.T) {
	for name, prepare := range colleagueWorkKinds {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			connectRep1Mailbox(t, e)
			minted := mintThroughCapture(t, e, "scrub")
			work := prepare(t, e, minted)
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
						return work.write(wctx, wtx, id)
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
			if err := within(t, written); err != nil && !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("writing %s after the scrub answered %v, want success or not found", name, err)
			}
			if work.landed(t) {
				t.Fatalf("%s landed on a contact the purge had already anonymised", name)
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

func seedRow(t *testing.T, e *integration.Env, sql string, args ...any) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), sql, args...).Scan(&id)
	}); err != nil {
		t.Fatalf("seeding a row: %v", err)
	}
	return id
}

func holds(t *testing.T, e *integration.Env, sql string, args ...any) bool {
	t.Helper()
	var yes bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), sql, args...).Scan(&yes)
	}); err != nil {
		t.Fatalf("reading what landed on the contact: %v", err)
	}
	return yes
}
