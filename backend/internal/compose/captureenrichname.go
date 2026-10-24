// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The name half of the signature reply. The model reports the parts it can read
// off the page. contacts.MapSignatureName is the one place that turns them into
// first_name and last_name, so no code here decides by word position.

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// signatureNameKey is where the reply carries the name.
const signatureNameKey = "name"

// signatureFieldList is the vocabulary as the user prompt offers it, derived
// from the list the gate and the schema read.
var signatureFieldList = func() string {
	encoded, err := json.Marshal(signatureFieldNames)
	if err != nil {
		panic(fmt.Sprintf("compose: encoding the signature vocabulary: %v", err))
	}
	return string(encoded)
}()

// signatureNameColumns are the contact columns a name reading fills. An
// expectation in the certification corpus names them although the model is never
// offered them as fields.
var signatureNameColumns = nameSet([]string{contacts.FieldFirstName, contacts.FieldLastName})

type signatureNameReply struct {
	FamilyName      string            `json:"family_name"`
	GivenNames      string            `json:"given_names"`
	Middle          string            `json:"middle"`
	Order           string            `json:"order"`
	EvidenceSnippet string            `json:"evidence_snippet"`
	Confidence      schema.Confidence `json:"confidence"`
}

// signatureNameOrders is the closed set the reply may report.
var signatureNameOrders = []string{
	contacts.SignatureOrderGivenFirst, contacts.SignatureOrderFamilyFirst, contacts.SignatureOrderUnclear,
}

func signatureNameSchema() schema.Node {
	return schema.Object(
		map[string]schema.Node{
			"family_name":           schema.String().Describe("The family name as printed; empty when the order is unclear."),
			"given_names":           schema.String().Describe("The given names as printed; empty when the order is unclear."),
			"middle":                schema.String().Describe("Any words between them as printed; empty when there are none."),
			"order":                 schema.Enum(signatureNameOrders...).Describe("Which part the signature prints first; unclear when the page does not show it."),
			extractionEvidenceKey:   schema.String().Describe("The printed line that holds the name, copied verbatim."),
			extractionConfidenceKey: schema.Number(),
		},
		"family_name", "given_names", "middle", "order", "evidence_snippet", "confidence",
	)
}

// signatureNameShapeValid is the retry validator's check of the name object: an
// order outside the closed set is a fault a retry can fix.
func signatureNameShapeValid(text string) error {
	var parsed struct {
		Name *signatureNameReply `json:"name"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &parsed); err != nil {
		return fmt.Errorf("output is not the required JSON shape: %w", err)
	}
	if parsed.Name == nil {
		return nil
	}
	if slices.Contains(signatureNameOrders, parsed.Name.Order) {
		return nil
	}
	return fmt.Errorf("name order %q is not one of %s", clampToken(parsed.Name.Order), strings.Join(signatureNameOrders, ", "))
}

// gateSignatureName applies to the name object what gateEvidence applies to a
// field. The snippet must be on the page, and the confidence must be in range
// and at or above the floor. A name that clears the gate is mapped to the
// first_name and last_name entries to apply, or to nothing.
//
// An unclear order is an answer, not a refusal, so it is returned without a
// dropped finding.
func gateSignatureName(modelText, lines string) ([]contacts.SignatureField, []droppedFinding) {
	var parsed struct {
		Name *signatureNameReply `json:"name"`
	}
	if err := json.Unmarshal([]byte(ai.Unfence(modelText)), &parsed); err != nil || parsed.Name == nil {
		return nil, nil
	}
	n := parsed.Name
	if n.Order == contacts.SignatureOrderUnclear {
		return nil, nil
	}
	drop := func(reason string) []droppedFinding {
		return []droppedFinding{{
			Lane: laneFields, Field: signatureNameKey, Value: n.GivenNames + " " + n.FamilyName,
			EvidenceSnippet: n.EvidenceSnippet, Reason: reason,
		}}
	}
	switch {
	case strings.TrimSpace(n.EvidenceSnippet) == "":
		return nil, drop(dropEmptyEvidence)
	case !evidenceOnPage(lines, normalizeEvidence(lines), n.EvidenceSnippet):
		return nil, drop(dropEvidenceNotOnPage)
	case n.Confidence <= 0 || n.Confidence > 1:
		return nil, drop(dropConfidenceRange)
	case float64(n.Confidence) < enrichConfidenceFloor:
		return nil, drop(dropBelowFloor)
	}
	mapped, ok := contacts.MapSignatureName(contacts.SignatureNameParts{
		Family: n.FamilyName, Given: n.GivenNames, Middle: n.Middle, Order: n.Order, Evidence: n.EvidenceSnippet,
	})
	if !ok {
		return nil, drop(dropNameUnmappable)
	}
	field := func(name, value string) contacts.SignatureField {
		return contacts.SignatureField{
			Name: name, Value: value, Evidence: n.EvidenceSnippet, Confidence: float64(n.Confidence),
			ClaimKey: ai.ClaimKey(ai.ProfileFieldClaimPath(name)), Display: mapped.Display,
		}
	}
	return []contacts.SignatureField{
		field(contacts.FieldFirstName, mapped.First), field(contacts.FieldLastName, mapped.Last),
	}, nil
}
