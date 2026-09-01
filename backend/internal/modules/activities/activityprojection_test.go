// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Every column of the activity projection lands in its OWN field.
//
// Five string-ish neighbours can transpose without an error — meeting_status,
// source_system, source_id, source, captured_by — and they are not the only
// pair that can. Two booleans (is_done, bulk_mail_attested) and seven
// timestamps are as interchangeable to a scan as two strings, and a swap among
// them is just as silent.
//
// So every destination is filled from its POSITION in the projection, and every
// field the scan materializes is asserted against the column it is declared
// for. A value that could be any column's is a value this test cannot judge.

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// epoch anchors the timestamp sentinels. Each column gets epoch plus its own
// index, so two timestamps are never the same instant.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// sentinelRow answers each destination with a value derived from its position,
// writing through the pointers the projection handed it — the same path pgx
// takes. A row that filled a struct by name would prove nothing about an order
// nobody stated.
type sentinelRow struct {
	t     *testing.T
	names []string
	// bools counts the boolean destinations as they arrive, so consecutive
	// ones alternate. Two booleans set to the same value are indistinguishable
	// after a swap, which is exactly the case this row exists to catch.
	bools int
	// flip inverts that alternation. Three booleans and two values means one
	// pair must collide in any single run — is_done and content_available did
	// — so the scan runs twice with the parity reversed, and a pair that
	// matched in one run differs in the other.
	flip bool
	// last is content_available's position. It is forced true in BOTH runs:
	// false there withholds the content columns, so a run that flipped it
	// would report the withholding as a transposition rather than testing one.
	last int
}

func (r *sentinelRow) Scan(dest ...any) error {
	r.t.Helper()
	if len(dest) != len(r.names) {
		r.t.Fatalf("the scan asked for %d destinations and the projection declares %d columns",
			len(dest), len(r.names))
	}
	for i, d := range dest {
		r.fill(d, r.names[i], i)
	}
	return nil
}

// fill writes a value that identifies the column it was written for: the name
// itself where a string will hold one, the index where a number or an instant
// will, and an alternating value for a boolean, which has only two.
//
//craft:ignore naked-any a scan destination is exactly what pgx.Row.Scan takes, and the whole subject here is which concrete pointer arrives in which position — naming a type would be naming the answer this switch exists to discover.
func (r *sentinelRow) fill(dest any, name string, index int) {
	r.t.Helper()
	switch d := dest.(type) {
	case *string:
		*d = name
	case **string:
		v := name
		*d = &v
	case *ids.UUID:
		*d = uuidFor(index)
	case **ids.UUID:
		v := uuidFor(index)
		*d = &v
	case *time.Time:
		*d = epoch.Add(time.Duration(index) * time.Hour)
	case **time.Time:
		v := epoch.Add(time.Duration(index) * time.Hour)
		*d = &v
	case *bool:
		*d = r.boolAt(index)
	case **bool:
		v := r.boolAt(index)
		*d = &v
	case *int64:
		*d = int64(index)
	case **int:
		v := index
		*d = &v
	case **int64:
		v := int64(index)
		*d = &v
	default:
		r.t.Fatalf("the projection declares a destination this row cannot fill for %s: %T", name, dest)
	}
}

// boolAt answers the boolean this column gets: alternating in arrival order,
// inverted on the second run, and always true for content_available.
func (r *sentinelRow) boolAt(index int) bool {
	v := r.bools%2 == 0
	r.bools++
	if index == r.last {
		return true
	}
	return v != r.flip
}

// uuidFor is a distinct, reproducible id per column position.
func uuidFor(index int) ids.UUID {
	var u ids.UUID
	u[15] = byte(index + 1)
	return u
}

var _ pgx.Row = (*sentinelRow)(nil)

func TestEveryProjectedColumnLandsInItsOwnField(t *testing.T) {
	names := make([]string, len(activityProjection))
	index := map[string]int{}
	for i, c := range activityProjection {
		names[i] = c.sql
		if names[i] == "" {
			names[i] = "content_available"
		}
		index[names[i]] = i
	}
	// content_available is the projection's last column, and this test relies
	// on that to keep it true in both runs. Asserted rather than assumed: a
	// column appended after it would make the forcing silently miss.
	if index["content_available"] != len(activityProjection)-1 {
		t.Fatalf("content_available is column %d of %d — it is expected last, and the boolean "+
			"sentinel forces it by position", index["content_available"], len(activityProjection))
	}

	// TWICE, with the boolean parity reversed. Three booleans share two values,
	// so one pair collides in any single run — is_done and content_available
	// did, which made swapping their destinations invisible. Reversing the
	// parity separates whichever pair matched.
	for _, flip := range []bool{false, true} {
		t.Run(map[bool]string{false: "as declared", true: "with the booleans reversed"}[flip], func(t *testing.T) {
			row := &sentinelRow{t: t, names: names, flip: flip, last: index["content_available"]}
			got, err := scanActivity(row)
			if err != nil {
				t.Fatalf("scanActivity: %v", err)
			}
			assertProjection(t, got, index, flip)
		})
	}
}

// assertProjection checks every field the scan materializes against the column
// it is declared for.
func assertProjection(t *testing.T, got crmcontracts.Activity, index map[string]int, flip bool) {
	t.Helper()
	// content_available decides whether the content columns reach the caller
	// at all, so a run where it landed false would blank half the fields below
	// and report the blanking as a transposition.
	if got.ContentState == nil {
		t.Fatal("content_state was never set — the scan did not reach the audience arm")
	}
	if string(*got.ContentState) != "available" {
		t.Fatalf("content_available did not scan as true (content_state %q) — every content "+
			"assertion below would then be reading the withholding, not the scan", *got.ContentState)
	}

	for _, want := range []struct {
		column string
		got    *string
	}{
		{"a.subject", got.Subject},
		{"a.body", got.Body},
		{"a.source_system", got.SourceSystem},
		{"a.source_id", got.SourceId},
		{"a.captured_by", got.CapturedBy},
		{"a.channel_provider", got.ChannelProvider},
		{"a.thread_key", got.ThreadKey},
	} {
		assertString(t, want.column, want.got)
	}
	if got.Source != "a.source" {
		t.Errorf("a.source landed in a field holding %q", got.Source)
	}

	// The typed enums come off string columns and are the other half of the
	// same hazard: a swap reads as a valid-looking wrong value.
	assertString(t, "a.direction", stringOf(got.Direction))
	assertString(t, "a.meeting_status", stringOf(got.MeetingStatus))
	assertString(t, "a.capture_label", stringOf(got.CaptureLabel))
	assertString(t, "a.audience", stringOf(got.Audience))
	if string(got.Kind) != "a.kind" {
		t.Errorf("a.kind landed as %q", got.Kind)
	}

	// The instants. Seven of them, and any two are as interchangeable to a
	// scan as any two strings.
	for _, want := range []struct {
		column string
		got    *time.Time
	}{
		{"a.occurred_at", &got.OccurredAt},
		{"a.due_at", got.DueAt},
		{"a.remind_at", got.RemindAt},
		{"a.done_at", got.DoneAt},
		{"a.created_at", &got.CreatedAt},
		{"a.updated_at", &got.UpdatedAt},
		{"a.archived_at", got.ArchivedAt},
	} {
		if want.got == nil {
			t.Errorf("%s scanned into nothing", want.column)
			continue
		}
		if at := epoch.Add(time.Duration(index[want.column]) * time.Hour); !want.got.Equal(at) {
			t.Errorf("%s holds %v, want %v — two timestamp destinations are transposed",
				want.column, want.got, at)
		}
	}

	// The ids.
	if ids.UUID(got.Id) != uuidFor(index["a.id"]) {
		t.Errorf("a.id holds %v", got.Id)
	}
	if got.AssigneeId == nil || ids.UUID(*got.AssigneeId) != uuidFor(index["a.assignee_id"]) {
		t.Errorf("a.assignee_id holds %v", got.AssigneeId)
	}

	// The numbers.
	if got.DurationSeconds == nil || *got.DurationSeconds != index["a.duration_seconds"] {
		t.Errorf("a.duration_seconds holds %v", got.DurationSeconds)
	}
	if got.Version == nil || int(*got.Version) != index["a.version"] {
		t.Errorf("a.version holds %v", got.Version)
	}

	// The booleans, which carry the least information of anything here and are
	// therefore the easiest to transpose undetected.
	assertBool(t, "a.is_done", got.IsDone, boolFor(index["a.is_done"]) != flip)
	assertBool(t, "a.bulk_mail_attested", got.BulkMailAttested, boolFor(index["a.bulk_mail_attested"]) != flip)
}

// boolFor answers what the row wrote for the boolean at this column, by
// counting the booleans that precede it in declaration order. Derived rather
// than written down, so the two stay in step when a column moves.
func boolFor(at int) bool {
	seen := 0
	for i, c := range activityProjection {
		if i >= at {
			break
		}
		if isBoolColumn(c.sql) {
			seen++
		}
	}
	return seen%2 == 0
}

// isBoolColumn names the projection's boolean columns. A list, because the
// destination's type is only knowable at scan time and this has to answer
// before one exists.
func isBoolColumn(sql string) bool {
	switch sql {
	case "a.is_done", "a.bulk_mail_attested", "":
		return true
	}
	return false
}

func assertBool(t *testing.T, column string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Errorf("%s scanned into nothing", column)
		return
	}
	if *got != want {
		t.Errorf("%s holds %t, want %t — two boolean destinations are transposed, and a boolean "+
			"carries the least information of anything here, so a swap is the easiest to miss",
			column, *got, want)
	}
}

func assertString(t *testing.T, column string, got *string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s scanned into nothing", column)
		return
	}
	if *got != column {
		t.Errorf("%s landed in a field holding %q — two destinations are transposed, and a "+
			"transposition among these scans cleanly and puts the wrong value on the wire",
			column, *got)
	}
}

// stringOf reads a typed-enum pointer back as the string it was scanned from.
func stringOf[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	s := string(*v)
	return &s
}

// The select list and the destinations come from one declaration, so they agree
// by construction. Asserted anyway, because that is the property the
// declaration exists to hold, and a refactor that reintroduced a second list
// would land here first.
//
// The RENDERED list is what is checked, not just the count of destinations. A
// count on its own cannot tell a projection that declares a column from one
// that renders it: drop a column's SQL and the destinations still match the
// declaration, while the row pgx scans is one short — which fails as a scan
// arity error somewhere else, at a call site that has nothing to do with the
// omission.
func TestTheSelectListAndTheScanAgreeOnLength(t *testing.T) {
	// A contentArm with no comma in it, so the count below is the projection's
	// commas and not the predicate's.
	columns := activityColumns("true")
	dests := activityScanTargets(&activityScan{})
	if len(dests) != len(activityProjection) {
		t.Fatalf("the scan asks for %d destinations and the projection declares %d columns",
			len(dests), len(activityProjection))
	}
	rendered := strings.Split(columns, ", ")
	if len(rendered) != len(activityProjection) {
		t.Errorf("the select list renders %d columns and the projection declares %d — pgx scans "+
			"the row the LIST asked for, so a column declared and not rendered is a scan that "+
			"fails somewhere with nothing to do with the omission",
			len(rendered), len(activityProjection))
	}
	// And none of them is blank. A declared column whose SQL went missing still
	// renders a position — `a.due_at, , a.assignee_id` — so the count survives
	// while the row is one column short of what the destinations expect.
	for i, column := range rendered {
		if strings.TrimSpace(column) == "" {
			t.Errorf("the select list renders nothing at position %d; the projection declares a "+
				"column there", i)
		}
	}
	// And each declared column is actually in it, so a rename cannot keep the
	// count while selecting something else.
	for _, c := range activityProjection {
		if c.sql == "" {
			continue
		}
		if !strings.Contains(columns, c.sql) {
			t.Errorf("the projection declares %s and the select list does not name it", c.sql)
		}
	}
	if !strings.HasSuffix(columns, "AS content_available") {
		t.Errorf("the select list does not end with the audience arm: %q", columns)
	}
}

// withheldRow answers the scan the way a row the caller may DISCOVER but not
// read comes back: every column filled as the database has it, and
// content_available false. It fills the content columns deliberately — the
// question is what `record` does with a reason that IS set, not what happens
// when the database had none.
type withheldRow struct {
	t     *testing.T
	names []string
}

func (r *withheldRow) Scan(dest ...any) error {
	r.t.Helper()
	for i, d := range dest {
		switch d := d.(type) {
		case *string:
			*d = "set"
		case **string:
			v := "set"
			*d = &v
		case *bool:
			// content_available is the projection's last column and the only
			// one this row answers false: that is the whole condition under
			// test.
			*d = i != len(dest)-1
		case *int64:
			*d = 1
		case *int32:
			*d = 1
		case *time.Time:
			*d = epoch
		case **time.Time:
			v := epoch
			*d = &v
		case *ids.ActivityID:
			*d = ids.ActivityID{}
		case **ids.UserID:
			*d = nil
		}
	}
	return nil
}

// A withheld row says WHAT it is — kind, date, direction — and nothing about
// what it is ABOUT. `audience_reason` is on the second list: the reason a
// message is held describes its subject matter ("personnel", "legal",
// "security_incident"), so a colleague who may not read a held message must
// not learn why it is held either. The contract states this
// (crm.yaml: "absent whenever content_state is withheld"), and the field is
// optional on the wire, so nothing downstream fails when it leaks — which is
// why it is asserted here.
func TestAWithheldRowCarriesNoAudienceReason(t *testing.T) {
	t.Parallel()
	names := make([]string, len(activityProjection))
	for i, c := range activityProjection {
		names[i] = c.sql
	}
	got, err := scanActivity(&withheldRow{t: t, names: names})
	if err != nil {
		t.Fatalf("scanActivity: %v", err)
	}
	if got.ContentState == nil || *got.ContentState != crmcontracts.ActivityContentStateWithheld {
		t.Fatalf("the row did not come back withheld (content_state %v) — the assertions "+
			"below would then be reading an available row", got.ContentState)
	}
	// Named one at a time rather than as a loop over a list: each of these is
	// a separate promise the contract makes about a withheld row, and a
	// failure should say which one broke.
	if got.AudienceReason != nil {
		t.Errorf("a withheld row carried audience_reason %q — the reason describes what the "+
			"message is about, so it is withheld with the content", *got.AudienceReason)
	}
	if got.Subject != nil {
		t.Errorf("a withheld row carried a subject")
	}
	if got.Body != nil {
		t.Errorf("a withheld row carried a body")
	}
	if got.ThreadKey != nil {
		t.Errorf("a withheld row carried a thread key — it identifies the message at the provider")
	}
	// The markers a discoverable row keeps. Asserted so the test cannot pass by
	// blanking everything, which would withhold the row's existence too.
	if got.Audience == nil {
		t.Error("a withheld row lost its audience — the caller may know a row is limited")
	}
	if got.Kind == "" {
		t.Error("a withheld row lost its kind — a discoverable row still says what it is")
	}
}
