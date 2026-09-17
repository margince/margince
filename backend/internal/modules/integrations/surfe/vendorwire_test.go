// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package surfe

// The vendor's names, held against the vendor's own reference.
//
// Every other test in this package spells Surfe's path and its array field as
// Go string literals. That made them agree with whatever the adapter said: a
// rename sweeping the tree rewrote `people` to `contacts` in the adapter AND
// in the assertions, so the suite stayed green while every real run answered
// 404. A test that changes in lockstep with the code under test proves
// nothing about a third party.
//
// So the expected values live in testdata/vendor-wire.json, recorded
// from Surfe's published reference. It is data, not source, and nothing that
// rewrites identifiers in this tree reaches it. What the checks below do is
// drive the REAL adapter and ask whether what left, and what it could read,
// match what the vendor documents.
//
// What this still cannot see: a vendor that changes its API without this file
// being re-recorded. Nothing offline can. The recording carries the date it
// was read and the pages it came from, so the answer to a suspected drift is
// to read them again — and the live contract check that would close the gap
// needs a key and a budget, which is a decision this package does not own.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

type vendorCall struct {
	Method               string          `json:"method"`
	Path                 string          `json:"path"`
	RequestSubjectsField string          `json:"request_subjects_field"`
	Response             json.RawMessage `json:"response"`
}

type vendorWire struct {
	Start   vendorCall `json:"start"`
	Get     vendorCall `json:"get"`
	Credits vendorCall `json:"credits"`
}

func recordedVendorWire(t *testing.T) vendorWire {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "vendor-wire.json"))
	if err != nil {
		t.Fatalf("reading the recorded vendor wire: %v", err)
	}
	var wire vendorWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("the recording is not readable: %v", err)
	}
	// Every call the adapter makes, not only the one that broke: a recording
	// covering part of the surface reports PASS over the part it does not
	// reach, and there is no failing assertion to notice that.
	if wire.Start.Path == "" || wire.Get.Path == "" || wire.Credits.Path == "" || wire.Start.RequestSubjectsField == "" {
		t.Fatal("the recording names no path or no subjects field, so every check below would pass on nothing")
	}
	return wire
}

// What leaves goes to the path the vendor serves, carrying the subjects under
// the name the vendor reads.
func TestSubmitSpeaksTheRecordedVendorWire(t *testing.T) {
	wire := recordedVendorWire(t)
	rec := &recorder{status: http.StatusAccepted, body: string(wire.Start.Response)}

	sub, err := testAdapter(rec).Submit(context.Background(), provider.Credential("k"), aRequest())
	if err != nil {
		t.Fatal(err)
	}
	if sub.Outcome != provider.OutcomeAccepted {
		t.Fatalf("the vendor's own start response read as %q, want accepted", sub.Outcome)
	}
	if sub.ProviderJobID == "" {
		t.Error("no handle came out of the vendor's own start response, so a started enrichment could never be polled")
	}
	if got := rec.seen.Method; got != wire.Start.Method {
		t.Errorf("submitted with %s, the vendor serves %s", got, wire.Start.Method)
	}
	if got := rec.seen.URL.Path; got != wire.Start.Path {
		t.Errorf("posted to %q; the vendor serves %q — a path it does not serve answers 404, and the connection goes to provider_error", got, wire.Start.Path)
	}

	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rec.sent), &sent); err != nil {
		t.Fatalf("the request body is not an object: %v", err)
	}
	if _, ok := sent[wire.Start.RequestSubjectsField]; !ok {
		t.Errorf("the body carries %v; the vendor reads the subjects from %q, and a body without it enriches nobody",
			keysOf(sent), wire.Start.RequestSubjectsField)
	}
}

// And what comes back is read under the names the vendor answers with. A
// response whose array this adapter cannot find decodes to an empty slice,
// which is indistinguishable from a genuine no-match — a charged run reported
// as having found nobody.
func TestPollReadsTheRecordedVendorResponse(t *testing.T) {
	wire := recordedVendorWire(t)
	rec := &recorder{status: http.StatusOK, body: string(wire.Get.Response)}

	got, err := testAdapter(rec).Poll(context.Background(), provider.Credential("k"), "enr-9")
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != provider.OutcomeCompleted {
		t.Fatalf("the vendor's own completed response read as %q — the result array was not found under the name the vendor answers with", got.Outcome)
	}
	if got.Result == nil || len(got.Result.Claims) == 0 {
		t.Fatal("a completed enrichment yielded no claims, so a charged run delivers nothing")
	}
	if got := rec.seen.Method; got != wire.Get.Method {
		t.Errorf("polled with %s, the vendor serves %s", got, wire.Get.Method)
	}
	// The recorded path names the handle as `{id}`; what matters is the rest.
	if prefix := filepath.Dir(wire.Get.Path); rec.seen.URL.Path != filepath.Join(prefix, "enr-9") {
		t.Errorf("polled %q; the vendor serves %q", rec.seen.URL.Path, wire.Get.Path)
	}
}

// The balance read is the same class of string and was not what broke, which
// is the reason to hold it: a recording covering only the call that failed
// would leave the next one to fail the same way.
func TestCreditsReadsTheRecordedVendorBalance(t *testing.T) {
	wire := recordedVendorWire(t)
	rec := &recorder{status: http.StatusOK, body: string(wire.Credits.Response)}

	got, err := testAdapter(rec).Credits(context.Background(), provider.Credential("k"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Balances) == 0 {
		t.Fatal("the vendor's own balance read as no pools at all, which refuses every run this key could still pay for")
	}
	for pool, balance := range got.Balances {
		if balance == 0 {
			t.Errorf("the %s pool read as empty from a balance the vendor answers with credits in", pool)
		}
	}
	if seen := rec.seen.URL.Path; seen != wire.Credits.Path || rec.seen.Method != wire.Credits.Method {
		t.Errorf("read the balance with %s %q; the vendor serves %s %q",
			rec.seen.Method, seen, wire.Credits.Method, wire.Credits.Path)
	}
}

func keysOf(object map[string]json.RawMessage) []string {
	names := make([]string, 0, len(object))
	for name := range object {
		names = append(names, name)
	}
	return names
}
