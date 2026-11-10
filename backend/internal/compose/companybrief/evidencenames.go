// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import (
	"strings"

	"github.com/margince/margince/backend/internal/compose/claims"
)

// knownRecords is what this brief was written from, keyed by type and id.
//
// Keying on the id alone accepted a real deal id cited as a contact. The id
// passes, and the card then routes the reader to the wrong screen, or to a
// record of a kind they were never shown. The pair is the reference, so the
// pair is what is checked. The value is what that record said, as the model
// was shown it, so a sentence can be held to the rows it points at.
func knownRecords(companyID string, in Input) map[Evidence]string {
	known := map[Evidence]string{{EntityType: citeCompany, EntityID: companyID}: accountItself(in)}
	for _, deal := range in.OpenDeals {
		known[Evidence{EntityType: citeDeal, EntityID: deal.ID}] = claims.Source(deal)
	}
	for _, act := range in.Recent {
		known[Evidence{EntityType: citeActivity, EntityID: act.ID}] = claims.Source(act)
	}
	for _, contact := range in.Contacts {
		known[Evidence{EntityType: citeContact, EntityID: contact.ID}] = claims.Source(contact)
	}
	for _, task := range in.OpenTasks {
		known[Evidence{EntityType: citeActivity, EntityID: task.ID}] = claims.Source(task)
	}
	for _, commitment := range in.OpenCommitments {
		// A commitment cites the message it was read from, which Recent may
		// carry too, and one message can hold two promises. The texts are
		// joined under the one key so a sentence is held to all of them.
		key := Evidence{EntityType: citeActivity, EntityID: commitment.ActivityID}
		known[key] = strings.TrimSpace(known[key] + "\n" + claims.Source(commitment))
	}
	return known
}

// accountItself is the account's own facts: the input with its lists emptied.
//
// The whole input would make a citation of the company a skeleton key. Most
// sentences cite it, and every figure anywhere in the payload would answer
// for them. A sentence about the account may state what the account row says:
// its industry, its size, its lifetime won.
func accountItself(in Input) string {
	in.Contacts, in.OpenDeals, in.Recent, in.OpenTasks, in.OpenCommitments = nil, nil, nil, nil, nil
	return claims.Source(in)
}

// recordKey is knownRecords' pair without the citation's descriptive Name.
// A name lookup and a grounding lookup can then share one shape without either
// caring what the other keys on.
type recordKey struct {
	EntityType string
	EntityID   string
}

// recordNames is every citable record's own display name, read from the same
// input the brief was written from. It never comes from the model's reply,
// which cannot be trusted to relay a record's name correctly. It never comes
// from the evidence a sentence already carries, which for the model lane has
// none.
func recordNames(in Input) map[recordKey]string {
	names := make(map[recordKey]string, len(in.OpenDeals)+len(in.Recent)+len(in.Contacts)+len(in.OpenTasks)+len(in.OpenCommitments))
	for _, deal := range in.OpenDeals {
		names[recordKey{citeDeal, deal.ID}] = deal.Name
	}
	for _, act := range in.Recent {
		names[recordKey{citeActivity, act.ID}] = act.Subject
	}
	for _, contact := range in.Contacts {
		names[recordKey{citeContact, contact.ID}] = contact.Name
	}
	for _, task := range in.OpenTasks {
		names[recordKey{citeActivity, task.ID}] = task.Name
	}
	// A commitment's message is named by its promise. A citation cannot pick
	// between two, and a cut page may hide a second. Then the subject stands.
	promises := make(map[recordKey]int, len(in.OpenCommitments))
	for _, commitment := range in.OpenCommitments {
		promises[recordKey{citeActivity, commitment.ActivityID}]++
	}
	for _, commitment := range in.OpenCommitments {
		key := recordKey{citeActivity, commitment.ActivityID}
		if promises[key] == 1 && !in.OpenCommitmentsCut {
			names[key] = commitment.Claim
		}
	}
	return names
}

// withEvidenceNames attaches each citation's own display name to sentences
// already accepted by the grounding filter. It is the one place that runs for
// both writers, so a "deal" chip names the deal whichever lane wrote the
// sentence about it. Run it after grounding, never before. A name is cosmetic.
// Attached earlier, it would risk a caller comparing full Evidence values
// before they learn to strip it. Grounded and Dedupe guard against that in
// package claims.
func withEvidenceNames(sentences []Sentence, in Input) []Sentence {
	names := recordNames(in)
	for i := range sentences {
		evidence := sentences[i].Evidence
		for j := range evidence {
			key := recordKey{evidence[j].EntityType, evidence[j].EntityID}
			if name, ok := names[key]; ok && name != "" && evidence[j].Name == "" {
				evidence[j].Name = name
			}
		}
	}
	return sentences
}

// withSectionEvidenceNames is withEvidenceNames over a brief's sections
// rather than a flat sentence list: the shape Write returns and Get caches.
// Get caches.
func withSectionEvidenceNames(sections []Section, in Input) []Section {
	for i := range sections {
		sections[i].Sentences = withEvidenceNames(sections[i].Sentences, in)
	}
	return sections
}
