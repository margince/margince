// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCapturedPartiesReplaysStoredRecipientProofWithoutInventingFolders(t *testing.T) {
	for _, excludedFolder := range []bool{false, true} {
		t.Run(map[bool]string{false: "no folder exclusion", true: "folder provenance unavailable"}[excludedFolder], func(t *testing.T) {
			e := integration.Setup(t)
			sink := newCaptureSink(e.Pool, CaptureConfig{})
			ref := capturePartiesMailUsing(t, e, sink.WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
			system := principal.SystemActing(e.Admin(), "party-replay-test")
			// An older installation has the provider original and no party-source projection.
			if err := InstallationDB(e.Pool).Tx(system, func(tx pgx.Tx) error {
				if _, err := tx.Exec(system, `DELETE FROM capture_party_source_replay WHERE activity_id=@activity`, pgx.NamedArgs{"activity": ref.ID}); err != nil {
					return err
				}
				_, err := tx.Exec(system, `DELETE FROM capture_party_source WHERE activity_id=@activity`, pgx.NamedArgs{"activity": ref.ID})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			var excluded ids.UUID
			exclusions := capture.NewExclusionStore(InstallationDB(e.Pool))
			if excludedFolder {
				entry, err := exclusions.Add(e.Admin(), capture.ExclusionScopeUser, capture.ExclusionKindContainer, "gmail:Private")
				if err != nil {
					t.Fatal(err)
				}
				excluded = entry.ID
			}

			n, err := recoverPartySourcesBatch(system, e.Pool, 1, quietLog())
			if err != nil || n != 1 {
				t.Fatalf("source replay=%d: %v", n, err)
			}
			if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND sent_by_owner AND NOT containers_known AND recipients @> ARRAY['bob@buyer.test'] AND raw_capture_id IS NOT NULL`, ref.ID) != 1 {
				t.Fatal("original recipient proof was not recovered")
			}
			if err := sink.RecoverPartyAdmissions(system, 1); err != nil {
				t.Fatal(err)
			}
			count := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`)
			if (count == 1) == excludedFolder {
				t.Fatalf("recipient contacts=%d with folder exclusion=%v", count, excludedFolder)
			}
			if excludedFolder {
				if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND admission_pending`, ref.ID) != 1 {
					t.Fatal("a removable folder exclusion discarded pending admission")
				}
				if err := exclusions.Remove(e.Admin(), excluded); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if err := sink.RecoverPartyAdmissions(system, 1); err != nil {
						t.Fatal(err)
					}
				}
				if e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test'`) != 1 {
					t.Fatal("removing the folder exclusion did not resume pending admission")
				}
			}
			n, err = recoverPartySourcesBatch(system, e.Pool, 1, quietLog())
			if err != nil || n != 0 {
				t.Fatalf("source replay repeated settled evidence: %d %v", n, err)
			}
		})
	}
}

func TestCapturedPartiesRecipientProofRetiresWithTheProviderOriginal(t *testing.T) {
	e := integration.Setup(t)
	ref := capturePartiesMailUsing(t, e, newCaptureSink(e.Pool, CaptureConfig{}).WithEnsurer(nil, nil), partyMailProof{outbound: true, sentByOwner: true}, "Bob Buyer <bob@buyer.test>", "", "Could we discuss the product requirements?")
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1`, ref.ID) != 1 {
		t.Fatal("missing recipient proof before purge")
	}
	system := principal.SystemActing(e.Admin(), "party-purge-test")
	if err := InstallationDB(e.Pool).Tx(system, func(tx pgx.Tx) error {
		return capture.NewPendingStore(InstallationDB(e.Pool)).PurgeRawCaptureTx(system, tx, []ids.UUID{ref.ID})
	}); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND (cardinality(recipients)>0 OR cardinality(policy_addresses)>0 OR NOT admission_blocked)`, ref.ID) != 0 {
		t.Fatal("recipient proof survived original destruction or its privacy hold was lost")
	}
}

func TestHistoricalCalendarReplayKeepsProviderPrivacy(t *testing.T) {
	for _, provider := range []string{"gcal", "graphcal"} {
		for _, privacy := range []string{"normal", "private", "confidential", "personal", "malformed"} {
			if provider == "gcal" && privacy == "personal" {
				continue
			}
			t.Run(provider+"/"+privacy, func(t *testing.T) {
				e := integration.Setup(t)
				e.GrantRole(t, e.AdminUser, "admin")
				owner := backfillOwner
				ctx := calendarOwnerCtx(e, e.AdminUser)
				var raw []byte
				var rec connector.NormalizedRecord
				var err error
				if provider == "gcal" {
					seedCalendarConnection(t, e)
					visibility := privacy
					if privacy == "normal" {
						visibility = "default"
					}
					event := map[string]any{
						"id": ids.NewV7().String(), "status": "confirmed", "summary": "Product requirements", "visibility": visibility,
						"start": map[string]string{"dateTime": "2026-10-05T10:00:00Z"}, "end": map[string]string{"dateTime": "2026-10-05T11:00:00Z"},
						"organizer": map[string]any{"email": owner, "self": true}, "attendees": []map[string]any{{"email": owner, "self": true, "organizer": true, "responseStatus": "accepted"}, {"email": "bob@buyer.test", "responseStatus": "accepted"}},
					}
					raw, err = json.Marshal(event)
					if err != nil {
						t.Fatal(err)
					}
					rec, err = gcal.RecordOf(raw, owner)
				} else {
					owner = seatAddress(t, e, e.AdminUser)
					ctx = connectorOwnerCtx(e, e.AdminUser, "graphcal")
					e.WsExec(t, `INSERT INTO capture_connection(provider,user_id,status,account_label) VALUES('graphcal',$1,'connected',$2)`, e.AdminUser, owner)
					event := map[string]any{
						"id": ids.NewV7().String(), "subject": "Product requirements", "sensitivity": privacy,
						"start": map[string]string{"dateTime": "2026-10-05T10:00:00", "timeZone": "UTC"}, "end": map[string]string{"dateTime": "2026-10-05T11:00:00", "timeZone": "UTC"},
						"responseStatus": map[string]string{"response": "accepted"}, "organizer": map[string]any{"emailAddress": map[string]string{"address": owner}},
						"attendees": []map[string]any{{"type": "required", "emailAddress": map[string]string{"address": owner}, "status": map[string]string{"response": "accepted"}}, {"type": "required", "emailAddress": map[string]string{"address": "bob@buyer.test"}, "status": map[string]string{"response": "accepted"}}},
					}
					raw, err = json.Marshal(event)
					if err != nil {
						t.Fatal(err)
					}
					rec, err = graphcal.RecordOf(raw, owner)
				}
				if err != nil {
					t.Fatal(err)
				}
				sink := newCaptureSink(e.Pool, CaptureConfig{})
				ref, err := sink.WithEnsurer(nil, nil).Upsert(ctx, rec)
				if err != nil {
					t.Fatal(err)
				}
				e.WsExec(t, `DELETE FROM capture_party_source_replay WHERE activity_id=$1`, ref.ID)
				e.WsExec(t, `DELETE FROM capture_party_source WHERE activity_id=$1`, ref.ID)
				if privacy == "malformed" {
					e.WsExec(t, `UPDATE raw_capture SET payload=$2 WHERE id=(SELECT raw_capture_id FROM activity WHERE id=$1)`, ref.ID, []byte(`[]`))
				}
				system := principal.SystemActing(e.Admin(), "calendar-party-replay-test")
				n, err := recoverPartySourcesBatch(system, e.Pool, 1, quietLog())
				if err != nil || n != 1 {
					t.Fatalf("replay=%d %v", n, err)
				}
				blocked := e.WsCount(t, `SELECT count(*) FROM capture_party_source WHERE activity_id=$1 AND admission_blocked`, ref.ID)
				wantBlocked := privacy != "normal"
				if (blocked == 1) != wantBlocked {
					t.Fatalf("privacy=%s blocked=%d", privacy, blocked)
				}
				if err := sink.RecoverPartyAdmissions(system, 200); err != nil {
					t.Fatal(err)
				}
				contacts := e.WsCount(t, `SELECT count(*) FROM contact_email WHERE lower(email)='bob@buyer.test' AND archived_at IS NULL`)
				if (contacts == 1) == wantBlocked {
					t.Fatalf("privacy=%s guest contacts=%d", privacy, contacts)
				}
			})
		}
	}
}
