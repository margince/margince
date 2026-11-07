// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apideprecation

import (
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

func contract(t *testing.T, operation string) *openapi3.T {
	t.Helper()
	text := "openapi: 3.1.0\ninfo: {title: Test, version: 1.0.0}\npaths:\n"
	if operation == "" {
		text += "  {}\n"
	} else {
		text += "  /old:\n    get:\n      operationId: old\n      responses: {'200': {description: OK}}\n" + operation
	}
	doc, err := openapi3.NewLoader().LoadFromData([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

const published = "      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: '2026-01-31T12:00:00Z'}\n"

func TestRemovedOperationsNeedThePublishedWindowAndReleaseProof(t *testing.T) {
	at := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	after := contract(t, "")
	for _, tc := range []struct {
		name, op, release string
		at                time.Time
		allowed           bool
	}{
		{"unmarked", "      summary: Still supported\n", "v1.1.0", at, false},
		{"pending", "      deprecated: true\n", "v1.1.0", at, false},
		{"no release", published, "", at, false},
		{"patch", published, "v1.0.1", at, false},
		{"early", published, "v1.1.0", at.Add(-time.Second), false},
		{"elapsed", published, "v1.1.0", at, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckChanges(contract(t, tc.op), after, tc.release, tc.at)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%t error=%v", tc.allowed, err)
			}
		})
	}
}

func TestInvalidOrDetachedNoticeMetadataIsRefused(t *testing.T) {
	for _, op := range []string{
		"      x-deprecation: {release: v1.0.0, announced_at: '2026-01-31T12:00:00Z'}\n",
		"      deprecated: true\n      x-deprecation: {relese: v1.0.0}\n",
		"      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: nonsense}\n",
		"      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: '2026-01-31T12:00:00Z', sunset_at: '2026-02-01T12:00:00Z'}\n",
	} {
		if _, err := Notices(contract(t, op)); err == nil {
			t.Errorf("invalid metadata admitted: %s", op)
		}
	}
}

func TestPublishedNoticeDatesCannotBeResetOrBroughtForward(t *testing.T) {
	before := contract(t, published+"      summary: Recorded publication\n")
	at := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	for _, next := range []string{
		"      deprecated: true\n",
		"      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: '2025-12-31T12:00:00Z'}\n",
	} {
		if err := CheckChanges(before, contract(t, next), "", at); err == nil {
			t.Fatal("published announcement was reset")
		}
	}
	old := contract(t, "      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: '2026-01-31T12:00:00Z', sunset_at: '2026-05-01T12:00:00+01:00'}\n")
	next := contract(t, "      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: '2026-01-31T12:00:00Z', sunset_at: '2026-05-01T11:00:00Z'}\n")
	if err := CheckChanges(old, next, "", at); err != nil {
		t.Fatalf("same sunset instant in another offset refused: %v", err)
	}
}

func TestAnnouncementIdentityUsesThePublicationInstant(t *testing.T) {
	before := contract(t, published)
	next := contract(t, "      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: '2026-01-31T13:00:00+01:00'}\n")
	at := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	if err := CheckChanges(before, next, "", at); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedNoticesRequireAnEarlierMarkerAndAPastPublication(t *testing.T) {
	at := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	if err := CheckChanges(contract(t, "      summary: Supported\n"), contract(t, published), "", at); err == nil {
		t.Fatal("marker and past publication introduced together")
	}
	if err := CheckChanges(contract(t, ""), contract(t, published), "", at); err == nil {
		t.Fatal("new operation claimed a past announcing release")
	}
	pending := contract(t, "      deprecated: true\n")
	if err := CheckChanges(pending, contract(t, published), "", time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("future publication admitted")
	}
	if err := CheckChanges(pending, contract(t, published), "", at); err != nil {
		t.Fatal(err)
	}
	if err := CheckChanges(contract(t, published), contract(t, "      summary: Supported again\n"), "", at); err != nil {
		t.Fatalf("withdrawing deprecation refused: %v", err)
	}
}
