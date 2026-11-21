// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// batchFileRows is the size of file the batch half has to answer in time.
const batchFileRows = 5000

// A test run asks every row of a file against the rows before it. At 5,000
// rows that is 12.5 million pairs if each row meets every other, which took
// half an hour. The batch files each record under the keys a pair needs, so a
// row meets its neighbours only. The contacts all share one employer domain,
// the one key that files every one of them together.
func TestTheBatchAnswersAFileOfFiveThousandRowsInTime(t *testing.T) {
	syllables := strings.Fields("ka lo mi ra tes vin dor pel su na bri gal hu fen zo tar lis mur qua ex")
	words := rand.New(rand.NewPCG(7, 7))
	word := func() string {
		var name strings.Builder
		for range 2 + words.IntN(2) {
			name.WriteString(syllables[words.IntN(len(syllables))])
		}
		return name.String()
	}
	admissions := 0
	inTx := func(func(pgx.Tx) error) error { admissions++; return nil }
	ctx := context.Background()
	started := time.Now()
	var batch Batch
	for i := range batchFileRows {
		line := fmt.Sprintf("%s Straße %d", word(), i%40)
		company := NewCompany{
			DisplayName: word() + " " + word() + " GmbH", Domains: []string{fmt.Sprintf("c%d.example", i)},
			Address: &crmcontracts.Address{Line1: &line, City: new("Berlin")},
		}
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
	elapsed := time.Since(started)
	t.Logf("%d companies and %d contacts in %s, %d admission queries", batchFileRows, batchFileRows, elapsed, admissions)
	if ceiling := time.Minute; elapsed > ceiling {
		t.Fatalf("the batch took %s for %d rows of each kind, over the %s ceiling", elapsed, batchFileRows, ceiling)
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
