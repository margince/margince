// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// A predicate is a READ of the field it names. The statement projects no
// masked column, so the leak was never the projection: a caller who may not
// read a deal's amount can still compile `amount_minor >= N` and take the
// figure off which rows come back, one bisection at a time.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskedAmount is the deal money mask an administrator configures.
var maskedAmount = principal.FieldMask{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}

// amountAtLeast is the oracle's first question: answer it thirty times and the
// amount is pinned exactly.
const amountAtLeast = `{"version":"v1","target":"deal","where":[{"field":"amount_minor","op":"gte","value":100000}]}`

// readerMasking reads the named record types under one mask. The row scope is
// narrow on purpose — a caller who reads every row carries no mask at all, so
// a test built on readerFor would assert nothing.
func readerMasking(mask principal.FieldMask, objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, o := range objects {
		grants[o] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		ID:   "human:masked",
		Permissions: principal.Permissions{
			Objects: grants, RowScope: principal.RowScopeTeam, FieldMasks: []principal.FieldMask{mask},
		},
	})
}

func TestAPredicateOnAMaskedFieldIsRefusedInItsOwnWords(t *testing.T) {
	_, err := validateJSON(readerMasking(maskedAmount, "deal"), t, amountAtLeast)
	fault := singleFault(t, err)
	if fault.Code != auth.CodeFieldMasked {
		t.Errorf("filtering on a masked amount refused as %q; want %q", fault.Code, auth.CodeFieldMasked)
	}
	if fault.Field != "where[0].field" {
		t.Errorf("the refusal points at %q, not at the predicate the caller has to drop", fault.Field)
	}
	if !strings.Contains(fault.Message, "amount_minor") || !strings.Contains(fault.Message, "your role") {
		t.Errorf("the refusal says neither which field is withheld nor why: %q", fault.Message)
	}
}

// A mask takes its group with it, here as everywhere else: a currency beside a
// withheld amount reads as a priced deal missing its figure, and an expected
// ARR is the same money in a second column.
func TestTheMaskGroupIsRefusedAsAFilterToo(t *testing.T) {
	ctx := readerMasking(maskedAmount, "deal")
	for _, clause := range []struct{ field, value string }{
		{"currency", `"EUR"`},
		{"expected_arr_minor", `100000`},
	} {
		t.Run(clause.field, func(t *testing.T) {
			doc := `{"version":"v1","target":"deal","where":[{"field":"` + clause.field +
				`","op":"eq","value":` + clause.value + `}]}`
			fault := singleFault(t, mustRefuse(ctx, t, doc))
			if fault.Code != auth.CodeFieldMasked {
				t.Errorf("%s is withheld by the amount's mask but filters as %q", clause.field, fault.Code)
			}
		})
	}
}

// The second door. A hop's predicates are checked against the hop target's own
// vocabulary, so a mask that held only on the plan's target would leave the
// whole oracle standing one relation away.
func TestATraversalPredicateOnAMaskedFieldIsRefusedToo(t *testing.T) {
	ctx := readerMasking(maskedAmount, "company", "deal")
	err := mustRefuse(ctx, t, `{"version":"v1","target":"company","traverse":`+
		`{"relation":"deals","where":[{"field":"amount_minor","op":"gte","value":100000}]}}`)
	fault := singleFault(t, err)
	if fault.Code != auth.CodeFieldMasked {
		t.Errorf("the hop's predicate on a masked amount refused as %q; want %q", fault.Code, auth.CodeFieldMasked)
	}
	if fault.Field != "traverse.where[0].field" {
		t.Errorf("the refusal points at %q rather than at the hop's own predicate", fault.Field)
	}
}

// The refusal follows the FIELD the mask names, not the presence of a mask:
// this caller's role withholds a deal's description and nothing else, and the
// money is theirs to ask about.
func TestAnUnmaskedCallerStillFiltersOnTheAmount(t *testing.T) {
	elsewhere := principal.FieldMask{Object: "deal", Field: "description", Condition: principal.MaskAlways}
	validated, err := validateJSON(readerMasking(elsewhere, "deal"), t, amountAtLeast)
	if err != nil {
		t.Fatalf("a role that withholds no part of the money was refused the amount: %v", err)
	}
	if got := validated.Plan.Where[0].Field; got != "amount_minor" {
		t.Errorf("the validated plan carries %q rather than the predicate the caller wrote", got)
	}
}

// A record embeds another object's block and the vocabulary flattens it to a
// dotted name, so a company publishes sixteen of a partner's own fields. The
// mask names the partner's tier; the filter names the company's copy of it,
// and a stamp that asked only under the target would leave every one of them
// filterable.
func TestAMaskReachesAnEmbeddedBlockRepublishedOnAnotherRecord(t *testing.T) {
	tier := principal.FieldMask{Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways}
	ctx := readerMasking(tier, "company")
	fault := singleFault(t, mustRefuse(ctx, t,
		`{"version":"v1","target":"company","where":[{"field":"partner.margin_tier","op":"eq","value":"gold"}]}`))
	if fault.Code != auth.CodeFieldMasked {
		t.Errorf("a partner's tier republished on the company filters as %q; want %q", fault.Code, auth.CodeFieldMasked)
	}
}

// The census under the rule the test above spells: EVERY field a nested block
// flattens is reachable by a mask on the block's own object. Derived from the
// vocabulary rather than from a list of blocks, so the seventeenth member of
// one cannot fall off it unnoticed — which is exactly how the company's copy
// of a partner's tier stayed filterable in the first place.
func TestEveryFlattenedBlockFieldIsReachableByAMaskOnItsOwnObject(t *testing.T) {
	vocab, err := NewVocabularyResolver().Resolve(readerFor("company", "contact", "deal"))
	if err != nil {
		t.Fatal(err)
	}
	nested := 0
	for _, target := range vocab.Targets {
		for _, field := range target.Fields {
			block, leaf, dotted := strings.Cut(field.Name, ".")
			if !dotted {
				continue
			}
			nested++
			mask := principal.FieldMask{Object: block, Field: leaf, Condition: principal.MaskAlways}
			under, err := NewVocabularyResolver().Resolve(readerMasking(mask, target.Target), target.Target)
			if err != nil {
				t.Fatalf("%s under a mask on %s.%s: %v", target.Target, block, leaf, err)
			}
			masked, _ := under.Target(target.Target)
			if got, ok := masked.Field(field.Name); !ok || !got.Masked {
				t.Errorf("a mask on %s.%s does not reach %s's copy of it", block, leaf, target.Target)
			}
		}
	}
	// A census that walked an empty vocabulary reports PASS and asserts
	// nothing, which is the one way this may not fail.
	if nested == 0 {
		t.Fatal("no record type published a flattened block; the census read a smaller tree than it should have")
	}
}

// The dotted prefix is a wire member name, not a promise of a maskable object:
// most of them — `address`, `author`, `strength` — name nothing a mask can be
// configured under, and the lookup has to answer plainly false for those
// rather than erroring or resolving somewhere else.
func TestABlockThatNamesNoMaskableObjectStaysFilterable(t *testing.T) {
	tier := principal.FieldMask{Object: "partner", Field: "margin_tier", Condition: principal.MaskAlways}
	ctx := readerMasking(tier, "company")
	for _, field := range []string{"address.city", "author.display_name", "strength.bucket"} {
		t.Run(field, func(t *testing.T) {
			doc := `{"version":"v1","target":"company","where":[{"field":"` + field + `","op":"eq","value":"x"}]}`
			if _, err := validateJSON(ctx, t, doc); err != nil {
				t.Errorf("filtering on %s was refused: %v", field, err)
			}
		})
	}
}

// The guard on the reconciliation with SEARCH-AC-16. Identical wording protects
// a field whose EXISTENCE is the secret, which is a field on a record type the
// caller cannot read at all — and such a target is refused before any predicate
// is looked at, so the mask refusal can never answer for one.
func TestAnUnreadableTargetIsStillRefusedBeforeAnyPredicate(t *testing.T) {
	fault := singleFault(t, mustRefuse(readerMasking(maskedAmount, "contact"), t, amountAtLeast))
	if fault.Code != CodeUnknownTarget {
		t.Errorf("a caller who cannot read deals was refused with %q; want %q, which says nothing about what exists",
			fault.Code, CodeUnknownTarget)
	}
	if strings.Contains(fault.Message, "amount_minor") {
		t.Errorf("the refusal for an unreadable record type names a field on it: %q", fault.Message)
	}
}

// A client told only "no" concludes the product holds no such data and goes
// looking under four more spellings. The published vocabulary carries the
// field and says it is withheld, so the client can tell its user which of the
// two it is.
func TestThePublishedVocabularyMarksAMaskedField(t *testing.T) {
	body, err := NewQuerySchemaResource(NewVocabularyResolver()).
		VocabularyDocument(readerMasking(maskedAmount, "deal"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Targets []struct {
			Target string
			Fields []struct {
				Name   string
				Ops    []string
				Masked bool
			}
		}
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("the published vocabulary is not readable JSON: %v", err)
	}
	marked, published := map[string]bool{}, map[string][]string{}
	for _, target := range doc.Targets {
		for _, field := range target.Fields {
			marked[target.Target+"."+field.Name] = field.Masked
			published[target.Target+"."+field.Name] = field.Ops
		}
	}
	if _, published := marked["deal.amount_minor"]; !published {
		t.Fatal("the masked field is missing from the published vocabulary, which is the silence that teaches an agent it does not exist")
	}
	if !marked["deal.amount_minor"] {
		t.Error("the published vocabulary offers deal.amount_minor without saying this role cannot filter on it")
	}
	if marked["deal.name"] {
		t.Error("a field no mask names is published as withheld")
	}
	// The document exists so a client can say something true. Advertising an
	// operator that is certain to be refused sends it to build the one plan it
	// cannot have.
	if len(published["deal.amount_minor"]) > 0 {
		t.Errorf("the withheld field advertises %v; every one of them refuses", published["deal.amount_minor"])
	}
	if len(published["deal.name"]) == 0 {
		t.Error("an unmasked field lost its operators along with the masked one")
	}
}
