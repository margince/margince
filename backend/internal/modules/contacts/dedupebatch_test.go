// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// batchFileRows is the size of file the batch half has to answer.
const batchFileRows = 5000

// The batch files each company under the keys a pair needs, so it meets its
// neighbours only. Counts, not a stopwatch, answer the same under coverage.
// The contacts share one employer domain and meet pairwise, but ask the
// database at most once per row and kind.
func TestTheBatchMeetsACompanyAtItsNeighboursOnly(t *testing.T) {
	syllables := strings.Fields("ka lo mi ra tes vin dor pel su na bri gal hu fen zo tar lis mur qua ex")
	words := rand.New(rand.NewPCG(7, 7))
	word := func() string {
		var name strings.Builder
		for range 2 + words.IntN(2) {
			name.WriteString(syllables[words.IntN(len(syllables))])
		}
		return name.String()
	}
	admissions, companyPairs := 0, 0
	inTx := func(func(pgx.Tx) error) error { admissions++; return nil }
	ctx := context.Background()
	var batch Batch
	for i := range batchFileRows {
		line := fmt.Sprintf("%s Straße %d", word(), i%40)
		company := NewCompany{
			DisplayName: word() + " " + word() + " GmbH", Domains: []string{fmt.Sprintf("c%d.example", i)},
			Address: &crmcontracts.Address{Line1: &line, City: new("Berlin")},
		}
		companyPairs += len(batch.companyNeighbours(heldCompanyOf(company)))
		if _, err := batch.CompanyLookAlike(ctx, inTx, company); err != nil {
			t.Fatal(err)
		}
		batch.AddCompany(company)
		contact := NewContact{FullName: word() + " " + word(), Emails: []string{fmt.Sprintf("p%d@bigcorp.example", i)}}
		if _, err := batch.ContactLookAlike(ctx, inTx, contact, nil); err != nil {
			t.Fatal(err)
		}
		batch.AddContact(contact)
	}
	// The seeded file meets 0.5% of all pairs; a scan that drops the keys meets all.
	if allPairs := batchFileRows * (batchFileRows - 1) / 2; companyPairs*100 > allPairs {
		t.Errorf("the batch met %d company pairs of %d, over 1%%: a row meets more than its neighbours", companyPairs, allPairs)
	}
	if most := 2 * batchFileRows; admissions > most {
		t.Errorf("the batch asked the database %d times for %d rows of each kind, over one query per row and kind", admissions, batchFileRows)
	}
}

// The batch asks whether a pair reaches the review bar without the full score.
// The two answers must agree on every name pair and employer agreement.
func TestTheReachTestAgreesWithTheContactScore(t *testing.T) {
	for _, names := range contactNameCorroborationCases {
		asked, held := contactNameFormsOf(names.candidate), contactNameFormsOf(names.incumbent)
		for _, agreement := range []float64{0, 0.8, 1} {
			for _, bar := range []float64{0.3, dedupeReviewThreshold, 0.9} {
				want := contactConfidenceOf(asked, held, agreement) >= bar
				if got := contactConfidenceReaches(asked, held, agreement, bar); got != want {
					t.Errorf("%q against %q at agreement %.1f, bar %.2f: reaches = %v, score says %v",
						names.candidate, names.incumbent, agreement, bar, got, want)
				}
			}
		}
	}
}
