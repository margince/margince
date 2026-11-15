// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type directContentWire struct{ body, raw, decision []byte }

func directContentPositions(t *testing.T) (body, raw, decision int) {
	t.Helper()
	body, raw, decision = -1, -1, -1
	scan := &activityScan{}
	for i, column := range activityProjection {
		switch column.sql {
		case "a.body":
			body = i
		case "a.raw":
			raw = i
		}
		if column.into(scan) == &scan.contentAvailable {
			decision = i
		}
	}
	if body < 0 || raw < 0 || decision < 0 {
		t.Fatal("original body/raw/bool scan targets are missing")
	}
	return body, raw, decision
}

func readDirectContent(ctx context.Context, t *testing.T, store *Store, activityIDs []ids.UUID,
	decision string, format int16, malformedRaw bool,
) ([]crmcontracts.Activity, []directContentWire, error) {
	t.Helper()
	bodyAt, rawAt, decisionAt := directContentPositions(t)
	var found []crmcontracts.Activity
	var wire []directContentWire
	err := store.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(value any) int { args = append(args, value); return len(args) }
		projection, err := activityColumns(ctx, decision, arg)
		if err != nil {
			return err
		}
		if malformedRaw {
			// The writer accepts objects; inject only a bad source shape at the SQL boundary.
			columns := gatekit.SplitSQLList(projection)
			if !strings.Contains(columns[rawAt], "a.raw") {
				return errors.New("raw expression missing")
			}
			columns[rawAt] = strings.ReplaceAll(columns[rawAt], "a.raw", "jsonb_build_array(a.raw)")
			projection = strings.Join(columns, ", ")
		}
		query := "SELECT " + projection + " FROM activity a WHERE a.id = ANY(" + storekit.SQLf("$%d", arg(activityIDs)) + ") ORDER BY a.id"
		rows, err := tx.Query(ctx, query, append([]any{pgx.QueryResultFormats{format}}, args...)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		fields := rows.FieldDescriptions()
		if len(fields) != len(activityProjection) {
			return errors.New("projection width changed")
		}
		if fields[bodyAt].DataTypeOID != pgtype.TextOID || fields[rawAt].DataTypeOID != pgtype.JSONBOID || fields[decisionAt].DataTypeOID != pgtype.BoolOID {
			return errors.New("original payload or decision column type changed")
		}
		for rows.Next() {
			values := rows.RawValues()
			wire = append(wire, directContentWire{body: bytes.Clone(values[bodyAt]), raw: bytes.Clone(values[rawAt]), decision: bytes.Clone(values[decisionAt])})
			row, err := scanActivity(ctx, rows)
			if err != nil {
				return err
			}
			found = append(found, row)
		}
		return rows.Err()
	})
	return found, wire, err
}

func directContentColleague(ctx context.Context, t *testing.T, e *sendEnv) context.Context {
	t.Helper()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.UserID, actor.ID = e.other, "human:"+e.other.String()
	return principal.WithActor(ctx, actor)
}

func TestDirectContentSQLDistinguishesWriterNullsFromWithholding(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil).WithClock(func() time.Time { return epoch })
	var nullMap map[string]any
	for _, tc := range []struct {
		name string
		body *string
		raw  *map[string]any
	}{
		{name: "both SQL NULL"},
		{name: "JSON null", raw: &nullMap},
		{name: "empty body and object", body: new(""), raw: new(map[string]any{})},
		{name: "escaped", body: new("NULL, {text} \\ \"quoted\"\nTiếng Việt"), raw: new(map[string]any{"text": "\\\"{}\n", "number": 1.5})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			written, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Subject: new("Payload"), Body: tc.body, Raw: tc.raw, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.ActivityKind](ids.UUID(written.Id))
			if _, err := store.SetAudience(ctx, id, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
				t.Fatal(err)
			}
			author, err := store.GetActivity(ctx, id, storekit.LiveOnly)
			if err != nil || author.ContentState == nil || *author.ContentState != "available" || !reflect.DeepEqual(author.Body, tc.body) || !reflect.DeepEqual(author.Raw, written.Raw) {
				t.Fatalf("actual readable NULL/empty payload changed: %v", err)
			}
			colleague, err := store.GetActivity(directContentColleague(ctx, t, e), id, storekit.LiveOnly)
			if err != nil || colleague.ContentState == nil || *colleague.ContentState != "withheld" || colleague.Subject != nil || colleague.Body != nil || colleague.Raw != nil {
				t.Fatalf("actual audience did not withhold NULL/empty payload: %v", err)
			}
			for _, format := range []int16{pgtype.TextFormatCode, pgtype.BinaryFormatCode} {
				got, wire, err := readDirectContent(ctx, t, store, []ids.UUID{ids.UUID(written.Id)}, "TRUE", format, false)
				if err != nil || len(got) != 1 || len(wire) != 1 {
					t.Fatalf("readable row: %v", err)
				}
				if got[0].ContentState == nil || *got[0].ContentState != "available" || !reflect.DeepEqual(got[0].Body, tc.body) || !reflect.DeepEqual(got[0].Raw, written.Raw) {
					t.Fatal("readable payload or availability changed")
				}
				if (wire[0].body == nil) != (tc.body == nil) || (wire[0].raw == nil) != (tc.raw == nil) {
					t.Fatal("SQL NULL, empty body, or JSON null lost its wire distinction")
				}
				if wire[0].decision == nil {
					t.Fatal("available NULL payload lost its true decision")
				}
				held, withheldWire, err := readDirectContent(ctx, t, store, []ids.UUID{ids.UUID(written.Id)}, "FALSE", format, false)
				if err != nil || len(held) != 1 || len(withheldWire) != 1 {
					t.Fatalf("withheld row: %v", err)
				}
				if withheldWire[0].body != nil || withheldWire[0].raw != nil || withheldWire[0].decision == nil {
					t.Fatal("withheld payload was transferred or final decision was lost")
				}
				if held[0].Body != nil || held[0].Raw != nil || held[0].Subject != nil || held[0].ContentState == nil || *held[0].ContentState != "withheld" {
					t.Fatal("withheld row carried content or reported available")
				}
				if held[0].Id != got[0].Id || held[0].Kind != got[0].Kind || !held[0].OccurredAt.Equal(got[0].OccurredAt) || !reflect.DeepEqual(held[0].Version, got[0].Version) {
					t.Fatal("withheld markers changed")
				}
				_, nullWire, err := readDirectContent(ctx, t, store, []ids.UUID{ids.UUID(written.Id)}, "NULL::boolean", format, false)
				_, _, decisionAt := directContentPositions(t)
				var scanError pgx.ScanArgError
				if !errors.As(err, &scanError) || scanError.ColumnIndex != decisionAt {
					t.Fatalf("NULL decision did not fail its original bool scan: %v", err)
				}
				if len(nullWire) != 1 || nullWire[0].body != nil || nullWire[0].raw != nil || nullWire[0].decision != nil {
					t.Fatal("NULL decision did not leave the original three SQL values NULL")
				}
			}
		})
	}
}

func TestDirectContentRetainsReadableRowsAfterTheCursorCloses(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil).WithClock(func() time.Time { return epoch })
	want := map[ids.UUID]crmcontracts.Activity{}
	var activityIDs []ids.UUID
	for i, body := range []string{strings.Repeat("escaped \\ \"{}\"\nTiếng Việt ", 1024), "", "Last body"} {
		row, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Subject: new(fmt.Sprintf("Lifetime %d", i)), Body: &body, Raw: new(map[string]any{"sequence": i, "text": body}), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.UUID(row.Id)
		want[id], activityIDs = row, append(activityIDs, id)
	}
	for _, format := range []int16{pgtype.TextFormatCode, pgtype.BinaryFormatCode} {
		rows, _, err := readDirectContent(ctx, t, store, activityIDs, "TRUE", format, false)
		if err != nil || len(rows) != len(want) {
			t.Fatalf("multirow read: %v", err)
		}
		for _, row := range rows {
			expected, ok := want[ids.UUID(row.Id)]
			if !ok || !reflect.DeepEqual(row.Body, expected.Body) || !reflect.DeepEqual(row.Raw, expected.Raw) || !reflect.DeepEqual(row.Subject, expected.Subject) {
				t.Fatal("a retained row changed after another row was scanned")
			}
		}
		if rows[0].Raw == nil || rows[1].Raw == nil {
			t.Fatal("readable objects missing")
		}
		(*rows[0].Raw)["first_only"] = true
		if _, shared := (*rows[1].Raw)["first_only"]; shared {
			t.Fatal("readable rows share the same decoded map")
		}
	}
}

func TestDirectContentDoesNotDecodeAnInvalidWithheldMap(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil).WithClock(func() time.Time { return epoch })
	written, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Subject: new("Legacy raw shape"), Body: new("Readable body"), Raw: new(map[string]any{"original": true}), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	_, rawAt, _ := directContentPositions(t)
	for _, format := range []int16{pgtype.TextFormatCode, pgtype.BinaryFormatCode} {
		_, _, err := readDirectContent(ctx, t, store, []ids.UUID{ids.UUID(written.Id)}, "TRUE", format, true)
		var scanError pgx.ScanArgError
		if !errors.As(err, &scanError) || scanError.ColumnIndex != rawAt {
			t.Fatalf("available array-shaped JSON did not fail its map scan: %v", err)
		}
		held, wire, err := readDirectContent(ctx, t, store, []ids.UUID{ids.UUID(written.Id)}, "FALSE", format, true)
		if err != nil || len(held) != 1 || len(wire) != 1 {
			t.Fatalf("withheld bad shape was decoded: %v", err)
		}
		if wire[0].raw != nil || held[0].Raw != nil || held[0].ContentState == nil || *held[0].ContentState != "withheld" {
			t.Fatal("withheld bad shape reached the scanner")
		}
	}
}

func TestDirectContentKeepsActualAudienceAndArchiveReads(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil).WithClock(func() time.Time { return epoch })
	written, _, err := store.LogActivity(ctx, LogActivityInput{
		Kind: "note", Subject: new(noteSubject), Body: new(noteBody),
		Raw: new(map[string]any{"case": "audience"}), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(written.Id))
	if _, err := store.SetAudience(ctx, id, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	colleague := directContentColleague(ctx, t, e)
	got, err := store.GetActivity(colleague, id, storekit.LiveOnly)
	if err != nil || got.ContentState == nil || *got.ContentState != "withheld" || got.Body != nil || got.Raw != nil || got.Subject != nil {
		t.Fatalf("colleague read = %+v, %v", got, err)
	}
	own, err := store.GetActivity(ctx, id, storekit.LiveOnly)
	if err != nil || own.ContentState == nil || *own.ContentState != "available" || own.Body == nil || *own.Body != noteBody || !reflect.DeepEqual(own.Raw, written.Raw) {
		t.Fatalf("author read = %+v, %v", own, err)
	}
	if _, err := store.ArchiveActivity(e.asArchiver(), id, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetActivity(ctx, id, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("archived live read = %v", err)
	}
	archived, err := store.GetActivity(ctx, id, storekit.IncludeArchived)
	if err != nil || archived.ArchivedAt == nil || archived.ContentState == nil || *archived.ContentState != "available" || archived.Body == nil || *archived.Body != noteBody || !reflect.DeepEqual(archived.Raw, written.Raw) {
		t.Fatalf("archived author read = %+v, %v", archived, err)
	}
	archivedHidden, err := store.GetActivity(colleague, id, storekit.IncludeArchived)
	if err != nil || archivedHidden.ArchivedAt == nil || archivedHidden.ContentState == nil || *archivedHidden.ContentState != "withheld" || archivedHidden.Body != nil || archivedHidden.Raw != nil || archivedHidden.Subject != nil {
		t.Fatalf("archived colleague read = %+v, %v", archivedHidden, err)
	}
}

func TestDirectContentEmailSummaryKeepsMultipleReadableRows(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil).WithClock(func() time.Time { return epoch })
	var activityIDs []ids.UUID
	want := map[ids.UUID]crmcontracts.Activity{}
	for i, body := range []string{"Please send pricing.", "Second request.", ""} {
		row, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "email", Subject: new(fmt.Sprintf("Price request %d", i)), Body: &body, Raw: new(map[string]any{"row": i}), Direction: new("inbound"), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.UUID(row.Id)
		activityIDs, want[id] = append(activityIDs, id), row
	}
	summaries, err := store.EmailSummariesByIDAt(ctx, activityIDs, epoch)
	if err != nil || len(summaries) != len(want) {
		t.Fatalf("readable batch: %d, %v", len(summaries), err)
	}
	for id, row := range want {
		summary, ok := summaries[id]
		if !ok || !reflect.DeepEqual(summary.Subject, row.Subject) {
			t.Fatal("summary lost or transposed a readable subject")
		}
		preview := ""
		if row.Body != nil {
			preview = EmailSummaryText(*row.Body)
		}
		if preview == "" {
			if summary.Preview != nil {
				t.Fatal("empty body acquired a preview")
			}
		} else if summary.Preview == nil || *summary.Preview != preview {
			t.Fatal("summary lost or transposed a readable preview")
		}
	}
	privateID := ids.From[ids.ActivityKind](activityIDs[1])
	if _, err := store.SetAudience(ctx, privateID, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	hidden, err := store.EmailSummariesByIDAt(directContentColleague(ctx, t, e), activityIDs, epoch)
	if err != nil || len(hidden) != len(want)-1 {
		t.Fatalf("mixed summary batch: %d, %v", len(hidden), err)
	}
	if _, disclosed := hidden[activityIDs[1]]; disclosed {
		t.Fatal("content batch admitted the withheld email")
	}
	for _, id := range []ids.UUID{activityIDs[0], activityIDs[2]} {
		if !reflect.DeepEqual(hidden[id].Subject, want[id].Subject) {
			t.Fatal("mixed batch lost a readable subject")
		}
	}
	if _, err := store.ArchiveActivity(e.asArchiver(), ids.From[ids.ActivityKind](activityIDs[0]), nil); err != nil {
		t.Fatal(err)
	}
	after, err := store.EmailSummariesByIDAt(ctx, activityIDs, epoch)
	if err != nil || len(after) != len(want)-1 {
		t.Fatalf("archived summary batch: %d, %v", len(after), err)
	}
	if _, disclosed := after[activityIDs[0]]; disclosed {
		t.Fatal("content batch admitted an archived email")
	}
}

func TestDirectContentOrdinaryAndCollapsedPagesKeepRowsCursorsAndCount(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil).WithClock(func() time.Time { return epoch })
	var activityIDs []ids.ActivityID
	written := map[ids.UUID]crmcontracts.Activity{}
	for i := range 4 {
		row, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "note", Subject: new(fmt.Sprintf("Cursor %d", i)), Body: new(fmt.Sprintf("Body %d", i)), Raw: new(map[string]any{"row": i}), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.UUID(row.Id)
		activityIDs, written[id] = append(activityIDs, ids.From[ids.ActivityKind](id)), row
	}
	privateID := activityIDs[1]
	if _, err := store.SetAudience(ctx, privateID, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		reader   context.Context
		withheld bool
	}{
		{reader: ctx}, {reader: directContentColleague(ctx, t, e), withheld: true},
	} {
		reader := scenario.reader
		baseline, fullPage, err := store.ListActivities(reader, ListActivitiesInput{IDs: activityIDs, Limit: new(10)})
		if err != nil || len(baseline) != len(written) || fullPage.HasMore {
			t.Fatalf("full page: %d, %v", len(baseline), err)
		}
		for _, row := range baseline {
			want, ok := written[ids.UUID(row.Id)]
			if !ok || row.ContentState == nil {
				t.Fatal("unexpected activity or missing content state")
			}
			if scenario.withheld && ids.UUID(row.Id) == privateID.UUID {
				if *row.ContentState != "withheld" || row.Body != nil || row.Raw != nil || row.Subject != nil {
					t.Fatal("mixed page exposed private content")
				}
			} else if *row.ContentState != "available" || !reflect.DeepEqual(row.Body, want.Body) || !reflect.DeepEqual(row.Raw, want.Raw) {
				t.Fatal("page lost a readable body or raw object")
			}
		}
		var ordinaryCursor string
		for _, collapsed := range []bool{false, true} {
			in := ListActivitiesInput{IDs: activityIDs, Limit: new(2), CollapseSeatCopies: collapsed}
			count, err := store.CountActivities(reader, in)
			if err != nil || count != len(written) {
				t.Fatalf("count = %d, %v", count, err)
			}
			var found []crmcontracts.Activity
			for pageNumber := range 2 {
				rows, page, err := store.ListActivities(reader, in)
				if err != nil || len(rows) != 2 {
					t.Fatalf("page %d length = %d, %v", pageNumber, len(rows), err)
				}
				found = append(found, rows...)
				if page.HasMore != (pageNumber == 0) {
					t.Fatalf("page %d has_more = %t", pageNumber, page.HasMore)
				}
				if pageNumber == 0 {
					if page.NextCursor == "" {
						t.Fatal("first page has no continuation cursor")
					}
					if !collapsed {
						ordinaryCursor = page.NextCursor
					} else if page.NextCursor != ordinaryCursor {
						t.Fatal("collapsed page changed the unique-row cursor")
					}
				} else if page.NextCursor != "" {
					t.Fatal("exhausted page has a continuation cursor")
				}
				in.Cursor = &page.NextCursor
			}
			if !reflect.DeepEqual(found, baseline) {
				t.Fatal("ordinary/collapsed pagination changed full rows, order, or retained payloads")
			}
		}
	}
}
