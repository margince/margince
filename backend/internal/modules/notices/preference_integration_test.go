// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// One seat's routing decision against a real database: the whole set comes back
// so a screen cannot overwrite a row it never rendered, the choice is the
// seat's own and reaches nobody else's, and the write lands in the shape every
// mutation lands in — row, ledger entry, announcement — with the class on the
// wire and the choice left off it.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNotificationPreferenceAnswersTheWholeSetAndLeavesAColleaguesAlone(t *testing.T) {
	e := setupNotices(t)

	set, err := e.store.SaveNotificationPreference(e.asUser(e.recipient), classAutomation, DeliveryDigest)
	if err != nil {
		t.Fatalf("saving a choice: %v", err)
	}

	// The WHOLE set, not the row that moved: a settings screen that had to
	// merge the answer into what it already held would write back a stale copy
	// of every other class the moment two tabs disagreed.
	if len(set) != len(noticeClasses) {
		t.Fatalf("the save answered %d rows, want one per class (%d)", len(set), len(noticeClasses))
	}
	chosen := rowFor(t, set, classAutomation)
	if chosen.Delivery != DeliveryDigest || !chosen.Chosen {
		t.Fatalf("the saved class came back as %+v, want the digest, chosen", chosen)
	}
	if approvals := rowFor(t, set, ClassApprovalPending); approvals.Delivery != DeliveryEmail || approvals.Chosen {
		t.Fatalf("an untouched approvals row came back as %+v, want the mailed default, unchosen", approvals)
	}

	// The seat's own read says the same thing the save did.
	mine, err := e.store.MyNotificationPreferences(e.asUser(e.recipient))
	if err != nil {
		t.Fatalf("reading my settings: %v", err)
	}
	if again := rowFor(t, mine, classAutomation); again != chosen {
		t.Fatalf("the read answers %+v where the save answered %+v", again, chosen)
	}

	// A colleague reads pure defaults. Nothing about one seat's decision is
	// visible in another's, and nothing the first seat did created a row for
	// the second.
	theirs, err := e.store.MyNotificationPreferences(e.asUser(e.other))
	if err != nil {
		t.Fatalf("a colleague reading their settings: %v", err)
	}
	for _, row := range theirs {
		if row.Chosen || row.Delivery != DefaultDelivery(row.Class) {
			t.Fatalf("a colleague who decided nothing holds %+v", row)
		}
	}
}

func TestNotificationPreferenceIsWrittenInTheWriteShapeAndOnlyWhenItMoves(t *testing.T) {
	e := setupNotices(t)
	seat := e.asUser(e.recipient)
	if _, err := e.store.SaveNotificationPreference(seat, classLeadSLA, DeliveryEmail); err != nil {
		t.Fatalf("saving a choice: %v", err)
	}

	audits, announcements := preferenceWrites(t, e)
	if audits != 1 || announcements != 1 {
		t.Fatalf("write shape: %d audit rows, %d announcements — want one of each", audits, announcements)
	}

	// The announcement names the CLASS and not the choice: a subscriber must
	// not learn from a fan-out who switched their mail off.
	var payload map[string]json.RawMessage
	var raw []byte
	if err := e.owner.QueryRow(context.Background(),
		`SELECT envelope->'payload' FROM event_outbox
		  WHERE envelope->>'type' = 'notification.preference_changed'`).Scan(&raw); err != nil {
		t.Fatalf("reading the announcement: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decoding the announcement: %v", err)
	}
	if class := string(payload["class"]); class != `"`+classLeadSLA+`"` {
		t.Fatalf("the announcement names class %s, want %q", class, classLeadSLA)
	}
	if _, told := payload["delivery"]; told {
		t.Fatalf("the announcement carries the choice itself: %s", raw)
	}

	// The same choice saved again is not a change. A settings screen that saves
	// on every render would otherwise fill the ledger with changes nobody made.
	if _, err := e.store.SaveNotificationPreference(seat, classLeadSLA, DeliveryEmail); err != nil {
		t.Fatalf("saving the same choice again: %v", err)
	}
	if audits, announcements = preferenceWrites(t, e); audits != 1 || announcements != 1 {
		t.Fatalf("a save that moved nothing wrote %d audit rows and %d announcements", audits, announcements)
	}

	// A different choice IS a change, and the ledger says what it changed from.
	if _, err := e.store.SaveNotificationPreference(seat, classLeadSLA, DeliveryDigest); err != nil {
		t.Fatalf("changing the choice: %v", err)
	}
	var before, after string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT before->>$2, after->>$2 FROM audit_log
		  WHERE entity_type = 'user' AND entity_id = $1 AND action = 'update'
		  ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		e.recipient, classLeadSLA).Scan(&before, &after); err != nil {
		t.Fatalf("reading the ledger entry: %v", err)
	}
	if before != DeliveryEmail || after != DeliveryDigest {
		t.Fatalf("the ledger records %q → %q, want %q → %q", before, after, DeliveryEmail, DeliveryDigest)
	}
}

// The delivery legs' own read: a system principal, inside a transaction it
// already holds, for a seat it is delivering to.
func TestNotificationPreferenceDeliveryForAnswersTheChoiceOrTheDefault(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.SaveNotificationPreference(e.asUser(e.recipient), classAutomation, DeliveryOff); err != nil {
		t.Fatalf("saving a choice: %v", err)
	}

	ctx := e.engineCtx()
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, tc := range []struct {
			name      string
			recipient ids.UserID
			class     string
			want      string
		}{
			{"the seat's own decision", e.recipient, classAutomation, DeliveryOff},
			{"a class that seat never decided", e.recipient, classCapture, DeliveryInApp},
			{"a seat who decided nothing", e.other, classAutomation, DeliveryInApp},
			{"the class that leaves the product", e.other, ClassApprovalPending, DeliveryEmail},
		} {
			delivery, err := DeliveryFor(ctx, tx, tc.recipient, tc.class)
			if err != nil {
				return err
			}
			if delivery != tc.want {
				t.Errorf("%s: DeliveryFor = %q, want %q", tc.name, delivery, tc.want)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("reading deliveries: %v", err)
	}
}

// rowFor answers the set's entry for one class, failing when there is none: a
// class a notice can arrive in and the reader cannot route is a hole in the
// settings screen rather than an empty row.
func rowFor(t *testing.T, set []Preference, class string) Preference {
	t.Helper()
	for _, row := range set {
		if row.Class == class {
			return row
		}
	}
	t.Fatalf("the answer holds no row for %q", class)
	return Preference{}
}

// preferenceWrites counts what the save left behind on both ledgers.
func preferenceWrites(t *testing.T, e *noticeEnv) (audits, announcements int) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT (SELECT count(*) FROM audit_log
		          WHERE entity_type = 'user' AND entity_id = $1 AND action = 'update'),
		        (SELECT count(*) FROM event_outbox
		          WHERE envelope->>'type' = 'notification.preference_changed')`,
		e.recipient).Scan(&audits, &announcements); err != nil {
		t.Fatalf("counting what the save wrote: %v", err)
	}
	return audits, announcements
}
