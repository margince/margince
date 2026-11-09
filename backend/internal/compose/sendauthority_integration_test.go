// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARecipientRoleChangeWaitsForTheSendReservation(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	manager := memberManagerFor(t, a.e)
	a.grantCommitmentRole(t, a.e.Rep1)
	decision := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	reader := a.e.As(a.e.Rep1, nil, integration.AdminPerms)
	basis := readBasis(reader, t, a.e, a.e.Rep1, sendRefs{approvals: []ids.UUID{decision.approval}}, clocktest.Now(t))
	if len(basis.rows) == 0 || len(basis.excluded) != 0 {
		t.Fatal("the decision had no qualifying send basis")
	}
	changed := make(chan error, 1)
	if err := a.e.DB().Tx(reader, func(tx pgx.Tx) error {
		if err := holdSendBasisTx(reader, tx, basis, func() time.Time { return clocktest.Now(t) }); err != nil {
			return err
		}
		go func() {
			changed <- manager.service.ChangeUserRole(a.e.Admin(), recoveryAdminIdentity(a.e), ids.From[ids.UserKind](a.e.Rep1), "read_only")
		}()
		waitForBackendBlockedBy(t, tx, backendPID(t, tx), changed)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
}

func TestACommittedRoleChangeInvalidatesThePreviouslyReadSendBasis(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	manager := memberManagerFor(t, a.e)
	a.grantCommitmentRole(t, a.e.Rep1)
	decision := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	reader := a.e.As(a.e.Rep1, nil, integration.AdminPerms)
	basis := readBasis(reader, t, a.e, a.e.Rep1, sendRefs{approvals: []ids.UUID{decision.approval}}, clocktest.Now(t))
	if len(basis.rows) == 0 || len(basis.excluded) != 0 {
		t.Fatal("the decision had no qualifying send basis")
	}
	admin := recoveryAdminIdentity(a.e)
	role, err := manager.service.CreateRole(a.e.Admin(), admin, "read_only", "Recipient without source access")
	if err != nil {
		t.Fatal(err)
	}
	grant := role.Objects["activity"]
	grant.Read = false
	if _, err := manager.service.SetRoleObjectGrant(a.e.Admin(), admin, role.Key, "activity", grant, nil); err != nil {
		t.Fatal(err)
	}
	if err := manager.service.ChangeUserRole(a.e.Admin(), admin, ids.From[ids.UserKind](a.e.Rep1), role.Key); err != nil {
		t.Fatal(err)
	}
	if err := holdBasis(reader, a.e, basis, clocktest.Now(t)); !errors.Is(err, errSendBasisChanged) {
		t.Fatalf("stale role was still accepted: %v", err)
	}
}

func TestACombinedClaimHoldsAuthorityBeforeCheckingItsBasis(t *testing.T) {
	e := setupWeeklyMail(t)
	manager := memberManagerFor(t, e.Env)
	seedWorkFor(t, e.Env, weeklyMailClock, e.Rep1)
	run, err := routineBriefEngine(e.Pool).SnapshotRun(e.repCtx, weeklyMailClock)
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan error, 1)
	part := morningDigest{lines: digestLines{total: 1}, window: notices.Window{Day: weeklyMailClock}}
	claim, won, err := claimRoutine(e.repCtx, e.Pool, run.ID, ids.Nil, part, weeklyMailClock, func(tx pgx.Tx) error {
		go func() {
			changed <- manager.service.ChangeUserRole(e.Admin(), recoveryAdminIdentity(e.Env), ids.From[ids.UserKind](e.Rep1), "read_only")
		}()
		waitForBackendBlockedBy(t, tx, backendPID(t, tx), changed)
		return nil
	})
	if err != nil || !won || claim.morning.Run.ID != run.ID || !claim.digest {
		t.Fatalf("combined claim: won=%v, err=%v", won, err)
	}
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
}

func TestRoutineClaimsWaitForAuthorityBeforeWritingAnEdition(t *testing.T) {
	for _, edition := range []string{"combined", "morning"} {
		t.Run(edition, func(t *testing.T) {
			e := setupWeeklyMail(t)
			manager := memberManagerFor(t, e.Env)
			seedWorkFor(t, e.Env, weeklyMailClock, e.Rep1)
			run, err := routineBriefEngine(e.Pool).SnapshotRun(e.repCtx, weeklyMailClock)
			if err != nil {
				t.Fatal(err)
			}
			roleDone, claimDone := make(chan error, 1), make(chan error, 1)
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				var user ids.UUID
				if err := tx.QueryRow(e.Admin(), `SELECT id FROM app_user WHERE id=$1 FOR UPDATE`, e.Rep1).Scan(&user); err != nil {
					return err
				}
				go func() {
					roleDone <- manager.service.ChangeUserRole(e.Admin(), recoveryAdminIdentity(e.Env), ids.From[ids.UserKind](e.Rep1), "read_only")
				}()
				waitForBackendBlockedBy(t, tx, backendPID(t, tx), roleDone)
				go func() {
					if edition == "combined" {
						_, _, err := claimRoutine(e.repCtx, e.Pool, run.ID, ids.Nil, morningDigest{}, weeklyMailClock, func(pgx.Tx) error { return nil })
						claimDone <- err
					} else {
						worker := routineBriefWorker(e.Pool, manager.service, nil, BriefMailConfig{})
						worker.now = func() time.Time { return weeklyMailClock }
						_, _, _, err := worker.claimMorningMail(e.repCtx, morningMailCandidate{run: run}, weeklyMailClock)
						claimDone <- err
					}
				}()
				waitForBackendsBlockedBy(t, tx, backendPID(t, tx), 2, claimDone)
				var id ids.UUID
				return tx.QueryRow(e.Admin(), `SELECT id FROM brief_run WHERE id=$1 FOR UPDATE NOWAIT`, run.ID).Scan(&id)
			}); err != nil {
				t.Fatal(err)
			}
			if err := <-roleDone; err != nil {
				t.Fatal(err)
			}
			if err := <-claimDone; err != nil && !errors.Is(err, errSendBasisChanged) {
				t.Fatal(err)
			}
		})
	}
}

func TestACombinedWeekQuotesOnlySourcesTheRecipientCanStillRead(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "readable", true: "revoked"}[revoke], func(t *testing.T) {
			e := setupWeeklyMail(t)
			manager := memberManagerFor(t, e.Env)
			yes := true
			if _, err := e.worker.users.SaveMyDelivery(e.repCtx, identity.Delivery{CombineRoutine: &yes}); err != nil {
				t.Fatal(err)
			}
			d := &digestEnv{Env: e.Env, store: notices.NewStore(e.DB()), now: weeklyMailClock}
			if _, err := d.store.SaveNotificationPreference(e.repCtx, "automation", notices.DeliveryDigest); err != nil {
				t.Fatal(err)
			}
			contact := d.capturedContact(t, "Pricing contact", e.Rep1)
			d.raise(t, "Pricing contact replied", contactRef(contact))
			d.recordedAt(t, "digest_suite:Pricing contact replied", weeklyMailClock.Add(-7*time.Hour))
			review := e.seedReview(t, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
			candidate, ready := e.worker.prepareWeeklyMail(e.repCtx, review, weeklyMailClock)
			if !ready {
				t.Fatal("weekly candidate was not ready")
			}
			if revoke {
				admin := recoveryAdminIdentity(e.Env)
				role, err := manager.service.CreateRole(e.Admin(), admin, "read_only", "Weekly reader without contact access")
				if err != nil {
					t.Fatal(err)
				}
				grant := role.Objects["contact"]
				grant.Read = false
				if _, err := manager.service.SetRoleObjectGrant(e.Admin(), admin, role.Key, "contact", grant, nil); err != nil {
					t.Fatal(err)
				}
				if err := manager.service.ChangeUserRole(e.Admin(), admin, ids.From[ids.UserKind](e.Rep1), role.Key); err != nil {
					t.Fatal(err)
				}
			}
			submission, won, err := e.worker.claimWeeklyMail(e.repCtx, candidate, weeklyMailClock)
			if err != nil || !won {
				t.Fatalf("weekly claim: won=%v err=%v", won, err)
			}
			if strings.Contains(submission.body.Text, "Pricing contact replied") == revoke {
				t.Fatalf("contact visibility was not respected: revoked=%v body=%s", revoke, submission.body.Text)
			}
		})
	}
}

func TestASendReservationRestoresTheCallersLockWait(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantCommitmentRole(t, a.e.Rep1)
	decision := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	reader := a.e.As(a.e.Rep1, nil, integration.AdminPerms)
	basis := readBasis(reader, t, a.e, a.e.Rep1, sendRefs{approvals: []ids.UUID{decision.approval}}, clocktest.Now(t))
	if len(basis.rows) == 0 || len(basis.excluded) > 0 {
		t.Fatal("the decision had no qualifying send basis")
	}
	for _, wait := range []string{"0", "25ms", "750ms"} {
		t.Run(wait, func(t *testing.T) {
			if err := a.e.DB().Tx(reader, func(tx pgx.Tx) error {
				var previous, after string
				if err := tx.QueryRow(reader, `SELECT set_config('lock_timeout', @wait, true)`, pgx.StrictNamedArgs{"wait": wait}).Scan(&previous); err != nil {
					return err
				}
				if err := holdSendBasisTx(reader, tx, basis, func() time.Time { return clocktest.Now(t) }); err != nil {
					return err
				}
				if err := tx.QueryRow(reader, `SELECT current_setting('lock_timeout')`).Scan(&after); err != nil {
					return err
				}
				if previous != after {
					t.Fatalf("reservation changed the caller's lock wait: %s -> %s", previous, after)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
