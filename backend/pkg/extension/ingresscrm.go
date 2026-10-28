// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The company and deal payloads a Record may carry in place of an activity.
//
//margince:extension-surface

package extension

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// KindCompany is a company the unit's provider knows, matched by the id
// the provider gives it.
const KindCompany RecordKind = "company"

// KindDeal is a sales opportunity the unit's provider knows, matched by the id
// the provider gives it.
const KindDeal RecordKind = "deal"

// AdoptKeyDomain names the one natural key a hand-made company may be adopted
// by: its web domain. A company holds no registration number, so the domain is
// the identity column the core already matches on.
const AdoptKeyDomain = "domain"

// MaxRelinkActivities caps how many activities one company or deal record may
// move onto itself, so one call cannot be a bulk edit of the timeline.
const MaxRelinkActivities = 50

// landableKinds are the kinds a source may declare.
var landableKinds = []RecordKind{KindActivity, KindCompany, KindDeal, KindSignal}

// uuidShape is the spelling of an id a deal names its pipeline and stage by.
var uuidShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// CompanyFields is a company on its way into the CRM.
//
// A record that finds no company under its outside id either adopts a
// hand-made one (AdoptOn) or creates one. Adopting and updating only fill
// fields that are empty. The core cannot tell a value a user typed from one
// a unit wrote earlier, so it keeps every value that is there.
type CompanyFields struct {
	DisplayName string
	LegalName   string
	Industry    string
	Description string
	// Domain is the company's primary web domain.
	Domain string
	// AdoptOn, when set to AdoptKeyDomain, lets this record take over a
	// hand-made company that holds Domain and no outside id. Two or more such
	// companies make the record a conflict rather than a guess.
	AdoptOn string
	// RelinkActivities are keys of activities this unit already landed from
	// the same system. Each one found and visible to the acting member is moved
	// onto this company. A key that matches nothing the member may see is
	// skipped without a word, so the list cannot be used to probe for records.
	RelinkActivities []string
}

// DealFields is a deal on its way into the CRM.
//
// There is no natural key for a deal: it is matched by its outside id alone.
type DealFields struct {
	Name string
	// PipelineID and StageID are the core's own ids. A deal is born on them;
	// a later record naming a different StageID moves the deal there. A stage
	// the core calls lost needs LostReason.
	PipelineID string
	StageID    string
	// LostReason is why the deal was lost; only read on a move to a lost stage.
	LostReason string
	// AmountMinor is the deal value in the smallest unit of Currency.
	AmountMinor *int64
	Currency    string
	// CustomFields are the values of the installation's custom deal fields the
	// unit offers, keyed by the field's `cf_` column name. A key no active field
	// takes is refused. Like the amount, a value a user last edited is left as
	// it is and named in Result.Withheld.
	CustomFields map[string]any
	// RelinkActivities has the same meaning as on CompanyFields.
	RelinkActivities []string
}

// Kind is which kind of record this is, read from the payload it carries.
// A record with no company and no deal payload is an activity, so a record
// written before the other kinds existed is still valid.
func (r Record) Kind() RecordKind {
	switch {
	case r.Company != nil:
		return KindCompany
	case r.Deal != nil:
		return KindDeal
	case r.Signal != nil:
		return KindSignal
	}
	return KindActivity
}

// validatePayload holds the shape rules of a company or deal record. An
// activity record is held to the activity grammar instead.
func (r Record) validatePayload() error {
	payloads := 0
	for _, set := range []bool{r.Company != nil, r.Deal != nil, r.Signal != nil} {
		if set {
			payloads++
		}
	}
	if payloads > 1 {
		return errors.New("extension: a record carries a company, a deal or a signal, not several")
	}
	if r.Activity != (ActivityFields{}) {
		return fmt.Errorf("extension: a %s record carries no activity", r.Kind())
	}
	switch r.Kind() {
	case KindCompany:
		return r.Company.validate()
	case KindSignal:
		return r.Signal.validate()
	}
	return r.Deal.validate()
}

func (c CompanyFields) validate() error {
	switch {
	case strings.TrimSpace(c.DisplayName) == "":
		return errors.New("extension: the company has no display name")
	case utf8.RuneCountInString(c.DisplayName) > MaxDisplayNameRunes,
		utf8.RuneCountInString(c.LegalName) > MaxDisplayNameRunes,
		utf8.RuneCountInString(c.Industry) > MaxDisplayNameRunes:
		return fmt.Errorf("extension: a company name or industry is over the %d-rune cap", MaxDisplayNameRunes)
	case utf8.RuneCountInString(c.Description) > MaxSubjectRunes:
		return fmt.Errorf("extension: the company description is over the %d-rune cap", MaxSubjectRunes)
	case c.AdoptOn != "" && c.AdoptOn != AdoptKeyDomain:
		return fmt.Errorf("extension: %q is not a key a company can be adopted by (only %q)", c.AdoptOn, AdoptKeyDomain)
	case c.AdoptOn != "" && strings.TrimSpace(c.Domain) == "":
		return errors.New("extension: the company asks to be adopted by its domain and names none")
	}
	return validateRelinkKeys(c.RelinkActivities)
}

func (d DealFields) validate() error {
	switch {
	case strings.TrimSpace(d.Name) == "":
		return errors.New("extension: the deal has no name")
	case utf8.RuneCountInString(d.Name) > MaxDisplayNameRunes:
		return fmt.Errorf("extension: the deal name is over the %d-rune cap", MaxDisplayNameRunes)
	case !uuidShape.MatchString(d.PipelineID), !uuidShape.MatchString(d.StageID):
		return errors.New("extension: a deal names its pipeline and stage by their ids")
	}
	if err := validateCustomFieldKeys(d.CustomFields); err != nil {
		return err
	}
	return validateRelinkKeys(d.RelinkActivities)
}

// MaxLandedCustomFields caps the custom fields one landed record carries.
const MaxLandedCustomFields = 50

var customFieldKey = regexp.MustCompile(`^cf_[a-z0-9_]{1,60}$`)

func validateCustomFieldKeys(values map[string]any) error {
	if len(values) > MaxLandedCustomFields {
		return fmt.Errorf("extension: the record offers %d custom fields, over the cap of %d", len(values), MaxLandedCustomFields)
	}
	for key := range values {
		if !customFieldKey.MatchString(key) {
			return fmt.Errorf("extension: %q is not a custom field column; name it as cf_ and lower snake_case", key)
		}
	}
	return nil
}

func validateRelinkKeys(keys []string) error {
	if len(keys) > MaxRelinkActivities {
		return fmt.Errorf("extension: the record relinks %d activities, over the cap of %d", len(keys), MaxRelinkActivities)
	}
	for _, key := range keys {
		if strings.TrimSpace(key) == "" || len(key) > MaxKeyLength {
			return fmt.Errorf("extension: a relink key is blank or over the %d-byte cap", MaxKeyLength)
		}
	}
	return nil
}
