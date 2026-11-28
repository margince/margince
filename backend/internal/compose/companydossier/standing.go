// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

// The company record beyond its profile, as the floor states it.
//
// A company nobody has read a website for still has a record. It says what the
// company does, its size, its lifecycle and what this reader can count of it.
// The floor states those, so its overview is not "nothing recorded".

import (
	"context"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/compose/claims"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Standing is what the floor states from the company record besides its two
// assessed columns. The model is not shown it, so it stays out of Input's JSON
// and of every fingerprint that JSON feeds.
type Standing struct {
	Description string `json:"description,omitempty"`
	Lifecycle   string `json:"lifecycle,omitempty"`
	// The counts are nil when this reader may not count them.
	Contacts  *int `json:"contacts,omitempty"`
	OpenDeals *int `json:"open_deals,omitempty"`
}

// readStanding fills the standing from the company row as this reader may read
// it. A company the read leaves out keeps no standing, not an error: the
// sidecar reads before it already answered whether the reader may see it.
func readStanding(ctx context.Context, facts Facts, id ids.CompanyID) (Standing, error) {
	rows, _, err := facts.ListCompanies(ctx, contacts.ListCompaniesInput{
		IDs: []ids.UUID{id.UUID}, IncludeArchived: true, IncludeAnchor: true,
	})
	if err != nil || len(rows) == 0 {
		return Standing{}, err
	}
	row := rows[0]
	standing := Standing{Contacts: row.ContactCount, OpenDeals: row.OpenDealCount}
	if row.Description != nil {
		standing.Description = strings.TrimSpace(*row.Description)
	}
	if row.Status != nil && *row.Status != crmcontracts.CompanyStatusUnknown {
		standing.Lifecycle = string(*row.Status)
	}
	return standing, nil
}

// standingSentences states the standing, each sentence citing the company. A
// count of zero is not stated, so a company with nothing recorded still says so.
func standingSentences(in Input, lang textlang.Lang) map[string][]claims.Sentence {
	said := langcopy.For(string(lang))
	cite := []claims.Evidence{{EntityType: citeCompany, EntityID: in.CompanyID}}
	out := map[string][]claims.Sentence{}
	add := func(kind string, label langcopy.Phrase, value string) {
		value = claims.TerminateSentence(value)
		if value == "" || in.CompanyID == "" {
			return
		}
		out[kind] = append(out[kind], claims.Sentence{Text: said.Say(label) + ": " + value, Nature: natureFact, Evidence: cite})
	}
	add(sectionSummary, standingLabels.description, in.Standing.Description)
	if words, ok := lifecycleWords[crmcontracts.CompanyStatus(in.Standing.Lifecycle)]; ok {
		add(sectionSummary, standingLabels.lifecycle, said.Say(words))
	}
	add(sectionSummary, standingLabels.contacts, countWords(in.Standing.Contacts))
	add(sectionSummary, standingLabels.openDeals, countWords(in.Standing.OpenDeals))
	if in.SizeBand != "" {
		add(sectionFirmographics, standingLabels.employees, strings.Replace(in.SizeBand, "-", "–", 1))
	}
	return out
}

// countWords is a count worth stating, or "" for none or one withheld.
func countWords(count *int) string {
	if count == nil || *count <= 0 {
		return ""
	}
	return strconv.Itoa(*count)
}

// floorRecords is KnownRecords with the standing added to the company's source,
// so the floor's own standing sentences are held to the record they cite.
func floorRecords(in Input) map[claims.Evidence]string {
	known := KnownRecords(in)
	if in.CompanyID == "" || in.Standing == (Standing{}) {
		return known
	}
	company := claims.Evidence{EntityType: citeCompany, EntityID: in.CompanyID}
	known[company] += "\n" + claims.Source(in.Standing)
	return known
}
