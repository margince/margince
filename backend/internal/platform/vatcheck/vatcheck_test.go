// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vatcheck

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A stand-in number, deliberately not a real company's: the assertions are
// about what this package does with an answer, and a fixture that IS somebody's
// VAT ID sends a test suite's traffic at a real register.
const someVAT = "DE123456789"

// serveVIES stands in for the Commission's endpoint and records what it was
// asked, so the request this package builds is provable rather than assumed.
func serveVIES(t *testing.T, status int, answer map[string]any) (*VIES, *checkRequest) {
	t.Helper()
	var asked checkRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the consultation this package sent: %v", err)
		}
		if err := json.Unmarshal(body, &asked); err != nil && len(body) > 0 {
			t.Errorf("the consultation was not the JSON the service takes: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if answer != nil {
			if err := json.NewEncoder(w).Encode(answer); err != nil {
				t.Errorf("writing the stand-in answer: %v", err)
			}
		}
	}))
	t.Cleanup(server.Close)
	client := NewVIES(server.URL, "DE999999999", server.Client())
	// The floor is real time, and no assertion here is about pacing, so it is
	// removed rather than waited out.
	client.pacer = NewPacer(0)
	return client, &asked
}

// The three answers are three different facts, and a caller's retry policy
// turns on which it got. Collapsing "the number is not real" into "the lookup
// failed" would have a company re-consulted forever over a settled answer.
func TestTheThreeAnswersAreToldApart(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		answer map[string]any
		want   Status
	}{
		{
			name:   "a live number",
			status: http.StatusOK,
			answer: map[string]any{"valid": true, "requestIdentifier": "WAPIAAAAXk3"},
			want:   StatusValid,
		},
		{
			name:   "a number the register does not hold",
			status: http.StatusOK,
			answer: map[string]any{"valid": false},
			want:   StatusInvalid,
		},
		{
			// VIES reports an unreachable member state as a userError on a 200,
			// so the status code alone does not say whether it answered.
			name:   "a member state that did not answer",
			status: http.StatusOK,
			answer: map[string]any{"valid": false, "userError": "MS_UNAVAILABLE"},
			want:   StatusUnavailable,
		},
		{
			name:   "the service itself unwell",
			status: http.StatusBadGateway,
			answer: nil,
			want:   StatusUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := serveVIES(t, tc.status, tc.answer)
			got, err := client.Check(context.Background(), someVAT)
			if err != nil {
				t.Fatalf("Check returned an error for an answered consultation: %v", err)
			}
			if got.Status != tc.want {
				t.Errorf("status = %q, want %q", got.Status, tc.want)
			}
		})
	}
}

// The consultation number is the whole reason this package stores anything.
// Losing it would leave a `valid` flag that proves nothing to a tax authority.
func TestTheConsultationNumberIsKept(t *testing.T) {
	const receipt = "WAPIAAAAXk3-not-a-real-receipt"
	client, _ := serveVIES(t, http.StatusOK, map[string]any{
		"valid":             true,
		"requestIdentifier": receipt,
		"name":              "Muster GmbH",
		"address":           "Musterstr. 1, Berlin",
	})

	got, err := client.Check(context.Background(), someVAT)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.ConsultationNumber != receipt {
		t.Errorf("consultation number = %q, want %q — the receipt is the evidence", got.ConsultationNumber, receipt)
	}
	if got.Name != "Muster GmbH" {
		t.Errorf("name = %q, want the register's answer kept", got.Name)
	}
}

// Asking under our own VAT number is what makes VIES issue a receipt, so the
// requester has to reach the wire. It travelled in the client and not the
// request once, which produced answers with no evidence attached.
func TestTheRequestNamesThisInstallationSoAReceiptIsIssued(t *testing.T) {
	client, asked := serveVIES(t, http.StatusOK, map[string]any{"valid": true})

	if _, err := client.Check(context.Background(), "DE 123.456-789"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if asked.RequesterMemberState != "DE" || asked.RequesterNumber != "999999999" {
		t.Errorf("the consultation named requester %q/%q, want DE/999999999",
			asked.RequesterMemberState, asked.RequesterNumber)
	}
	// The same number printed two ways is one number. An Impressum spaces and
	// dots it as readily as not, and two spellings would consult as two
	// companies.
	if asked.CountryCode != "DE" || asked.VatNumber != "123456789" {
		t.Errorf("consulted %q/%q, want DE/123456789 — separators must not reach the service",
			asked.CountryCode, asked.VatNumber)
	}
}

// An installation that has not stated its own VAT number still gets an answer.
// Refusing to check without one would turn a missing setting into a missing
// capability.
func TestAnInstallationWithNoVatNumberStillGetsAnAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"valid": true}); err != nil {
			t.Errorf("writing the stand-in answer: %v", err)
		}
	}))
	defer server.Close()
	client := NewVIES(server.URL, "", server.Client())
	client.pacer = NewPacer(0)

	got, err := client.Check(context.Background(), someVAT)
	if err != nil {
		t.Fatalf("Check without a requester number: %v", err)
	}
	if got.Status != StatusValid {
		t.Errorf("status = %q, want valid — a missing requester number is not a missing answer", got.Status)
	}
	if got.ConsultationNumber != "" {
		t.Errorf("consultation number = %q, want empty: VIES issues none without a requester", got.ConsultationNumber)
	}
}

// A refusal carries its own schedule, and a caller that treated it as an
// ordinary error would come back immediately and be refused again.
func TestARefusalIsDistinctAndCarriesItsSchedule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := NewVIES(server.URL, "DE999999999", server.Client())
	client.pacer = NewPacer(0)

	_, err := client.Check(context.Background(), someVAT)
	var refused *ProviderRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("a 429 gave %v, want a ProviderRefusedError the caller can schedule against", err)
	}
	if refused.RetryAfter != 2*time.Minute {
		t.Errorf("RetryAfter = %v, want 2m — the service's own instruction", refused.RetryAfter)
	}
}

// A 4xx is OUR request being wrong, not the register being down. Recorded as
// unavailable it would file our own mistake as the service's, and — because a
// recorded answer stops the number being re-asked — hide it for good.
func TestARefusedRequestIsNotAnUnavailableRegister(t *testing.T) {
	client, _ := serveVIES(t, http.StatusBadRequest, nil)

	got, err := client.Check(context.Background(), someVAT)
	if err == nil {
		t.Fatalf("a 400 answered %q with no error, want an error: a malformed request is not a register that declined", got.Status)
	}
	if got.Status == StatusUnavailable {
		t.Error("a 400 was recorded as an unavailable register, which files our own bad request as the service's outage")
	}
}

// The receipt's date is the REGISTER's. A worker clock skewed across midnight
// would file the proof under a day the consultation did not happen on.
func TestTheConsultationDateComesFromTheRegister(t *testing.T) {
	client, _ := serveVIES(t, http.StatusOK, map[string]any{
		"valid": true, "requestIdentifier": "WAPIAAAAXk6-stand-in",
		"requestDate": "2026-08-20+01:00",
	})

	got, err := client.Check(context.Background(), someVAT)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.RequestDate.IsZero() {
		t.Fatal("the service's own consultation date was dropped, so the receipt is dated by our clock instead")
	}
	if y, m, d := got.RequestDate.Date(); y != 2026 || m != time.August || d != 20 {
		t.Errorf("request date = %v, want 2026-08-20", got.RequestDate)
	}
}

// A service that sent no date is not an error: the answer stands, and the
// caller falls back to its own clock rather than storing nothing.
func TestAnAbsentConsultationDateIsNotAFailure(t *testing.T) {
	client, _ := serveVIES(t, http.StatusOK, map[string]any{"valid": true})

	got, err := client.Check(context.Background(), someVAT)
	if err != nil {
		t.Fatalf("Check without a request date: %v", err)
	}
	if got.Status != StatusValid {
		t.Errorf("status = %q, want valid — a missing date is not a missing answer", got.Status)
	}
	if !got.RequestDate.IsZero() {
		t.Errorf("request date = %v, want the zero time so the caller knows to use its own clock", got.RequestDate)
	}
}

// A number that is not VAT-shaped is answerable here, and asking somebody
// else's service about it would spend a request to be told what we knew.
func TestAMalformedNumberIsRefusedWithoutAsking(t *testing.T) {
	var asked bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewVIES(server.URL, "DE999999999", server.Client())
	client.pacer = NewPacer(0)

	for _, bad := range []string{"", "  ", "12", "123456789", "D1234"} {
		if _, err := client.Check(context.Background(), bad); !errors.Is(err, ErrMalformedNumber) {
			t.Errorf("Check(%q) = %v, want ErrMalformedNumber", bad, err)
		}
	}
	if asked {
		t.Error("a malformed number reached the service — its shape is answerable here")
	}
}

// Several registers answer "---" for a field they do not disclose. Stored
// verbatim it would render as a company's registered name.
func TestAnUndisclosedFieldIsAbsentRatherThanAPlaceholder(t *testing.T) {
	client, _ := serveVIES(t, http.StatusOK, map[string]any{
		"valid": true, "name": "---", "address": "---",
	})

	got, err := client.Check(context.Background(), someVAT)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got.Name != "" || got.Address != "" {
		t.Errorf("name/address = %q/%q, want both empty — a placeholder is an absence", got.Name, got.Address)
	}
}

// The pacer is a queue, not a suggestion: two consultations may not start
// together. Proved on a seam rather than a real clock, because a test that
// waited out the floor would be skipped and a skipped test proves nothing.
func TestTheSecondConsultationWaitsForTheFloor(t *testing.T) {
	clock := time.Unix(1_700_000_000, 0)
	var slept []time.Duration
	pacer := &Pacer{
		interval: 2 * time.Second,
		now:      func() time.Time { return clock },
		sleep: func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			clock = clock.Add(d)
			return nil
		},
	}

	if err := pacer.Wait(context.Background()); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	if len(slept) != 0 {
		t.Fatalf("the first consultation waited %v, want none", slept)
	}
	clock = clock.Add(500 * time.Millisecond)
	if err := pacer.Wait(context.Background()); err != nil {
		t.Fatalf("second Wait: %v", err)
	}
	if len(slept) != 1 || slept[0] != 1500*time.Millisecond {
		t.Errorf("the second consultation waited %v, want one wait of 1.5s", slept)
	}
}

// A caller who has given up must not hold the queue for everyone behind it.
func TestAnAbandonedConsultationReleasesTheQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pacer := NewPacer(time.Hour)
	if err := pacer.Wait(context.Background()); err != nil {
		t.Fatalf("first Wait: %v", err)
	}
	if err := pacer.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Wait on a cancelled context = %v, want context.Canceled", err)
	}
}

// The endpoint and verb are part of the contract with the service; a GET or a
// renamed path would fail at runtime against the real one only.
func TestTheConsultationIsPostedToTheCheckEndpoint(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"valid": true}); err != nil {
			t.Errorf("writing the stand-in answer: %v", err)
		}
	}))
	defer server.Close()
	client := NewVIES(server.URL, "DE999999999", server.Client())
	client.pacer = NewPacer(0)

	if _, err := client.Check(context.Background(), someVAT); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if method != http.MethodPost || !strings.HasSuffix(path, "/check-vat-number") {
		t.Errorf("consulted %s %s, want POST …/check-vat-number", method, path)
	}
}
