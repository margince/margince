// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const reversedRowID = "018f3a2c-1b4d-7c8e-9f01-23456789abcd"

func TestAReversalLinkComesBackFromAnEvidenceImage(t *testing.T) {
	link, err := reversalLinkFromEvidence(map[string]any{UndidAuditLogID: reversedRowID})
	if err != nil {
		t.Fatalf("reading the link off an evidence image: %v", err)
	}
	if link == nil || link.String() != reversedRowID {
		t.Errorf("evidence link = %v, want %s", link, reversedRowID)
	}
}

// A key present but JSON null is not the string "null": the writer records the
// key unconditionally on some paths, and a reversal of nothing is no reversal.
func TestAnEvidenceLinkThatIsJSONNullReadsAsAbsent(t *testing.T) {
	var evidence map[string]any
	if err := json.Unmarshal([]byte(`{"`+UndidAuditLogID+`": null}`), &evidence); err != nil {
		t.Fatalf("decoding the evidence image: %v", err)
	}
	if _, present := evidence[UndidAuditLogID]; !present {
		t.Fatal("the case under test needs the key PRESENT and null")
	}
	link, err := reversalLinkFromEvidence(evidence)
	if err != nil {
		t.Fatalf("a null link is absent, not an error: %v", err)
	}
	if link != nil {
		t.Errorf("null evidence link = %v, want absent", link)
	}
}

func TestAnEvidenceImageWithoutTheLinkReadsAsAbsent(t *testing.T) {
	link, err := reversalLinkFromEvidence(map[string]any{"source": "import"})
	if err != nil {
		t.Fatalf("an ordinary evidence image is not an error: %v", err)
	}
	if link != nil {
		t.Errorf("link on an unrelated evidence image = %v, want absent", link)
	}
}

// A link that is neither absent nor a uuid is a corrupt spine row, and a read
// that guessed past it would pair a reversal with the wrong change.
func TestALinkThatIsNotAUUIDIsAnError(t *testing.T) {
	for name, value := range map[string]any{
		"a number":     json.Number("42"),
		"a bare word":  "not-a-uuid",
		"an empty key": "",
	} {
		if _, err := reversalLinkFromEvidence(map[string]any{UndidAuditLogID: value}); err == nil {
			t.Errorf("%s as a link: want an error, got none", name)
		}
	}
}

func TestTheProjectedLinkColumnCarriesTheLink(t *testing.T) {
	projected := reversedRowID
	link, err := reversalLinkFromColumn(&projected)
	if err != nil {
		t.Fatalf("reading the projected link column: %v", err)
	}
	if link == nil || link.String() != reversedRowID {
		t.Errorf("projected link = %v, want %s", link, reversedRowID)
	}
}

// `evidence ->> key` yields SQL NULL for both an absent key and a JSON null one,
// so the column path meets exactly one shape of absence.
func TestAnAbsentProjectedLinkColumnReadsAsAbsent(t *testing.T) {
	link, err := reversalLinkFromColumn(nil)
	if err != nil {
		t.Fatalf("a NULL column is absent, not an error: %v", err)
	}
	if link != nil {
		t.Errorf("NULL projected link = %v, want absent", link)
	}
}

// The link rides recordAuditColumns, whose scanner is shared by both readers of
// the audit spine. Neither end can gain a column alone: the list and the decode
// agree on arity here, or the other reader breaks at runtime on a query nothing
// in this package exercises without a database.
func TestTheAuditProjectionScansEveryColumnItSelects(t *testing.T) {
	selected := len(strings.Split(strings.TrimSpace(recordAuditColumns), ","))
	counted := &countingScanner{}
	if err := scanRecordAuditRow(counted, &recordAuditRow{}); err != nil {
		t.Fatalf("scanning the projection: %v", err)
	}
	if counted.dests != selected {
		t.Errorf("recordAuditColumns selects %d columns, scanRecordAuditRow reads %d", selected, counted.dests)
	}
}

type countingScanner struct{ dests int }

//craft:ignore naked-any the auditRowScanner contract this stands in for is pgx's own variadic any
func (c *countingScanner) Scan(dest ...any) error {
	c.dests = len(dest)
	return nil
}

func TestARecordHistoryEntryCarriesTheLinkItReversed(t *testing.T) {
	reversed := ids.MustParse(reversedRowID)
	row := recordAuditRow{actorType: "human", actorID: "human:x", action: "restore", undidAuditLogID: &reversed}

	entry := recordHistoryEntry(row, nil)
	if entry.UndidAuditLogID == nil || entry.UndidAuditLogID.String() != reversedRowID {
		t.Fatalf("entry link = %v, want %s", entry.UndidAuditLogID, reversedRowID)
	}
	if wire := recordHistoryEntryToWire(entry); wire.UndidAuditLogId == nil ||
		wire.UndidAuditLogId.String() != reversedRowID {
		t.Errorf("wire link = %v, want %s", wire.UndidAuditLogId, reversedRowID)
	}
	if recordHistoryEntry(recordAuditRow{action: "update"}, nil).UndidAuditLogID != nil {
		t.Error("an ordinary update must carry no link")
	}
}

// The link surfaces for EVERY actor, where the evidence image it rides surfaces
// for agents only: a restore's actor is a human, so masking the link with the
// image would hide it on every row that has one.
func TestAFieldHistoryEntryCarriesTheLinkForAHumanActor(t *testing.T) {
	reversed := ids.MustParse(reversedRowID)
	row := auditDiffRow{
		id: ids.NewV7(), action: "restore", actorType: "human", actorID: "human:x",
		before: map[string]any{"title": "C"}, after: map[string]any{"title": "B"},
		undidAuditLogID: &reversed,
	}

	entries := diffAuditRowFields(row, nil, nil)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want the one changed field", len(entries))
	}
	if entries[0].UndidAuditLogID == nil || entries[0].UndidAuditLogID.String() != reversedRowID {
		t.Fatalf("field entry link = %v, want %s", entries[0].UndidAuditLogID, reversedRowID)
	}
	if entries[0].Evidence != nil {
		t.Error("a human actor's evidence image stays withheld")
	}
	if wire := fieldHistoryEntryToWire(entries[0]); wire.UndidAuditLogId == nil ||
		wire.UndidAuditLogId.String() != reversedRowID {
		t.Errorf("wire link = %v, want %s", wire.UndidAuditLogId, reversedRowID)
	}
}
