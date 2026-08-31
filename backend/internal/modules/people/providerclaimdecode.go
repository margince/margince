// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Reading a run's claims into the handful of things a record can hold.
//
// The decode structs are hand-written against shapes another package produces,
// so a claim this build cannot read FAILS rather than being skipped: a purchase
// silently dropped here would look exactly like a provider that had nothing,
// and the difference is money.
//
// A provider returns empty strings for what it does not have, so every value
// is trimmed and an empty one is treated as absent. Writing "" into a title
// would fill the field with nothing and lock out the fill that follows.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// applicableClaims is what one run answered, reduced to the fields a record
// holds. Everything else a provider sells — location, seniority, departments,
// the work history — has no column to land in and stays beside the record.
type applicableClaims struct {
	title         string
	linkedInURL   string
	companyName   string
	companyDomain string
	email         string
	phone         string
}

// providerEmployment is the current-role claim's stored shape.
type providerEmployment struct {
	CompanyName   string `json:"company_name"`
	CompanyDomain string `json:"company_domain"`
	JobTitle      string `json:"job_title"`
}

// providerEmailClaim is one bought address.
type providerEmailClaim struct {
	Value string `json:"value"`
}

// providerPhoneClaim is one bought number.
type providerPhoneClaim struct {
	Value string `json:"value"`
}

// decodeApplicableClaims reduces a run's answers to what can be filled.
//
// Only the FIRST address and number are taken. A provider may return several;
// putting them all on the record would make a purchase look like a decision
// somebody made about which address is right, and the record's own primary
// flag is that decision.
func decodeApplicableClaims(claims []provider.Claim) (applicableClaims, error) {
	var out applicableClaims
	for _, c := range claims {
		switch c.Key {
		case provider.ClaimLinkedInProfile:
			var url string
			if err := decodeClaim(c, &url); err != nil {
				return applicableClaims{}, err
			}
			out.linkedInURL = strings.TrimSpace(url)
		case provider.ClaimCurrentEmployment:
			var e providerEmployment
			if err := decodeClaim(c, &e); err != nil {
				return applicableClaims{}, err
			}
			out.title = strings.TrimSpace(e.JobTitle)
			out.companyName = strings.TrimSpace(e.CompanyName)
			out.companyDomain = strings.TrimSpace(e.CompanyDomain)
		case provider.ClaimProfessionalEmails:
			var addresses []providerEmailClaim
			if err := decodeClaim(c, &addresses); err != nil {
				return applicableClaims{}, err
			}
			if len(addresses) > 0 {
				out.email = strings.TrimSpace(addresses[0].Value)
			}
		case provider.ClaimMobilePhones:
			var phones []providerPhoneClaim
			if err := decodeClaim(c, &phones); err != nil {
				return applicableClaims{}, err
			}
			if len(phones) > 0 {
				out.phone = strings.TrimSpace(phones[0].Value)
			}
		}
	}
	return out, nil
}

// decodeClaim reads one claim's value, naming the key when it cannot.
//
//craft:ignore naked-any the decode target is a different shape per claim key — a URL, a struct, a slice — and json.Unmarshal's own parameter is what this forwards to
func decodeClaim(c provider.Claim, into any) error {
	if err := json.Unmarshal(c.Value, into); err != nil {
		return fmt.Errorf("people: reading the %s a purchase returned: %w", c.Key, err)
	}
	return nil
}
