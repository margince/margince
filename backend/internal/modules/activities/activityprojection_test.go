// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Every column of the activity projection lands in its OWN field.
//
// Five neighbours are string-ish and nullable — meeting_status, source_system,
// source_id, source, captured_by — so transposing any two of them scans
// cleanly and puts the wrong value on the wire. Nothing errors. A record
// simply says its source is a meeting status.
//
// The scan is driven by a row that hands each column a sentinel naming the
// column it came from, so a swap is not "some string in some field" but a
// named mismatch.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sentinelRow answers each destination with a value derived from its POSITION
// in the projection, so what a field ends up holding names where it came from.
//
// It writes through the destination pointers the projection handed it, which
// is the same path pgx takes — a row that filled a struct by name would prove
// nothing about an order nobody stated.
type sentinelRow struct {
	t     *testing.T
	names []string
}

func (r *sentinelRow) Scan(dest ...any) error {
	r.t.Helper()
	if len(dest) != len(r.names) {
		r.t.Fatalf("the scan asked for %d destinations and the projection declares %d columns",
			len(dest), len(r.names))
	}
	for i, d := range dest {
		fill(r.t, d, r.names[i])
	}
	return nil
}

// fill writes a value that carries `name` where the type allows it, and
// something valid where it does not. Only the string-ish columns can be
// silently transposed, so only they need to be distinguishable.
//
//craft:ignore naked-any a scan destination is exactly what pgx.Row.Scan takes, and the whole subject here is which concrete pointer arrives in which position — naming a type would be naming the answer this switch exists to discover.
func fill(t *testing.T, dest any, name string) {
	t.Helper()
	switch d := dest.(type) {
	case *string:
		*d = name
	case **string:
		v := name
		*d = &v
	case *ids.UUID:
		*d = ids.NewV7()
	case **ids.UUID:
		v := ids.NewV7()
		*d = &v
	case *time.Time:
		*d = time.Unix(0, 0).UTC()
	case **time.Time:
		v := time.Unix(0, 0).UTC()
		*d = &v
	case *bool:
		// content_available true, so nothing is withheld and every field the
		// audience test would blank stays readable for the comparison below.
		*d = true
	case **bool:
		v := true
		*d = &v
	case *int64:
		*d = 1
	case **int:
		v := 1
		*d = &v
	case **int64:
		v := int64(1)
		*d = &v
	default:
		t.Fatalf("the projection declares a destination this row cannot fill for %s: %T", name, dest)
	}
}

var _ pgx.Row = (*sentinelRow)(nil)

func TestEveryProjectedColumnLandsInItsOwnField(t *testing.T) {
	names := make([]string, len(activityProjection))
	for i, c := range activityProjection {
		names[i] = c.sql
		if names[i] == "" {
			names[i] = "content_available"
		}
	}

	got, err := scanActivity(&sentinelRow{t: t, names: names})
	if err != nil {
		t.Fatalf("scanActivity: %v", err)
	}

	// Only the string-carrying columns can transpose without an error, so
	// those are the ones worth naming. Each is asserted against the column it
	// is declared for, not against a position.
	for _, want := range []struct {
		column string
		got    *string
	}{
		{"a.subject", got.Subject},
		{"a.body", got.Body},
		{"a.source_system", got.SourceSystem},
		{"a.source_id", got.SourceId},
		{"a.channel_provider", got.ChannelProvider},
		{"a.thread_key", got.ThreadKey},
		{"a.captured_by", got.CapturedBy},
	} {
		if want.got == nil {
			t.Errorf("%s scanned into nothing", want.column)
			continue
		}
		if *want.got != want.column {
			t.Errorf("%s landed in a field holding %q — two destinations are transposed, and a "+
				"transposition among these scans cleanly and puts the wrong value on the wire",
				want.column, *want.got)
		}
	}
	// Source is a plain string on the record rather than a pointer.
	if got.Source != "a.source" {
		t.Errorf("a.source landed in a field holding %q", got.Source)
	}
	// The typed enums come off the same string columns and are the other half
	// of the same hazard: a swap here reads as a valid-looking wrong value.
	if got.Direction == nil || string(*got.Direction) != "a.direction" {
		t.Errorf("a.direction landed as %v", got.Direction)
	}
	if got.MeetingStatus == nil || string(*got.MeetingStatus) != "a.meeting_status" {
		t.Errorf("a.meeting_status landed as %v", got.MeetingStatus)
	}
	if got.CaptureLabel == nil || string(*got.CaptureLabel) != "a.capture_label" {
		t.Errorf("a.capture_label landed as %v", got.CaptureLabel)
	}
	if string(got.Kind) != "a.kind" {
		t.Errorf("a.kind landed as %q", got.Kind)
	}
	if got.Audience == nil || string(*got.Audience) != "a.audience" {
		t.Errorf("a.audience landed as %v", got.Audience)
	}
}

// The select list and the destinations come from one declaration, so their
// lengths agree by construction. Asserted anyway, because that is the property
// the declaration exists to hold, and a refactor that reintroduced a second
// list would land here first.
func TestTheSelectListAndTheScanAgreeOnLength(t *testing.T) {
	columns := activityColumns("true")
	dests := activityScanTargets(&activityScan{})
	if len(dests) != len(activityProjection) {
		t.Fatalf("the scan asks for %d destinations and the projection declares %d columns",
			len(dests), len(activityProjection))
	}
	if columns == "" {
		t.Fatal("the select list rendered empty")
	}
}
