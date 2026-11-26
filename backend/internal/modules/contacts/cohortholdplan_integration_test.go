// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The cohort repair runs on every contact.created and contact.updated event, so
// a history import pays for it once per contact it touches. Its first statement
// locks the activities the pass may write. That statement must find them
// through the contact's own addresses and participant rows. A read that walks
// every address-only participant row or every meeting costs more with each
// message the installation holds (#10557).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedUnrelatedMeetings adds meetings whose attendees are other contacts, so the
// meeting arm has resolved attendee rows to skip.
func (e *dedupeEnv) seedUnrelatedMeetings(ctx context.Context, t *testing.T, count int) {
	t.Helper()
	for after := 0; after < count; after += 5000 {
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `WITH attendees AS (
   SELECT id, row_number() OVER (ORDER BY id) AS n FROM contact
    WHERE captured_by = 'test:bounded-history' ORDER BY id OFFSET @after LIMIT 5000),
  meetings AS (
   INSERT INTO activity(kind,direction,source,source_system,source_id,captured_by,occurred_at)
   SELECT 'meeting','inbound','gcal:seed','gcal','unrelated-meeting-'||(@after::int+n),'connector:gcal',now()
     FROM attendees RETURNING id, source_id)
  INSERT INTO activity_participant(activity_id,role,contact_id)
  SELECT m.id,'attendee',a.id FROM meetings m JOIN attendees a ON m.source_id='unrelated-meeting-'||(@after::int+a.n)`,
				pgx.NamedArgs{"after": after})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheCohortHoldSeeksPastAnUnrelatedInstallation(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	connected := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	e.seedHistoryReadBackground(ctx, t, connected)
	e.seedUnrelatedMeetings(ctx, t, 20000)

	const address, retiredAddress = "cohort@bounded-cohort.test", "retired@bounded-cohort.test"
	contact := ids.From[ids.ContactKind](e.plainContact(ctx, t, "Bounded Cohort", address))
	retired := e.plainContact(ctx, t, "Retired Cohort", retiredAddress)
	mail := e.seedCapturedMail(ctx, t, address, "from")
	attended := e.seedCapturedMeeting(ctx, t, address)
	mergedAttendee := e.seedCapturedMeeting(ctx, t, retiredAddress)
	// An attendee row that still names a record merged into this contact: the
	// meeting arm resolves it through the merge redirect.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE activity_participant SET contact_id = $1, address = NULL WHERE activity_id = $2`,
			retired, mergedAttendee); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE contact SET merged_into_id = $1, archived_at = now() WHERE id = $2`, contact, retired)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	analyzeAsOwner(t, "activity", "activity_participant", "activity_link", "contact", "contact_email")
	want := []ids.UUID{mail.UUID, attended.UUID, mergedAttendee.UUID}
	slices.SortFunc(want, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })

	recorder := &historyPlanTrace{}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		t.Run(mode, func(t *testing.T) {
			if err := e.store.tx(ctx, func(nativeTx pgx.Tx) error {
				tx := historyPlanTx{Tx: nativeTx, trace: recorder}
				if _, err := tx.Exec(ctx, `SELECT set_config('plan_cache_mode',@mode,true)`, pgx.NamedArgs{"mode": mode}); err != nil {
					return err
				}
				recorder.needle, recorder.query, recorder.args = "FOR NO KEY UPDATE", "", nil
				held, err := holdCohortActivities(ctx, tx, contact, skipBusyActivities)
				if err != nil {
					return err
				}
				if !slices.Equal(held, want) {
					return fmt.Errorf("held %v, want %v", held, want)
				}
				recorder.needle = ""
				raw, err := testdb.ExplainCachedStatement(ctx, tx, recorder.query, recorder.args)
				if err != nil {
					return err
				}
				var plans []struct{ Plan mailboxPlanNode }
				if err := json.Unmarshal(raw, &plans); err != nil {
					return err
				}
				buffers := plans[0].Plan.SharedHits + plans[0].Plan.SharedReads
				if len(plans) != 1 || !plans[0].Plan.boundsHistoryRows(64) || buffers > 512 {
					return fmt.Errorf("the cohort hold visits unrelated rows (%d buffers): %s", buffers, raw)
				}
				t.Logf("shared_buffers=%d", buffers)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}

	// The narrowed read changes no outcome: the repair still files every
	// message under the contact.
	if _, err := e.store.RepairContactCohort(ctx, contact); err != nil {
		t.Fatal(err)
	}
	for _, activity := range []ids.ActivityID{mail, attended, mergedAttendee} {
		if linked, _ := e.cohortStateOf(ctx, t, activity, contact); !linked {
			t.Errorf("activity %s is not filed under the contact after the repair", activity)
		}
	}
	if _, named := e.cohortStateOf(ctx, t, attended, contact); !named {
		t.Error("the attendee row of the meeting does not name the contact after the repair")
	}
}
