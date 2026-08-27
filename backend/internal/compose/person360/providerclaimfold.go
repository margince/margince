// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// Folding stored claims into the rendered snapshot.
//
// Each claim's value_json is the shape the adapter normalized it to, which is
// why this decodes per key rather than into one struct: the categories a
// provider sells have nothing in common beyond belonging to the same person.
//
// Nothing here invents a value. A field the provider did not return is absent,
// not empty — "we asked and they had nothing" and "we never asked" are
// different facts, and the categories_not_requested list carries the second.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// storedClaim is one row as the fold reads it.
type storedClaim struct {
	key         string
	value       []byte
	confidence  *float64
	retrievedAt time.Time
	provider    string
}

// foldClaims reads every retained claim for this person and folds it into the
// profile. Ordered oldest-first so a later run's answer for the same category
// lands after an earlier one — the page shows both, and the newest reads last.
func (s *Service) foldClaims(ctx context.Context, tx pgx.Tx, personID ids.PersonID, out *crmcontracts.PersonProviderProfile) error {
	rows, err := tx.Query(ctx, `
		SELECT claim_key, value_json, confidence, retrieved_at, provider
		  FROM person_provider_claim
		 WHERE person_id = $1
		 ORDER BY retrieved_at`, personID)
	if err != nil {
		return fmt.Errorf("person360: reading the provider claims: %w", err)
	}
	defer rows.Close()
	var claims []storedClaim
	for rows.Next() {
		var c storedClaim
		if err := rows.Scan(&c.key, &c.value, &c.confidence, &c.retrievedAt, &c.provider); err != nil {
			return fmt.Errorf("person360: scanning a provider claim: %w", err)
		}
		claims = append(claims, c)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("person360: reading the provider claims: %w", err)
	}
	for _, c := range claims {
		if err := foldOne(c, out); err != nil {
			return err
		}
	}
	return nil
}

// statableConfidence reports whether a score is one this contract can carry: a
// proportion, zero through one.
//
// HERE rather than at each provider, because this is the one place a PROVIDER
// CLAIM's confidence reaches the contract, and every claim source passes
// through it. A check at each provider would be one per provider, and the next
// one added would not have it.
//
// The value arrives through `value_json`, which carries no CHECK. The
// `confidence` COLUMN beside it does — and is not where this number comes from,
// which is what made the path look guarded while carrying none of the traffic.
// A vendor scoring out of 100 would put 87 on a field the contract declares
// `maximum: 1`, and the page renders a confidence as a percentage.
//
// This is HARDENING, not a live defect: every Surfe score in this tree is a
// proportion. The number is vendor-controlled and nothing validated it, which
// is reason enough.
//
// `compose/enrichextract.go` guards the same invariant for extracted fields and
// answers it differently — it drops the whole field and treats zero as invalid.
// Both are right for their own input: an extracted value is model-fabricated
// and worth nothing without a confidence, where a phone number was paid for and
// stands on its own. Named here so the difference reads as a choice.
//
// An unstatable score drops the CONFIDENCE, never the value beside it. The
// phone number is what the row was bought for and is unaffected by how a vendor
// scales its certainty; the confidence is optional in the contract precisely so
// it can be absent. Saying nothing about how sure we are beats saying something
// the contract cannot express — and clamping would assert maximal certainty
// while normalising would guess a scale the vendor never declared.
//
// Guarded at READ rather than at write, so `value_json` keeps what the vendor
// actually asserted: a scale corrected later restores every confidence, and the
// Art. 15 export still shows what was received.
//
// NaN fails `>= 0`; the infinities fail one bound each — `+Inf >= 0` is TRUE
// and only the upper bound stops it. Neither needs a test of its own, but the
// reason they are excluded is not the same reason.
func statableConfidence(score float64) bool {
	return score >= 0 && score <= 1
}

// foldOne decodes one claim onto the profile. An unreadable value is an error
// rather than a silent omission: the row was paid for, and a page that quietly
// dropped it would tell the reader the provider returned nothing.
func foldOne(c storedClaim, out *crmcontracts.PersonProviderProfile) error {
	switch provider.ClaimKey(c.key) {
	case provider.ClaimProfessionalEmails, provider.ClaimPersonalEmails:
		return foldEmails(c, out)
	case provider.ClaimMobilePhones:
		return foldPhones(c, out)
	case provider.ClaimLinkedInProfile:
		return decodeInto(c, &out.LinkedinUrl)
	case provider.ClaimCurrentEmployment:
		return foldEmployment(c, out)
	case provider.ClaimJobHistory:
		return foldJobHistory(c, out)
	case provider.ClaimLocation:
		return foldLocation(c, out)
	case provider.ClaimDepartments:
		return foldStrings(c, &out.Departments)
	case provider.ClaimSeniorities:
		return foldStrings(c, &out.Seniorities)
	default:
		// Unreachable today: the nine ClaimKey constants and the nine keys
		// the CHECK constraint admits are the same nine, and each has a case
		// above. It guards the next key added to that vocabulary — skipped
		// rather than errored, because a page that refused to load on an
		// unfamiliar category would be worse than one showing the rest, and
		// the claim is stored and exported either way.
		return nil
	}
}

// providerEmail is the wire shape the adapter stores for an address.
type providerEmail struct {
	Value            string  `json:"value"`
	EmailType        *string `json:"email_type"`
	ValidationStatus *string `json:"validation_status"`
}

// foldEmails appends the addresses. email_type may be ABSENT even under the
// professional cascade — the provider omits it — so the platform labels the
// address from the request policy and never claims the provider said so.
func foldEmails(c storedClaim, out *crmcontracts.PersonProviderProfile) error {
	var addresses []providerEmail
	if err := json.Unmarshal(c.value, &addresses); err != nil {
		return fmt.Errorf("person360: reading the %s claim: %w", c.key, err)
	}
	requested := c.key == string(provider.ClaimProfessionalEmails)
	for _, a := range addresses {
		email := crmcontracts.PersonProviderEmail{
			Value:            openapi_types.Email(a.Value),
			ValidationStatus: a.ValidationStatus,
		}
		switch {
		case a.EmailType != nil:
			email.EmailType = providerPtr(crmcontracts.PersonProviderEmailEmailType(*a.EmailType))
			email.EmailTypeSource = providerPtr(crmcontracts.PersonProviderEmailEmailTypeSourceProvider)
		case requested:
			// Labeled from what we ASKED for, and marked as such: the
			// professional cascade returned it, so it is professional by
			// request rather than by the provider's word.
			email.EmailType = providerPtr(crmcontracts.PersonProviderEmailEmailTypeProfessional)
			email.EmailTypeSource = providerPtr(crmcontracts.PersonProviderEmailEmailTypeSourceRequestedCascade)
		}
		out.Emails = append(out.Emails, email)
	}
	return nil
}

// providerPhone is the wire shape stored for a mobile number.
type providerPhone struct {
	Value      string   `json:"value"`
	Confidence *float64 `json:"confidence"`
}

func foldPhones(c storedClaim, out *crmcontracts.PersonProviderProfile) error {
	var phones []providerPhone
	if err := json.Unmarshal(c.value, &phones); err != nil {
		return fmt.Errorf("person360: reading the %s claim: %w", c.key, err)
	}
	for _, p := range phones {
		confidence := p.Confidence
		if confidence == nil {
			confidence = c.confidence
		}
		phone := crmcontracts.PersonProviderPhone{Value: p.Value}
		if confidence != nil && statableConfidence(*confidence) {
			// The contract carries a float32 because a confidence is a band,
			// not a measurement; the extra precision would imply one.
			phone.Confidence = providerPtr(float32(*confidence))
		}
		out.MobilePhones = append(out.MobilePhones, phone)
	}
	return nil
}

// providerEmployment is the wire shape stored for the current job.
type providerEmployment struct {
	CompanyName   string `json:"company_name"`
	CompanyDomain string `json:"company_domain"`
	JobTitle      string `json:"job_title"`
}

func foldEmployment(c storedClaim, out *crmcontracts.PersonProviderProfile) error {
	var e providerEmployment
	if err := json.Unmarshal(c.value, &e); err != nil {
		return fmt.Errorf("person360: reading the %s claim: %w", c.key, err)
	}
	out.CurrentEmployment = &crmcontracts.PersonProviderEmployment{
		CompanyName:   emptyToNil(e.CompanyName),
		CompanyDomain: emptyToNil(e.CompanyDomain),
		JobTitle:      emptyToNil(e.JobTitle),
	}
	return nil
}

// providerJob is one past role.
type providerJob struct {
	CompanyName string `json:"company_name"`
	JobTitle    string `json:"job_title"`
	LinkedInURL string `json:"linkedin_url"`
	StartedAt   string `json:"started_at"`
	EndedAt     string `json:"ended_at"`
}

// foldJobHistory appends past roles. The provider returns EMPTY STRINGS for
// fields it does not have, and an empty string is not a value: it renders as
// a blank line on the page and would export as one in an Art. 15 package.
// Normalizing them to absent is the platform's job.
func foldJobHistory(c storedClaim, out *crmcontracts.PersonProviderProfile) error {
	var jobs []providerJob
	if err := json.Unmarshal(c.value, &jobs); err != nil {
		return fmt.Errorf("person360: reading the %s claim: %w", c.key, err)
	}
	for _, j := range jobs {
		out.JobHistory = append(out.JobHistory, crmcontracts.PersonProviderJobHistory{
			CompanyName: j.CompanyName,
			JobTitle:    emptyToNil(j.JobTitle),
			LinkedinUrl: emptyToNil(j.LinkedInURL),
			StartedAt:   monthStart(j.StartedAt),
			EndedAt:     monthStart(j.EndedAt),
		})
	}
	return nil
}

// monthStart reads the provider's "YYYY-MM" job dates onto the contract's
// date-time fields. Anything it cannot parse is ABSENT rather than guessed: a
// zero time renders as January of year 1, which is worse than an undated role
// because it looks like data.
func monthStart(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01", value)
	if err != nil {
		return nil
	}
	return &parsed
}

// foldLocation carries the provider's location verbatim.
//
// It is NOT split onto the typed city/region/country fields. That needs a
// parse this cannot do honestly — "Munich, Germany" and "Springfield, IL, US"
// are not the same shape — and inventing a city from a string we cannot read
// would assert something the provider never said. The typed parts stay absent
// until a provider returns them as parts.
func foldLocation(c storedClaim, out *crmcontracts.PersonProviderProfile) error {
	var location string
	if err := json.Unmarshal(c.value, &location); err != nil {
		return fmt.Errorf("person360: reading the %s claim: %w", c.key, err)
	}
	out.Location = emptyToNil(location)
	return nil
}

func foldStrings(c storedClaim, into *[]string) error {
	var values []string
	if err := json.Unmarshal(c.value, &values); err != nil {
		return fmt.Errorf("person360: reading the %s claim: %w", c.key, err)
	}
	*into = append(*into, values...)
	return nil
}

// decodeInto reads a bare JSON string claim onto a nullable field.
func decodeInto(c storedClaim, into **string) error {
	var value string
	if err := json.Unmarshal(c.value, &value); err != nil {
		return fmt.Errorf("person360: reading the %s claim: %w", c.key, err)
	}
	*into = emptyToNil(value)
	return nil
}

// emptyToNil turns the provider's empty string into an absent field.
func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// providerPtr is this file's own pointer helper. Named rather than reusing
// the package's ptr(): that one is sectionstimeline.go's, and two files
// sharing an unexported one-liner is how a later split silently breaks one of
// them.
func providerPtr[T any](v T) *T { return &v }
