// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package messagingpolicy compiles a pack's Markdown policy files into the
// messaging.Rules it declares. gen-messaging-rules writes the result into the
// pack as Go, and the consent tests run the same sources through the engine.
//
// Only the front matter compiles. The Markdown body is the explanation a
// reviewer reads, and it reaches the build through the digest alone.
package messagingpolicy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"
)

// fence opens and closes the front matter of every policy and fixture file.
const fence = "---"

// splitFrontMatter answers the front matter and the body of one file.
func splitFrontMatter(path string, raw []byte) (front, body []byte, err error) {
	rest, opened := bytes.CutPrefix(raw, []byte(fence+"\n"))
	if !opened {
		return nil, nil, fmt.Errorf("%s: a policy file opens with a %q line and its front matter", path, fence)
	}
	front, body, closed := bytes.Cut(rest, []byte("\n"+fence+"\n"))
	if !closed {
		return nil, nil, fmt.Errorf("%s: the front matter has no closing %q line", path, fence)
	}
	return append(front, '\n'), body, nil
}

// decodeStrict decodes front matter into out and refuses a key out does not
// name, so a misspelt obligation fails instead of being dropped.
//
//craft:ignore naked-any yaml.Decoder.Decode takes any; each caller passes a pointer to its own key set
func decodeStrict(path string, front []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(front))
	decoder.KnownFields(true)
	// An empty front matter is no keys, and decodes to the zero value.
	if err := decoder.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// indexFile is policy/index.md: the layers, most general first.
type indexFile struct {
	Layers []string `yaml:"layers"`
}

// layerFile is the closed key set of one policy layer. Each obligation key
// spells one messaging.Rules field in lower-case words joined by hyphens, and
// an absent key leaves the layer below in force.
//
//nolint:tagliatelle // policy files spell their keys in kebab case
type layerFile struct {
	Jurisdiction  *string           `yaml:"jurisdiction"`
	Version       *int              `yaml:"version"`
	Created       *time.Time        `yaml:"created"`
	SourceChecked *time.Time        `yaml:"source-checked"`
	Reviewed      *time.Time        `yaml:"reviewed"`
	ReviewDue     *time.Time        `yaml:"review-due"`
	Reviewer      *string           `yaml:"reviewer"`
	Evidence      []string          `yaml:"evidence"`
	Exceptions    []string          `yaml:"exceptions"`
	Fixtures      []fixturePair     `yaml:"fixtures"`
	Instruments   []instrumentEntry `yaml:"instruments"`
	Citations     []citationEntry   `yaml:"citations"`

	ReplyWindow                       *time.Duration     `yaml:"reply-window"`
	DealFollowUpWindow                *time.Duration     `yaml:"deal-follow-up-window"`
	MarketingExceptions               *[]exceptionEntry  `yaml:"marketing-exceptions"`
	ConsentOnly                       *bool              `yaml:"consent-only"`
	Disclosures                       *[]disclosureEntry `yaml:"disclosures"`
	SubjectPrefix                     *string            `yaml:"subject-prefix"`
	FrequencyCap                      *frequencyCapEntry `yaml:"frequency-cap"`
	RefusalSectionLast                *bool              `yaml:"refusal-section-last"`
	ClaimsNeedProof                   *[]string          `yaml:"claims-need-proof"`
	ForeignAdvertiserNeedsLocalAgency *bool              `yaml:"foreign-advertiser-needs-local-agency"`
	OptOutAcknowledgement             *bool              `yaml:"opt-out-acknowledgement"`
	OptOutAcknowledgementWithin       *time.Duration     `yaml:"opt-out-acknowledgement-within"`
	OptOutAcknowledgementUTCOffset    *time.Duration     `yaml:"opt-out-acknowledgement-utc-offset"`
	OptOutRecordsKeep                 *time.Duration     `yaml:"opt-out-records-keep"`
	DataProtectionFollowsRecipient    *bool              `yaml:"data-protection-follows-recipient"`
	NoLegitimateInterests             *bool              `yaml:"no-legitimate-interests"`
	DisclosureLanguage                *string            `yaml:"disclosure-language"`
	RecordConfirmationWithoutNews     *bool              `yaml:"record-confirmation-without-news"`
	ConsentConfirmationNeedsRequest   *bool              `yaml:"consent-confirmation-needs-request"`
	ConsentRequestFirstFor            *[]string          `yaml:"consent-request-first-for"`
}

// fixturePair names a draft that must raise finding and a near-identical one
// that must not. Paths are relative to the policy folder.
type fixturePair struct {
	Finding  string `yaml:"finding"`
	Triggers string `yaml:"triggers"`
	Clears   string `yaml:"clears"`
}

// instrumentEntry is one law. A refresh writes text-digest and text-fetched;
// text-url and checked-by-hand are written by whoever maintains the pack.
//
//nolint:tagliatelle // policy files spell their keys in kebab case
type instrumentEntry struct {
	Name          string    `yaml:"name"`
	Effective     time.Time `yaml:"effective"`
	Expires       time.Time `yaml:"expires"`
	Provision     string    `yaml:"provision"`
	SourceURL     string    `yaml:"source-url"`
	TextURL       string    `yaml:"text-url"`
	TextDigest    string    `yaml:"text-digest"`
	TextFetched   time.Time `yaml:"text-fetched"`
	CheckedByHand time.Time `yaml:"checked-by-hand"`
}

// readFrom is the address a refresh fetches: the readable form when the layer
// states one, else the official page.
func (e instrumentEntry) readFrom() string {
	if e.TextURL != "" {
		return e.TextURL
	}
	return e.SourceURL
}

// citationEntry names the law behind one engine code. territorial-test is
// required and must be one of the Territory values. The engine cannot decide a
// test written as prose, so such a test never selects a citation.
//
//nolint:tagliatelle // policy files spell their keys in kebab case
type citationEntry struct {
	Code            string  `yaml:"code"`
	TerritorialTest *string `yaml:"territorial-test"`
	Instrument      string  `yaml:"instrument"`
	Provision       string  `yaml:"provision"`
	SourceURL       string  `yaml:"source-url"`
}

//nolint:tagliatelle // policy files spell their keys in kebab case
type exceptionEntry struct {
	Kind                         string `yaml:"kind"`
	RequiresSaleEvidence         bool   `yaml:"requires-sale-evidence"`
	RequiresCollectionTimeOptOut bool   `yaml:"requires-collection-time-opt-out"`
	RequiresSimilarity           bool   `yaml:"requires-similarity"`
	RequiresNoObjection          bool   `yaml:"requires-no-objection"`
}

//nolint:tagliatelle // policy files spell their keys in kebab case
type disclosureEntry struct {
	Kind          string   `yaml:"kind"`
	MarketingOnly bool     `yaml:"marketing-only"`
	Means         []string `yaml:"means"`
}

type frequencyCapEntry struct {
	Messages int           `yaml:"messages"`
	Window   time.Duration `yaml:"window"`
}

// fixtureFile is the closed key set of one fixture: the send the draft below
// it is part of. The body is the draft's text.
//
//nolint:tagliatelle // policy files spell their keys in kebab case
type fixtureFile struct {
	Subject      string        `yaml:"subject"`
	Advertising  bool          `yaml:"advertising"`
	Sender       string        `yaml:"sender"`
	Language     string        `yaml:"language"`
	MailLanguage string        `yaml:"mail-language"`
	Particulars  Particulars   `yaml:"particulars"`
	Translations []Particulars `yaml:"translations"`
}

// Particulars are the controller particulars a fixture's installation states.
// Language is set only on a translation.
//
//nolint:tagliatelle // policy files spell their keys in kebab case
type Particulars struct {
	Language          string `yaml:"language"`
	LegalName         string `yaml:"legal-name"`
	PostalAddress     string `yaml:"postal-address"`
	PrivacyContact    string `yaml:"privacy-contact"`
	ObjectionRoute    string `yaml:"objection-route"`
	AdvertiserContact string `yaml:"advertiser-contact"`
}
