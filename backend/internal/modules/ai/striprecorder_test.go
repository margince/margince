// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"strings"
	"testing"
)

// The recorder accumulates across every body one traced attempt marshals, and
// reports the kinds once.
//
// Additive on purpose: a structured-output retry inside an adapter marshals a
// second body under the same attempt, and the row answers what left the process
// under this trace rather than what the last marshalling happened to hold.
func TestTheStripRecorderAccumulatesAcrossBodies(t *testing.T) {
	t.Parallel()
	rec := newStripRecorder(NewSecretStripper())
	const key = "AKIAQRSTUVWXYZABCDEF"
	for _, body := range []string{
		`{"text":"first ` + key + `"}`,
		`{"text":"second ` + key + ` and again ` + key + `"}`,
	} {
		if _, _, err := rec.Strip(context.Background(), []byte(body)); err != nil {
			t.Fatalf("Strip: %v", err)
		}
	}
	count, kinds := rec.report()
	if count != 3 {
		t.Errorf("recorded %d removals across two bodies, want 3", count)
	}
	if len(kinds) != 1 || kinds[0] != "aws_access_key" {
		t.Errorf("recorded kinds %v, want the one kind that fired, named once", kinds)
	}
}

// A call that removed nothing reports nothing — distinguishable from a call
// nobody recorded, because the column beside it says zero rather than being
// absent.
func TestTheStripRecorderReportsNothingWhenNothingMatched(t *testing.T) {
	t.Parallel()
	rec := newStripRecorder(NewSecretStripper())
	if _, _, err := rec.Strip(context.Background(), []byte(`{"text":"nothing secret here"}`)); err != nil {
		t.Fatalf("Strip: %v", err)
	}
	count, kinds := rec.report()
	if count != 0 || len(kinds) != 0 {
		t.Errorf("recorded %d removals %v on a clean body, want none", count, kinds)
	}
}

// It still strips. A recorder that counted and forgot to redact would pass a
// count assertion while putting the credential on the wire, which is the one
// way this wrapper could be worse than no wrapper at all.
func TestTheStripRecorderStillRedacts(t *testing.T) {
	t.Parallel()
	rec := newStripRecorder(NewSecretStripper())
	const key = "AKIAQRSTUVWXYZABCDEF"
	out, _, err := rec.Strip(context.Background(), []byte(`{"text":"`+key+`"}`))
	if err != nil {
		t.Fatalf("Strip: %v", err)
	}
	if strings.Contains(string(out), key) {
		t.Errorf("the credential survived the recording stripper: %s", out)
	}
}

// A nil inner strips nothing and records nothing, rather than panicking: the
// router wraps whatever the caller supplied, and a request may legitimately
// carry no stripper.
func TestTheStripRecorderToleratesNoStripper(t *testing.T) {
	t.Parallel()
	rec := newStripRecorder(nil)
	body := []byte(`{"text":"AKIAQRSTUVWXYZABCDEF"}`)
	out, _, err := rec.Strip(context.Background(), body)
	if err != nil {
		t.Fatalf("Strip: %v", err)
	}
	if string(out) != string(body) {
		t.Error("a nil stripper altered the body")
	}
	if count, _ := rec.report(); count != 0 {
		t.Errorf("a nil stripper recorded %d removals", count)
	}
}
