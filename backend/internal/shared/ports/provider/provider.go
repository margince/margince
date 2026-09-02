// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package provider is the licensed-data-provider seam (ARCH-SEAM-17): two
// interfaces facing opposite directions, in one package because they describe
// one relationship from its two ends.
//
// Adapter faces OUT. Each vendor implementation satisfies it, translating a
// frozen request into that vendor's wire protocol and its answer back into the
// bounded shape below. An adapter never decides policy, never widens what was
// requested, and never sees the database.
//
// RunService faces IN. A domain module calls it to queue and read runs, and
// sees nothing else — not the adapter, not the ledger, not the budget. That is
// what lets modules/people ask for an enrichment without depending on
// modules/integrations, which the module DAG forbids.
//
// This package is Tier-0 and stdlib-only, so nothing here names a database
// handle or a transaction. The callbacks that DO need one (writing claims,
// fencing a subject) are declared by modules/integrations as func types and
// supplied by compose — see that module's doc.go.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrNotConnected reports that no connection exists for the provider, or that
// the installation has none at all. It is a supported state, not a fault: the
// domain surface renders "no provider connected" and stays fully available.
var ErrNotConnected = errors.New("provider: no connected provider")

// Transport is how a provider delivers a result (PI-PARAM-4). The run-state
// machine is identical for all three; only the adapter's mechanics differ.
type Transport string

const (
	// TransportSynchronous answers in the submit call itself. Such an adapter
	// MUST write its claims inside the terminal transaction — it has no
	// re-readable handle, so a hand-off that failed separately could not be
	// recovered (PI-PARAM-10).
	TransportSynchronous Transport = "synchronous"
	// TransportPolled issues a job handle and answers later. Surfe is the
	// only surveyed provider of this kind.
	TransportPolled Transport = "polled"
	// TransportCallback would push its result to us. Declared so the
	// vocabulary is closed; no adapter uses it and no machinery exists.
	TransportCallback Transport = "callback"
)

// BillingBasis is what the provider charges for (PI-FORM-1). It decides what
// reconciliation does with a reservation once the outcome is known.
type BillingBasis string

const (
	// BillingPerSuccessfulResult charges only for a match, so a no-match
	// releases the whole reservation.
	BillingPerSuccessfulResult BillingBasis = "per_successful_result"
	// BillingPerRequest charges whether or not anything matched, so a
	// no-match releases nothing — there was no refund to pass on.
	BillingPerRequest BillingBasis = "per_request"
	// BillingPerRecordSubscription charges once per record for a window.
	// Honouring it needs a per-subject entitlement ledger that is not yet
	// specified, so RegisterableBilling rejects it rather than silently
	// charging full price for a re-read the customer already owns.
	BillingPerRecordSubscription BillingBasis = "per_record_subscription"
	// BillingUnmetered charges nothing per call. No reservation is taken;
	// the daily run ceiling still applies, because rate limits are real.
	BillingUnmetered BillingBasis = "unmetered"
)

// Issuance is whether a customer can mint the key themselves (PI-PARAM-12).
type Issuance string

const (
	IssuanceSelfService Issuance = "self_service"
	// IssuanceManual providers issue out of band (Zefix, by email). The card
	// must say so and must not promise immediate activation.
	IssuanceManual Issuance = "manual"
)

// Category is one kind of value a provider sells. The vocabulary is the
// provider's own, declared in its descriptor — never a shared enum, because
// no two providers slice their offering the same way.
type Category string

// Pool is one credit bucket a provider meters against, likewise its own.
type Pool string

// Cascade is a follow-up request the frozen policy permits when a primary
// category comes back empty — Surfe's personal-email fallback is the worked
// case. Its cost is reserved up front with everything else, so a cascade
// never needs a second ceiling check mid-run.
type Cascade struct {
	// Category is what the cascade fetches.
	Category Category
	// After names the category whose empty answer triggers it.
	After Category
	// Cost is what issuing it charges, per pool.
	Cost map[Pool]int
	// Excludes names categories this cascade cannot carry: Surfe's personal
	// pass returns no mobile, so requesting one alongside it is a lie.
	Excludes []Category
}

// Link is a titled URL for the settings disclosure (terms, DPA, security).
type Link struct {
	Label string
	URL   string
}

// Descriptor is everything the platform knows about a provider without
// calling it: the nine declared fields of PI-PARAM-8, plus the credential
// issuance mode and the category vocabulary a saved configuration validates
// against. A second provider fills this in rather than copying a paragraph.
type Descriptor struct {
	// Name is the closed discriminator (PI-PARAM-1), e.g. "surfe".
	Name string

	Transport Transport
	Billing   BillingBasis
	// CreditPools are the buckets this provider meters, its own names.
	CreditPools []Pool
	// CostTable is the base cost of requesting one category, per pool.
	CostTable map[Category]map[Pool]int
	Cascades  []Cascade
	// Identifiers names exactly what may leave the installation for this
	// provider. It is disclosure copy AND the egress contract.
	Identifiers []string
	// MatchRules is the same fact as Identifiers in a form admission can
	// apply: the combinations this provider can find somebody BY. A subject
	// satisfying none of them is skipped as no_identifiers instead of being
	// sent, because the vendor rejects such a request and the platform can
	// only read that rejection as a provider fault.
	//
	// Empty means the adapter declares no rule, and every subject is sent.
	MatchRules []MatchRule
	// EgressHost is the single allowlisted host this adapter may reach.
	EgressHost string
	// Verification names the cheapest read used to validate a credential at
	// connect time. Empty means the provider declares none.
	Verification string
	TermsLinks   []Link
	Issuance     Issuance

	// Categories is the full vocabulary; Presets are the named subsets the
	// settings card offers. "custom" is free choice within Categories.
	Categories    []Category
	Presets       map[string][]Category
	DefaultPreset string

	// Answers names the claim keys that satisfy each category: what a run has
	// to come back with for that category to count as answered.
	//
	// Declared by the adapter because only it knows the correspondence. The
	// two vocabularies are deliberately separate — a category is what you buy,
	// a claim is what arrives — and they do not match name for name: Surfe's
	// `professional_email` category is answered by the `professional_emails`
	// claim, and a category may be answered by more than one key.
	//
	// What it buys the reader: a category requested and NOT among the answers
	// is one the provider had nothing for, which is a different fact from one
	// nobody asked for. Without this the page shows a run that answered one
	// category out of six as a plain success with five blank fields.
	//
	// A category absent from this map is never reported as unanswered — an
	// adapter that has not declared its correspondence stays silent rather
	// than accusing a provider of withholding what it may well have sent.
	Answers map[Category][]ClaimKey

	// RequiresAnswerTo names, per category, another category that must come
	// back with something or this one is never put to the provider at all.
	//
	// Surfe's is the worked case: it skips the mobile lookup entirely when it
	// found no email, because a subject it cannot place has no number either
	// and asking would spend a mobile credit on a lookup already known to have
	// failed. The credit is saved, and no request is made.
	//
	// Declared so a reader is never told the provider "had nothing" for a
	// question nobody asked. It is the mirror of Cascades, which name what
	// runs only when something comes back EMPTY; this names what runs only
	// when something comes back FULL.
	RequiresAnswerTo map[Category]Category
}

// Credential is raw key material, alive only for the duration of one call.
// It is resolved from the vault at the execution boundary and never stored on
// anything that outlives the request.
type Credential []byte

// PersonIdentifiers is the closed set of facts that may be sent about a
// person. Surfe accepts a LinkedIn URL, or a name plus a company — and
// nothing else (PI-PARAM-11). A field absent here cannot leave.
type PersonIdentifiers struct {
	LinkedInURL   string
	FirstName     string
	LastName      string
	CompanyName   string
	CompanyDomain string
}

// Request is one submission, built from the run's frozen snapshot.
type Request struct {
	// CorrelationID is the run's external correlation id — an opaque handle
	// carrying no subject identity, safe to hand a third party.
	CorrelationID string
	Identifiers   PersonIdentifiers
	Categories    []Category
	Cascades      []Cascade
}

// Outcome classifies an adapter's answer in provider-neutral terms. It is a
// closed vocabulary because the run state machine switches on it.
type Outcome string

const (
	// OutcomeAccepted means a polled provider took the job and issued a handle.
	OutcomeAccepted Outcome = "accepted"
	// OutcomePending means a poll found the job still running.
	OutcomePending   Outcome = "pending"
	OutcomeCompleted Outcome = "completed"
	OutcomeNoMatch   Outcome = "no_match"

	OutcomeInvalidCredentials  Outcome = "invalid_credentials" //nolint:gosec // G101 false positive: an outcome NAME the adapter returns, not a credential
	OutcomeInsufficientCredits Outcome = "insufficient_credits"
	OutcomeRateLimited         Outcome = "rate_limited"
	OutcomeProviderError       Outcome = "provider_error"

	// OutcomeAmbiguous is the adapter saying "I do not know whether that
	// landed" — a timeout, or a response it could not read. It maps to the
	// submission_unknown run state, holds the reservation, and is NEVER
	// retried: a retry is how one ambiguous charge becomes two certain ones.
	OutcomeAmbiguous Outcome = "ambiguous"
)

// Terminal reports whether an outcome ends the run's dealings with the
// provider. Accepted and Pending are the only two that do not.
func (o Outcome) Terminal() bool {
	return o != OutcomeAccepted && o != OutcomePending
}

// ClaimKey is the bounded vocabulary of what a run can assert about a person.
// It matches the person_provider_claim CHECK constraint exactly.
type ClaimKey string

const (
	ClaimProfessionalEmails ClaimKey = "professional_emails"
	ClaimPersonalEmails     ClaimKey = "personal_emails"
	ClaimMobilePhones       ClaimKey = "mobile_phones"
	ClaimLinkedInProfile    ClaimKey = "linkedin_profile"
	ClaimCurrentEmployment  ClaimKey = "current_employment"
	ClaimJobHistory         ClaimKey = "job_history"
	ClaimLocation           ClaimKey = "location"
	ClaimDepartments        ClaimKey = "departments"
	ClaimSeniorities        ClaimKey = "seniorities"
)

// Claim is one normalized assertion. Value is the typed wire shape for the
// key; the adapter has already dropped whatever the provider sent that we did
// not ask for, so no raw payload is retained.
type Claim struct {
	Key        ClaimKey
	Value      json.RawMessage
	Confidence *float64
}

// Result is a completed provider answer.
type Result struct {
	Claims []Claim
	// PoolSpend is what the provider says it actually charged, per pool —
	// the input to reconciliation. Absent means "the provider did not say",
	// and the reservation stands as spent.
	PoolSpend map[Pool]int
}

// Submission is the answer to a submit call.
type Submission struct {
	Outcome Outcome
	// ProviderJobID is set by a polled provider that accepted the work. It
	// is the handle the recovery path re-reads, so it is stored even though
	// it never appears on the wire.
	ProviderJobID string
	// Result is set only by a synchronous provider answering in one call.
	Result *Result
	// SafeStatusCode is a closed product reason, never a provider body.
	SafeStatusCode string
}

// PollStatus is the answer to a poll.
type PollStatus struct {
	Outcome        Outcome
	Result         *Result
	SafeStatusCode string
}

// Credits is a provider's own ledger reading. Margince reports it as the
// provider's number and never as its own: the customer may spend the same
// credits through the provider's other apps.
type Credits struct {
	Balances map[Pool]int
	ReadAt   time.Time
}

// Adapter is what each vendor implementation satisfies.
type Adapter interface {
	// Descriptor is static: what this provider is, costs and may receive.
	Descriptor() Descriptor

	// VerifyCredential performs the declared verification call. It is the
	// only provider call allowed on an HTTP request path, because a key is
	// committed ONLY after it succeeds (PI-AC-1). For a provider declaring
	// no verification call, it performs the cheapest documented lookup —
	// being exempt from verification is not an option.
	VerifyCredential(ctx context.Context, cred Credential) (Credits, error)

	// Credits reads the balance. For Surfe this is the same call as
	// VerifyCredential, which is why the descriptor names it.
	Credits(ctx context.Context, cred Credential) (Credits, error)

	// Submit issues the request. A synchronous adapter returns a terminal
	// Submission with Result set; a polled one returns OutcomeAccepted and a
	// ProviderJobID.
	Submit(ctx context.Context, cred Credential, req Request) (Submission, error)

	// Poll re-reads a result by handle. It is also the recovery read: an
	// adapter must be able to re-serve a completed result by job id, which
	// is why no payload is ever parked in a queue between attempts.
	Poll(ctx context.Context, cred Credential, providerJobID string) (PollStatus, error)
}

// Reservation is one pool's hold for one run.
type Reservation struct {
	Pool     Pool
	Reserved int
	Actual   *int
}

// Snapshot is the effective configuration a run was admitted under, frozen at
// queue time. A later settings change cannot widen it: the run spends what it
// was authorized to spend, not what the connection permits now (PI-AC-2).
type Snapshot struct {
	Mode             string
	Preset           string
	Categories       []Category
	AutomaticCreate  bool
	AutomaticImport  bool
	RefreshAfterDays *int
	DailyRunLimit    *int
}

// Run is the wire-facing view of a run.
type Run struct {
	Snapshot        Snapshot
	ID              string
	SubjectKind     string
	PersonID        string
	Provider        string
	Trigger         Trigger
	State           RunState
	SkipReason      SkipReason
	ClaimsUnwritten bool
	// Applied says the run's answers reached the subject's own record, which
	// is not the same fact as completed: a paid, complete run whose values are
	// still only beside the record leaves the page looking empty, and a client
	// that stopped watching at completed would show it that way.
	Applied             bool
	ConnectionVersion   int64
	RequestedCategories []Category
	Reservations        []Reservation
	SafeStatusCode      string
	SubmittedAt         *time.Time
	CompletedAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// QueueInput is a request to enrich one subject.
type QueueInput struct {
	PersonID string
	// Provider empty means "every connected provider that admits this
	// trigger" — the event consumer's case, which should not have to know
	// what is registered.
	Provider    string
	Trigger     Trigger
	RequestedBy string
	// Categories narrows this ONE run to a subset of what the connection
	// permits. Empty means the connection's own selection, which is what every
	// automatic trigger uses.
	//
	// It can only ever narrow. The connection is the ceiling an admin set, and
	// a request naming a category outside it is refused rather than trimmed —
	// a rep must not spend on something an admin switched off, and silently
	// dropping it would buy less than the caller asked for while answering as
	// though it had complied.
	//
	// What it is FOR: automatic enrichment takes the free categories on
	// everybody, and a human presses a button to buy a priced one for a named
	// person. Without a per-run set, that button could only change the setting
	// for every future run too.
	Categories []Category
}

// RunService is what a domain module sees. modules/integrations implements it.
type RunService interface {
	// QueueRun admits, fences, freezes and reserves in ONE transaction, then
	// returns. It never calls a provider: the submission is a durable job,
	// which is what lets the HTTP surface answer 202 immediately. A duplicate
	// trigger for a live (subject, provider, fingerprint) returns the
	// existing run rather than buying the same data twice.
	QueueRun(ctx context.Context, in QueueInput) (Run, error)

	// GetRun reads one run for a subject.
	GetRun(ctx context.Context, personID, runID string) (Run, error)
}

// NotConnected is the RunService used when no provider is wired. Every call
// answers ErrNotConnected, which the domain surfaces render as the honest
// "no provider connected" state — and, crucially, zero outbound calls occur
// (PI-AC-9). It exists so "not connected" is a supported configuration
// proven by construction rather than by a runtime branch someone might miss.
type NotConnected struct{}

func (NotConnected) QueueRun(context.Context, QueueInput) (Run, error) {
	return Run{}, ErrNotConnected
}

func (NotConnected) GetRun(context.Context, string, string) (Run, error) {
	return Run{}, ErrNotConnected
}
