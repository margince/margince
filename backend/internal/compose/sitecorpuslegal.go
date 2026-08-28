// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The deep read's legal-identity gate: the entity census becomes the
// multi-entity abstention verdict, and legal-trio fields keep their
// authority rules — only a shallow legal page testifies, and a disputed
// entity means no legal identity is proposed at all.

import (
	"net/url"
	"strings"
	"unicode"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// corpusLegalEntity is one entity a legal page names, with the identity
// details printed alongside it. It is both the census the multi-entity
// abstention counts AND, when that abstention fires, the choice offered to
// the human: a group's imprint lists its subsidiaries in blocks — name,
// registered address, registration or VAT number — and dropping all of it
// because there were five leaves a person retyping what the page already
// stated, which is the one thing this read exists to prevent.
type corpusLegalEntity struct {
	Name string `json:"name"`
	// RegisteredAddress, RegisterNumber and VatNumber are empty when the page
	// states the entity but not that detail — never guessed to fill the block.
	//
	// The two numbers are separate because the authorities behind them are: a
	// court issues the register entry and a tax office issues the VAT ID, and
	// a company states both. Reading them into one field meant whichever the
	// page printed first stood for the other.
	RegisteredAddress string `json:"registered_address,omitempty"`
	RegisterNumber    string `json:"register_number,omitempty"`
	VatNumber         string `json:"vat_number,omitempty"`
	EvidenceSnippet   string `json:"evidence_snippet,omitempty"`
	SourceURL         string `json:"source_url"`
}

// dedupeLegalEntities folds the census into the list a human is offered.
// One entity reaches it several times: every locale of the legal page
// states it, and a group's page labels each block with a market name
// ("Gradion Singapur") above the entity that trades there. A register
// number is the identity a registry issues, so blocks sharing one are the
// same company however they are labelled; entities without one fall back
// to their normalized name. The richest sighting wins the block it is shown
// under, so a locale that printed the address is not lost to one that
// omitted it.
//
// Details the winner lacks are taken from the sighting it replaced rather
// than discarded. They are three different facts about one company and the
// page order decides nothing: an imprint printing the address and the VAT ID
// and a second locale printing the address and the register entry tie on
// count, so a winner-takes-all rule kept whichever came first and dropped
// the other's number entirely.
func dedupeLegalEntities(entities []corpusLegalEntity) []corpusLegalEntity {
	var out []corpusLegalEntity
	for _, entity := range entities {
		at := matchingLegalEntity(out, entity)
		if at < 0 {
			out = append(out, entity)
			continue
		}
		if legalEntityDetail(entity) > legalEntityDetail(out[at]) {
			entity = fillLegalDetailsFrom(entity, out[at])
			out[at] = entity
			continue
		}
		out[at] = fillLegalDetailsFrom(out[at], entity)
	}
	return removeBrandOnlyLegalAliases(out)
}

// fillLegalDetailsFrom completes one sighting of an entity from another of
// the same entity on the SAME page. Only a detail the kept sighting lacks is
// taken.
//
// The page restriction is what keeps the evidence honest. An entity carries
// ONE snippet and one source URL, and every field filled from it is published
// citing them (fillLegalTrioFromCensus). A number borrowed from another page
// would arrive quoting a passage that never printed it — the exact claim the
// no-guess gate exists to refuse. Two sightings on one page are two blocks of
// the same notice, which that page's snippet does cover.
func fillLegalDetailsFrom(kept, other corpusLegalEntity) corpusLegalEntity {
	if kept.SourceURL != other.SourceURL {
		return kept
	}
	if kept.RegisteredAddress == "" {
		kept.RegisteredAddress = other.RegisteredAddress
	}
	if kept.RegisterNumber == "" {
		kept.RegisterNumber = other.RegisterNumber
	}
	if kept.VatNumber == "" {
		kept.VatNumber = other.VatNumber
	}
	return kept
}

// matchingLegalEntity joins locale variants without collapsing genuinely
// distinct registrations. A translated legal page can omit the register
// number that another locale printed; those sightings still match by name.
// When both sightings carry different register numbers, the registry
// identities win and the entities remain separate even if their names match.
//
// A VAT ID is the same kind of evidence one authority weaker: a tax office
// issues it to one company, so two sightings sharing one are the same
// company and two carrying different ones are not. It decides only where no
// register number does, because a group's entities can share neither.
// Without it a page printing only VAT IDs had no identity at all: sightings
// of one entity split under their locale names and could trip the
// multi-entity abstention, and two different companies printed under one
// name merged into whichever was seen first.
func matchingLegalEntity(existing []corpusLegalEntity, candidate corpusLegalEntity) int {
	candidateName := legalEntityNameKey(candidate.Name)
	candidateRegister := normalizeEvidence(candidate.RegisterNumber)
	candidateVat := normalizeEvidence(candidate.VatNumber)
	for i, entity := range existing {
		name := legalEntityNameKey(entity.Name)
		register := normalizeEvidence(entity.RegisterNumber)
		vat := normalizeEvidence(entity.VatNumber)
		sameRegister := candidateRegister != "" && register != "" && candidateRegister == register
		sameVat := candidateVat != "" && vat != "" && candidateVat == vat
		compatibleName := candidateName != "" && candidateName == name &&
			bothOrEitherEmpty(candidateRegister, register) &&
			bothOrEitherEmpty(candidateVat, vat)
		if sameRegister || sameVat || compatibleName {
			return i
		}
	}
	return -1
}

// bothOrEitherEmpty reports whether two identifiers can belong to one
// company: they agree, or at least one sighting did not print its own.
func bothOrEitherEmpty(left, right string) bool {
	return left == "" || right == "" || left == right
}

// legalEntityNameKey treats punctuation-only legal-form variants as the
// same printed company: "B.V." and "BV", or a comma before "Inc.", do
// not create separate entities. Letters and digits remain authoritative;
// distinct registry numbers still prevent a fold in matchingLegalEntity.
func legalEntityNameKey(name string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

// removeBrandOnlyLegalAliases drops a bare brand label only when the same
// census also contains a richer registered name that includes that label
// and the bare row has no legal detail. This preserves sole traders and
// unusual legal names when they are the only identity the page states.
func removeBrandOnlyLegalAliases(entities []corpusLegalEntity) []corpusLegalEntity {
	kept := make([]corpusLegalEntity, 0, len(entities))
	for i, candidate := range entities {
		key := legalEntityNameKey(candidate.Name)
		dropAlias := false
		if legalEntityDetail(candidate) == 0 && len(strings.Fields(key)) == 1 {
			for j, richer := range entities {
				if i == j || legalEntityNameKey(richer.Name) == key {
					continue
				}
				for _, token := range strings.Fields(legalEntityNameKey(richer.Name)) {
					if token == key {
						dropAlias = true
						break
					}
				}
				if dropAlias {
					break
				}
			}
		}
		if !dropAlias {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// enrichLegalEntitiesFromProfile shares already-gated legal trio values
// with the single legal choice shown to the human. Both lanes cite shallow
// legal pages and the census has established that there is only one entity;
// this avoids asking the user to retype a number one lane recovered when the
// entity lane's 300-rune block ended between the address and the identifiers.
// Each field fills its own: a register entry recovered elsewhere cannot stand
// in for a VAT ID the page never printed.
func enrichLegalEntitiesFromProfile(entities []corpusLegalEntity, fields []evidencedField) []corpusLegalEntity {
	if len(entities) != 1 {
		return entities
	}
	out := append([]corpusLegalEntity(nil), entities...)
	for _, field := range fields {
		switch field.Field {
		case string(crmcontracts.ColdStartFieldFieldRegisteredAddress):
			if out[0].RegisteredAddress == "" {
				out[0].RegisteredAddress = field.Value
			}
		case string(crmcontracts.ColdStartFieldFieldRegisterNumber):
			if out[0].RegisterNumber == "" {
				out[0].RegisterNumber = field.Value
			}
		case string(crmcontracts.ColdStartFieldFieldRegisterVat):
			if out[0].VatNumber == "" {
				out[0].VatNumber = field.Value
			}
		}
	}
	return out
}

// fillLegalTrioFromCensus is the other direction, and the one that was
// missing: what the CENSUS proved reaches the profile fields.
//
// The two lanes read the same legal page and disagree about what survived.
// The census reads the whole page and gates each detail with groundedDetail;
// the profile lane reads a bounded excerpt and gates against the one passage
// the model cited. So an address the census confirmed was routinely dropped by
// the profile lane for citing a neighbouring passage — communicode.de had
// "Wittekindstr. 1a, 45131 Essen" on its entity and no registered_address
// field at all, which is the state 136 of the demo dataset's 190 companies
// were in.
//
// This adds nothing unevidenced. Every value copied here already passed the
// census's own no-guess gate against the page that printed it, and it carries
// that page's URL and evidence with it, so applyLegalGate judges it exactly as
// it judges a profile-lane field. It fills only what is ABSENT: a field the
// profile lane produced and the gate kept is the more specific answer and
// stands.
//
// Called AFTER applyLegalGate on purpose. The gate abstains wholesale when the
// census cannot say which entity the company is, and filling from a census
// that was just judged untrustworthy would reintroduce exactly what the
// abstention withheld.
func fillLegalTrioFromCensus(fields []evidencedField, entities []corpusLegalEntity, pageKind map[string]crmcontracts.SiteReadPageKind, abstained bool) []evidencedField {
	// One entity, or the company's legal identity is not settled and the
	// abstention owns this decision rather than us.
	if abstained || len(entities) != 1 {
		return fields
	}
	entity := entities[0]
	// Only a legal page speaks for legal identity — the same authority test
	// applyLegalGate applies, so a census sighting from anywhere else cannot
	// enter through this door either.
	if pageKind[entity.SourceURL] != crmcontracts.SiteReadPageKindImpressum || !legalAuthorityPage(entity.SourceURL) {
		return fields
	}
	present := make(map[string]bool, len(fields))
	for _, f := range fields {
		present[f.Field] = true
	}
	// A FIXED order, not a map's. These fields end up in the proposal whose
	// JSON is hashed, so iterating a map would give identical evidence a
	// different hash on every run.
	out := fields
	for _, candidate := range []struct {
		field string
		value string
	}{
		{string(crmcontracts.ColdStartFieldFieldLegalName), entity.Name},
		{string(crmcontracts.ColdStartFieldFieldRegisteredAddress), entity.RegisteredAddress},
		{string(crmcontracts.ColdStartFieldFieldRegisterNumber), entity.RegisterNumber},
		{string(crmcontracts.ColdStartFieldFieldRegisterVat), entity.VatNumber},
	} {
		if present[candidate.field] || strings.TrimSpace(candidate.value) == "" {
			continue
		}
		out = append(out, evidencedField{
			Field:           candidate.field,
			Value:           candidate.value,
			EvidenceSnippet: entity.EvidenceSnippet,
			SourceURL:       entity.SourceURL,
			Confidence:      censusFieldConfidence,
		})
	}
	return out
}

// censusFieldConfidence is what a census-filled field carries.
//
// The census does not score: it checks the value against the page that
// printed it and keeps it or drops it. So this is not a model's number, and
// the honest question is what a consumer of the field should do with it.
//
// 1 is right for the same reason the profile lane's own legal trio arrives at
// 1: these are the hard-gated, verbatim fields, matched against the page
// rather than judged. A lower number would read as doubt about a value that
// was checked more strictly than any scored field, and would fall under the
// 0.55 cold-start floor that decides whether a human is shown the field at
// all — withholding evidence the page plainly printed.
const censusFieldConfidence = 1

// legalEntityDetail counts how much of an entity block was actually
// printed — the tie-break when the same entity is seen twice.
func legalEntityDetail(entity corpusLegalEntity) int {
	filled := 0
	for _, value := range []string{entity.RegisteredAddress, entity.RegisterNumber, entity.VatNumber} {
		if strings.TrimSpace(value) != "" {
			filled++
		}
	}
	return filled
}

// The abstention's two user-facing spellings, one per cause. They are
// different facts: the first describes the COMPANY (its domain publishes
// several registrations), the second describes THIS RUN (a legal page's
// extraction never came back, so the count cannot be trusted). Printing the
// first when the second fired invents a corporate structure nobody read.
// legalWarningMultipleEntities has one spelling for the worker log, the
// debug report, and the E2E floor that greps it.
const (
	legalWarningMultipleEntities = "disagreeing legal pages: the domain hosts more than one entity — the legal-field override was dropped"
	legalWarningCensusIncomplete = "incomplete legal read: a legal page could not be extracted, so the entity census is unsettled — the legal-field override was dropped"
)

// legalAbstention is WHY the gate withheld the legal trio, spelled as the
// reason each withheld finding carries — so the drop record and the warning
// a human reads can never name different causes.
type legalAbstention string

const (
	legalAbstentionNone             legalAbstention = ""
	legalAbstentionMultipleEntities legalAbstention = dropLegalConflict
	legalAbstentionCensusIncomplete legalAbstention = dropLegalCensusIncomplete
)

// legalAbstentionOf reads the census verdict. A domain that states several
// distinct entities is answered first: that holds whether or not another
// legal page also failed, while an incomplete census says only that this
// run cannot trust its count.
func legalAbstentionOf(entities []corpusLegalEntity, censusIncomplete bool) legalAbstention {
	distinct := map[string]bool{}
	for _, e := range entities {
		distinct[legalEntityNameKey(e.Name)] = true
	}
	switch {
	case len(distinct) > 1:
		return legalAbstentionMultipleEntities
	case censusIncomplete:
		return legalAbstentionCensusIncomplete
	default:
		return legalAbstentionNone
	}
}

// warning is what a human is told about the abstention: the cause that
// actually fired, never a stand-in for it. A settled census warns nothing.
func (a legalAbstention) warning() string {
	switch a {
	case legalAbstentionMultipleEntities:
		return legalWarningMultipleEntities
	case legalAbstentionCensusIncomplete:
		return legalWarningCensusIncomplete
	default:
		return ""
	}
}

// applyLegalGate turns the gated legal-entity census into the abstention
// verdict: any abstention cause → the whole legal trio is stripped (missing
// beats another company's); a settled census → each trio field survives only
// when quoted from a shallow legal page.
func applyLegalGate(fields []evidencedField, entities []corpusLegalEntity, pageKind map[string]crmcontracts.SiteReadPageKind, censusIncomplete bool) ([]evidencedField, bool, []droppedFinding) {
	if abstention := legalAbstentionOf(entities, censusIncomplete); abstention != legalAbstentionNone {
		kept, dropped := withholdLegalTrio(fields, string(abstention))
		return kept, true, dropped
	}
	var dropped []droppedFinding
	kept := make([]evidencedField, 0, len(fields))
	for _, f := range fields {
		if legalPageFields[f.Field] &&
			(pageKind[f.SourceURL] != crmcontracts.SiteReadPageKindImpressum || !legalAuthorityPage(f.SourceURL)) {
			dropped = append(dropped, droppedFinding{
				Lane: laneLegal, Field: f.Field, Value: f.Value,
				EvidenceSnippet: f.EvidenceSnippet, Reason: dropLegalNotFromLegalPage,
			})
			continue
		}
		kept = append(kept, f)
	}
	return kept, false, dropped
}

// withholdLegalTrio strips every legal-trio field and records the abstention
// cause on each one, so the drop log states which of the two fired.
func withholdLegalTrio(fields []evidencedField, reason string) ([]evidencedField, []droppedFinding) {
	var dropped []droppedFinding
	kept := make([]evidencedField, 0, len(fields))
	for _, f := range fields {
		if legalPageFields[f.Field] {
			dropped = append(dropped, droppedFinding{
				Lane: laneLegal, Field: f.Field, Value: f.Value,
				EvidenceSnippet: f.EvidenceSnippet, Reason: reason,
			})
			continue
		}
		kept = append(kept, f)
	}
	return kept, dropped
}

// legalAuthorityPage limits legal-identity authority to legal pages the
// site OPERATOR plausibly owns: at most two path segments deep
// (/impressum, /de/impressum). Anything deeper reads like content ABOUT
// a legal page — a customer's imprint, an archived copy — and must not
// speak for the company.
func legalAuthorityPage(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	depth := 0
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment != "" {
			depth++
		}
	}
	return depth <= 2
}
