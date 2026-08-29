// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package vatcheck asks the EU whether a VAT identification number is real.
//
// It exists because an Impressum states a USt-IdNr under a statutory duty and
// a CRM copying it records only what a page said. VIES — the Commission's VAT
// Information Exchange System — answers two further questions: is this number
// live in its member state's register, and whose is it.
//
// The second answer is the one worth reading. A number that validates to a
// company nobody recognises is the finding, not a formality, and it is exactly
// the case a page copied from a template produces.
//
// THE CONSULTATION NUMBER IS THE POINT. VIES issues a receipt for a check made
// under the requester's own VAT number, and that receipt is what a business
// shows to say it verified its counterpart before treating a supply as
// intra-community (Art. 138 VAT Directive). Storing `valid: true` proves
// nothing to anybody; storing the receipt, the date and the number consulted
// is evidence. An installation that has not stated its own VAT number still
// gets an answer — just no receipt with it.
package vatcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
)

// Status is what the register said about a number.
type Status string

// The three things a consultation can conclude.
const (
	// StatusValid and StatusInvalid are ANSWERS: the member state's register
	// was reached and reported on the number.
	StatusValid   Status = "valid"
	StatusInvalid Status = "invalid"
	// StatusUnavailable is the service declining to answer — a member state's
	// register offline, or VIES itself refusing. A fact about the LOOKUP, never
	// about the company, and the difference matters because only one of the
	// three is worth showing a user as a problem with their record.
	StatusUnavailable Status = "unavailable"
)

// Result is one consultation.
type Result struct {
	Status Status
	// ConsultationNumber is the receipt, empty when VIES issued none.
	ConsultationNumber string
	// Name and Address are what the register holds, when the member state
	// discloses them. Several disclose neither, which is not an error.
	Name    string
	Address string
	// RequestDate is the date VIES stamped the consultation with. It is the
	// service's, not ours: a receipt names the day the register was asked, and
	// a worker's clock skewed across midnight would file the proof under the
	// wrong date. Zero when the service sent none, and the caller then falls
	// back to its own clock rather than storing no date at all.
	RequestDate time.Time
}

// PublicBaseURL is the Commission's own REST service.
const PublicBaseURL = "https://ec.europa.eu/taxation_customs/vies/rest-api"

// RecurringInterval is the floor between requests.
//
// VIES publishes no numeric rate limit; it throttles by IP and blocks abusers,
// and its terms describe the service as intended for occasional verification
// rather than bulk lookup. Two seconds is what an ingestion-rate client needs
// (a company is checked when its imprint is read, not in a sweep) and is far
// under any rate a human-driven CRM produces.
const RecurringInterval = 2 * time.Second

// UserAgent names this software to the service.
const UserAgent = outbound.EnrichHeader

// ErrNotConfigured says this deployment checks no VAT numbers. A REFUSAL
// rather than a failure: an offline or demo installation reaches no EU service
// on purpose, and the caller records that instead of retrying.
var ErrNotConfigured = errors.New("vatcheck: no VAT verification service is configured")

// ErrMalformedNumber says the input was not VAT-ID shaped, so no request was
// made. A number typed into a form is client input and its shape is answerable
// here without spending somebody else's service on it.
var ErrMalformedNumber = errors.New("vatcheck: not a VAT identification number")

// Checker consults a register.
//
// An INVALID number is not an error: the service did its job and the answer is
// "no such number", which is a fact about the company's page and must be
// recorded as one rather than retried forever. err is reserved for a
// consultation that did not complete.
type Checker interface {
	Check(ctx context.Context, vatNumber string) (Result, error)
}

// ProviderRefusedError is the service turning this installation away — a 429
// or a block — carrying its own instruction about when to come back.
type ProviderRefusedError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *ProviderRefusedError) Error() string {
	return fmt.Sprintf("vatcheck: the service refused the consultation (status %d)", e.Status)
}

// VIES is the Commission's service.
type VIES struct {
	baseURL string
	http    *http.Client
	pacer   *Pacer
	// requester is this installation's OWN VAT number, in two parts. Supplying
	// it is what makes VIES issue a consultation number, so an installation
	// that states it gets evidence and one that does not gets a bare answer.
	requesterCountry string
	requesterNumber  string
}

// NewVIES builds the client. An empty baseURL means the public service.
//
// The pacer is created here and held by the client, so ONE client is one
// requester against the one service every request goes to. The composition
// root builds exactly one.
func NewVIES(baseURL, requesterVAT string, httpClient *http.Client) *VIES {
	if baseURL == "" {
		baseURL = PublicBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	country, number := splitVAT(requesterVAT)
	return &VIES{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		http:             httpClient,
		pacer:            NewPacer(RecurringInterval),
		requesterCountry: country,
		requesterNumber:  number,
	}
}

// checkRequest is what VIES's REST endpoint takes.
//
//nolint:tagliatelle // the keys are the Commission's, on their wire; renaming them to house style would just fail to consult
type checkRequest struct {
	CountryCode          string `json:"countryCode"`
	VatNumber            string `json:"vatNumber"`
	RequesterMemberState string `json:"requesterMemberStateCode,omitempty"`
	RequesterNumber      string `json:"requesterNumber,omitempty"`
}

// checkResponse is the field set this package reads back. The service returns
// more; taking only these keeps the parse stable across its versions.
//
//nolint:tagliatelle // the keys are the Commission's, as sent; renaming them would decode nothing
type checkResponse struct {
	Valid              bool   `json:"valid"`
	Name               string `json:"name"`
	Address            string `json:"address"`
	RequestIdentifier  string `json:"requestIdentifier"`
	RequestDate        string `json:"requestDate"`
	UserError          string `json:"userError"`
	TraderNameMatch    string `json:"traderNameMatch"`
	TraderAddressMatch string `json:"traderAddressMatch"`
}

// Check consults the register about one number.
//
// Three outcomes, and telling them apart is the caller's whole retry policy: an
// answer (valid or invalid — recorded, never re-asked on a schedule), a service
// that declined (StatusUnavailable — a fact about the lookup), and a
// consultation that did not complete (an error — retried, and after a refusal
// on the service's own schedule).
func (v *VIES) Check(ctx context.Context, vatNumber string) (Result, error) {
	country, number := splitVAT(vatNumber)
	if country == "" || number == "" {
		return Result{}, ErrMalformedNumber
	}
	// The pace is taken BEFORE the request is built, and it is what makes this
	// client safe to run on ingestion.
	if err := v.pacer.Wait(ctx); err != nil {
		return Result{}, err
	}

	body, err := json.Marshal(checkRequest{
		CountryCode:          country,
		VatNumber:            number,
		RequesterMemberState: v.requesterCountry,
		RequesterNumber:      v.requesterNumber,
	})
	if err != nil {
		return Result{}, fmt.Errorf("vatcheck: building the consultation: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		v.baseURL+"/check-vat-number", strings.NewReader(string(body)))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := v.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("vatcheck: asking the service: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close: the decode below reads what it needs and may leave the body mid-stream, so a close error says nothing about what the register answered
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		return Result{}, &ProviderRefusedError{
			Status:     resp.StatusCode,
			RetryAfter: retryafter.Of(resp),
		}
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// A 4xx is OUR request being wrong — a malformed number, or a requester
		// this installation configured badly. Recording it as "the register was
		// unavailable" would file our own mistake as the service's, and the
		// equality predicate would then never ask about that number again.
		return Result{}, fmt.Errorf("vatcheck: the service refused the request as malformed (status %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		// A 5xx is the service being unwell, which is the same fact as a member
		// state's register being down: not an answer, and not something a retry
		// loop should chase indefinitely.
		return Result{Status: StatusUnavailable}, nil
	}

	var answer checkResponse
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		return Result{}, fmt.Errorf("vatcheck: reading the service's answer: %w", err)
	}
	// VIES reports a member state's register being unreachable as a userError
	// on a 200, so the status code alone does not say whether it answered.
	if unavailable(answer.UserError) {
		return Result{Status: StatusUnavailable}, nil
	}
	result := Result{
		Status:             StatusInvalid,
		ConsultationNumber: strings.TrimSpace(answer.RequestIdentifier),
		Name:               cleanRegisterField(answer.Name),
		Address:            cleanRegisterField(answer.Address),
		RequestDate:        parseRequestDate(answer.RequestDate),
	}
	if answer.Valid {
		result.Status = StatusValid
	}
	return result, nil
}

// serviceUnavailableErrors are the userError values that mean VIES could not
// reach an answer. Anything else on a 200 — including "INVALID_INPUT" — is the
// service answering about the number.
var serviceUnavailableErrors = map[string]bool{
	"MS_UNAVAILABLE":            true,
	"MS_MAX_CONCURRENT_REQ":     true,
	"SERVICE_UNAVAILABLE":       true,
	"TIMEOUT":                   true,
	"SERVER_BUSY":               true,
	"GLOBAL_MAX_CONCURRENT_REQ": true,
}

func unavailable(userError string) bool {
	return serviceUnavailableErrors[strings.ToUpper(strings.TrimSpace(userError))]
}

// parseRequestDate reads the date VIES stamped the consultation with. The
// service has sent it in more than one shape over the years, so each is tried
// and an unparseable one is an absence rather than an error: the answer is
// still an answer, and the caller falls back to its own clock.
func parseRequestDate(raw string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02-07:00", time.DateOnly} {
		if at, err := time.Parse(layout, strings.TrimSpace(raw)); err == nil {
			return at
		}
	}
	return time.Time{}
}

// cleanRegisterField normalises what a register returned. Several answer "---"
// for a field they do not disclose, which is an absence spelled as text and
// would otherwise be stored as a company's name.
func cleanRegisterField(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "---" || trimmed == "-" {
		return ""
	}
	return trimmed
}

// splitVAT separates the two-letter member state prefix from the rest.
//
// Spaces, dots and hyphens come off first: an Impressum prints
// "DE 123 456 789" as readily as "DE123456789", and the same company must not
// consult as two different numbers.
func splitVAT(raw string) (country, number string) {
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '.' || r == '-' || r == '/' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(raw)))
	if len(cleaned) < 3 {
		return "", ""
	}
	country = cleaned[:2]
	number = cleaned[2:]
	for _, r := range country {
		if r < 'A' || r > 'Z' {
			return "", ""
		}
	}
	return country, number
}
