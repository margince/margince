// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// readerFor builds a context whose principal may read exactly the named
// record types and nothing else.
func readerFor(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, o := range objects {
		grants[o] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:test",
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

// stubCatalog is the fieldcatalog seam, so a test can add and retire a custom
// field without a database. The seam is the boundary; mocking it is what
// mocking a boundary is for.
type stubCatalog struct {
	columns map[string][]fieldcatalog.Column
	err     error
}

func (c stubCatalog) ActiveColumns(_ context.Context, object string) ([]fieldcatalog.Column, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.columns[object], nil
}

// SEARCH-AC-15's first half, as a fitness function: the vocabulary is a
// derivation of the contract, not a list beside it. The walk here is
// deliberately a SECOND, independent implementation of the rule — every
// scalar json member of the contract record is askable — so a resolver that
// grew a hand-maintained list would disagree with it the moment the contract
// moved, which is the failure this criterion asks to be impossible to miss.
func TestVocabularyIsDerivedFromTheContractNotAList(t *testing.T) {
	resolver := NewVocabularyResolver()
	for entity, record := range contractRecords {
		vocab, err := resolver.Resolve(readerFor(entity), entity)
		if err != nil {
			t.Fatalf("%s: %v", entity, err)
		}
		target, ok := vocab.Target(entity)
		if !ok {
			t.Fatalf("%s: readable record type absent from its own vocabulary", entity)
		}
		for _, want := range independentlyDerivedScalars(record) {
			if _, ok := target.Field(want); !ok {
				t.Errorf("%s: contract member %q is not in the vocabulary; the vocabulary is not derived from the contract", entity, want)
			}
		}
		for _, got := range target.Fields {
			if strings.HasPrefix(got.Name, "cf_") {
				continue
			}
			if !slices.Contains(independentlyDerivedScalars(record), got.Name) && got.Kind != KindGeo {
				t.Errorf("%s: vocabulary offers %q, which the contract does not declare as a scalar", entity, got.Name)
			}
		}
	}
}

// The fitness function above shares scalarKind with production, so a
// mis-classification INSIDE it is invisible to both. These are the anchors
// that pin it: a known field of each kind, by name, with the operator set its
// kind must give it. Delete the time.Time identity check and every timestamp
// silently leaves every vocabulary — this is what notices.
func TestKnownFieldsResolveToTheKindTheirContractTypeGivesThem(t *testing.T) {
	vocab, err := NewVocabularyResolver().Resolve(readerFor("deal"), "deal")
	if err != nil {
		t.Fatal(err)
	}
	deal, _ := vocab.Target("deal")
	for _, want := range []struct {
		name string
		kind FieldKind
		op   string
	}{
		{"created_at", KindTimestamp, OpGt},      // time.Time — a struct
		{"expected_close_date", KindDate, OpLte}, // openapi_types.Date — a struct
		{"id", KindID, OpIn},                     // openapi_types.UUID — an array
		{"amount_minor", KindNumber, OpGte},      // *int64
		{"name", KindText, OpEq},                 // string
		{"close_date_provisional", KindBoolean, OpEq},
		{"version", KindNumber, OpGt}, // RowVersion — an int64 alias
	} {
		field, ok := deal.Field(want.name)
		if !ok {
			t.Errorf("deal.%s is not in the vocabulary at all", want.name)
			continue
		}
		if field.Kind != want.kind {
			t.Errorf("deal.%s resolved as kind %q, want %q", want.name, field.Kind, want.kind)
		}
		if !slices.Contains(field.Ops, want.op) {
			t.Errorf("deal.%s (%s) does not admit %q; its ops are %v", want.name, field.Kind, want.op, field.Ops)
		}
	}
}

// Every relation any record declares must be resolvable by the validator's
// narrowing pass, or a published hop refuses for a caller who may take it.
// The two are derived by separate rules (relation NAMING vs the name→target
// guess), and this is what keeps them in agreement — renaming one without the
// other fails here rather than in production.
func TestEveryDerivedRelationNameIsResolvableByTheValidatorsNarrowing(t *testing.T) {
	everything := make([]string, 0, len(contractRecords))
	for entity := range contractRecords {
		everything = append(everything, entity)
	}
	// Both postures, because they publish different relation sets and only one
	// of them publishes a join edge. Unwired, the vocabulary is the contract's
	// and carries scalar edges alone — so a resolver with no column reader
	// cannot exercise the plural rule on a record type no contract member
	// references, which is every join hop.
	for _, resolver := range map[string]*VocabularyResolver{
		"contract only": NewVocabularyResolver(),
		"with a schema": NewVocabularyResolver().WithColumnReader(stubColumns{tables: joinSchema}),
	} {
		vocab, err := resolver.Resolve(readerFor(everything...))
		if err != nil {
			t.Fatal(err)
		}
		hops := 0
		for _, target := range vocab.Targets {
			for _, relation := range target.Relations {
				hops++
				narrowed := hopTargets(Plan{Traverse: &Traversal{Relation: relation.Name}})
				if !slices.Contains(narrowed, relation.Target) {
					t.Errorf("%s declares hop %q onto %q, but the validator's narrowing resolves it to %v",
						target.Target, relation.Name, relation.Target, narrowed)
				}
			}
		}
		if hops == 0 {
			t.Error("no hops were checked, so the two rules were never compared")
		}
	}
}

// independentlyDerivedScalars re-derives the expected field names from the
// contract type without using the production walk.
func independentlyDerivedScalars(t reflect.Type) []string {
	var names []string
	for i := range t.NumField() {
		member := t.Field(i)
		tag := member.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if !member.IsExported() || name == "" || name == "-" {
			continue
		}
		inner := member.Type
		if inner.Kind() == reflect.Pointer {
			inner = inner.Elem()
		}
		if _, scalar := scalarKind(inner); scalar {
			names = append(names, name)
			continue
		}
		if inner.Kind() != reflect.Struct {
			continue
		}
		for j := range inner.NumField() {
			leaf := inner.Field(j)
			leafName, _, _ := strings.Cut(leaf.Tag.Get("json"), ",")
			if !leaf.IsExported() || leafName == "" || leafName == "-" {
				continue
			}
			leafType := leaf.Type
			if leafType.Kind() == reflect.Pointer {
				leafType = leafType.Elem()
			}
			if _, scalar := scalarKind(leafType); scalar {
				names = append(names, name+"."+leafName)
			}
		}
	}
	return names
}

// SEARCH-AC-15's second half: the custom-field catalog moves and the
// vocabulary moves with it, with no edit anywhere.
func TestACustomFieldEntersAndLeavesTheVocabularyWithTheCatalog(t *testing.T) {
	catalog := stubCatalog{columns: map[string][]fieldcatalog.Column{
		"deal": {{Name: "cf_renewal_risk", Type: fieldcatalog.TypePicklist}},
	}}
	ctx := readerFor("deal")

	added, err := NewVocabularyResolver().WithFieldCatalog(catalog).Resolve(ctx, "deal")
	if err != nil {
		t.Fatal(err)
	}
	deal, _ := added.Target("deal")
	field, ok := deal.Field("cf_renewal_risk")
	if !ok {
		t.Fatal("a custom field active in the catalog is not askable")
	}
	if field.Kind != KindText || !slices.Contains(field.Ops, OpIn) {
		t.Errorf("picklist custom field resolved as %q with ops %v; want a text field admitting %q", field.Kind, field.Ops, OpIn)
	}

	retired, err := NewVocabularyResolver().WithFieldCatalog(stubCatalog{}).Resolve(ctx, "deal")
	if err != nil {
		t.Fatal(err)
	}
	deal, _ = retired.Target("deal")
	if _, ok := deal.Field("cf_renewal_risk"); ok {
		t.Error("a retired custom field is still askable; the vocabulary is cached or maintained rather than derived")
	}
}

// SEARCH-AC-16: the vocabulary is per-caller, and what one caller cannot read
// is simply absent for them rather than refused differently.
func TestARecordTypeTheCallerCannotReadIsAbsentFromTheirVocabulary(t *testing.T) {
	resolver := NewVocabularyResolver()

	wide, err := resolver.Resolve(readerFor("deal", "company"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := wide.Target("deal"); !ok {
		t.Fatal("a readable record type is missing from the vocabulary")
	}

	narrow, err := resolver.Resolve(readerFor("company"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := narrow.Target("deal"); ok {
		t.Error("a record type the caller cannot read is in their vocabulary")
	}
	if slices.Contains(narrow.TargetNames(), "deal") {
		t.Error("a record type the caller cannot read is advertised in their target list")
	}
}

// A hop is a read of what it lands on, so a relation into a denied record
// type is not offered — otherwise a plan could filter by rows the caller
// cannot see and learn their contents from the count.
func TestAHopIntoADeniedRecordTypeIsNotOffered(t *testing.T) {
	resolver := NewVocabularyResolver()

	both, err := resolver.Resolve(readerFor("deal", "company"))
	if err != nil {
		t.Fatal(err)
	}
	deal, _ := both.Target("deal")
	if _, ok := deal.Relation("company"); !ok {
		t.Fatal("deal declares company_id but the hop is not offered to a caller who reads both")
	}

	dealOnly, err := resolver.Resolve(readerFor("deal"))
	if err != nil {
		t.Fatal(err)
	}
	deal, _ = dealOnly.Target("deal")
	if _, ok := deal.Relation("company"); ok {
		t.Error("a hop into a record type the caller cannot read is offered")
	}
}

// The inverse edge is derived too: company never declares a deal
// reference, but deal declares one to it.
func TestTheInverseOfADeclaredReferenceIsTraversable(t *testing.T) {
	vocab, err := NewVocabularyResolver().Resolve(readerFor("deal", "company"))
	if err != nil {
		t.Fatal(err)
	}
	company, _ := vocab.Target("company")
	relation, ok := company.Relation("deals")
	if !ok {
		t.Fatal("company has no inverse hop for deal.company_id")
	}
	if relation.Target != "deal" || relation.Via != "deal.company_id" {
		t.Errorf("inverse hop resolved to %+v; want a deal hop via deal.company_id", relation)
	}
}

// Every searchable entity must have a contract binding, or its vocabulary
// cannot be derived at all. This is the check that fails when a seventh
// searchable entity is added without one — the alternative being a runtime
// error on the first plan that names it.
func TestEverySearchableEntityHasAContractBinding(t *testing.T) {
	for _, branch := range searchBranches {
		// A text-only branch answers the name search and nothing else, so it
		// has no record vocabulary to derive and needs no binding.
		if branch.textOnly {
			continue
		}
		if contractRecords[branch.entity] == nil {
			t.Errorf("searchable entity %q has no contract record binding, so no vocabulary can be derived for it", branch.entity)
		}
	}
	for entity := range contractRecords {
		if !slices.ContainsFunc(searchBranches, func(b searchBranch) bool { return b.entity == entity }) {
			t.Errorf("contract binding %q names an entity this module does not search", entity)
		}
	}
}

// The six closed custom-field types must each map onto a kind, or a
// workspace's own column silently vanishes from its vocabulary.
func TestEveryCustomFieldTypeHasAVocabularyKind(t *testing.T) {
	// The port's own set, not a copy of it: a list restated here would pass
	// unchanged the day a seventh type is added, which is the one moment this
	// test exists to fail.
	for _, declared := range fieldcatalog.Types() {
		kind, ok := customFieldKinds[declared]
		if !ok {
			t.Errorf("custom-field type %q has no vocabulary kind, so a workspace column of that type is unaskable", declared)
			continue
		}
		if len(operatorsByKind[kind]) == 0 {
			t.Errorf("custom-field type %q maps to kind %q, which admits no operators", declared, kind)
		}
	}
}

// Every kind a field can carry must admit at least one operator, or a field
// of that kind is in the vocabulary and unusable.
func TestEveryFieldKindAdmitsAnOperator(t *testing.T) {
	for _, kind := range []FieldKind{KindText, KindNumber, KindBoolean, KindDate, KindTimestamp, KindID, KindGeo} {
		if len(newField("x", kind).Ops) == 0 {
			t.Errorf("kind %q admits no operator", kind)
		}
	}
}

// A catalog fault is reported, never swallowed into a narrower vocabulary: a
// silently smaller vocabulary refuses a legitimate field as unknown, which
// reads to the caller exactly like a plan they got wrong.
func TestACatalogFaultRefusesTheResolveRatherThanNarrowingIt(t *testing.T) {
	boom := errors.New("catalog unavailable")
	_, err := NewVocabularyResolver().WithFieldCatalog(stubCatalog{err: boom}).Resolve(readerFor("deal"), "deal")
	if !errors.Is(err, boom) {
		t.Fatalf("resolve returned %v; want the catalog fault", err)
	}
}

// Collections and free-form blobs carry no single value, so no operator here
// could mean anything against them.
func TestCollectionsAndBlobsAreNotAskable(t *testing.T) {
	vocab, err := NewVocabularyResolver().Resolve(readerFor("contact"), "contact")
	if err != nil {
		t.Fatal(err)
	}
	contact, _ := vocab.Target("contact")
	for _, name := range []string{"emails", "phones", "raw", "social", "consent"} {
		if _, ok := contact.Field(name); ok {
			t.Errorf("%q is askable, but it carries a collection or a free-form blob rather than a value", name)
		}
	}
}

// A nested contract object contributes its leaves under a dotted path, and
// the object itself contributes the place a radius would be measured from.
func TestANestedAddressContributesLeavesAndAPlace(t *testing.T) {
	vocab, err := NewVocabularyResolver().Resolve(readerFor("company"), "company")
	if err != nil {
		t.Fatal(err)
	}
	company, _ := vocab.Target("company")
	city, ok := company.Field("address.city")
	if !ok || city.Kind != KindText {
		t.Fatalf("address.city resolved to %+v, %v; want an exact text predicate", city, ok)
	}
	place, ok := company.Field("address")
	if !ok || place.Kind != KindGeo || !slices.Contains(place.Ops, OpWithinRadius) {
		t.Fatalf("address resolved to %+v, %v; want a place admitting %q", place, ok, OpWithinRadius)
	}
	if slices.Contains(place.Ops, OpEq) {
		t.Error("a place admits an equality predicate; a place does not compare equal to anything")
	}
}

// The contract half of the vocabulary is memoized, and a resolve APPENDS its
// workspace's custom columns to it and sorts the result. Both would reach
// through a shared backing array into the next caller's vocabulary if the
// memoized walk were handed out directly — one workspace's private column
// becoming another's, and a retired one never leaving.
func TestOneResolvesCustomColumnsDoNotLeakIntoTheNext(t *testing.T) {
	withColumn := NewVocabularyResolver().WithFieldCatalog(stubCatalog{
		columns: map[string][]fieldcatalog.Column{
			"deal": {{Name: "cf_private_to_this_read", Type: fieldcatalog.TypeText}},
		},
	})
	if _, err := withColumn.Resolve(readerFor("deal"), "deal"); err != nil {
		t.Fatal(err)
	}

	// A DIFFERENT resolver over an empty catalog — the only thing the two
	// share is the memoized contract walk.
	plain, err := NewVocabularyResolver().Resolve(readerFor("deal"), "deal")
	if err != nil {
		t.Fatal(err)
	}
	deal, _ := plain.Target("deal")
	if _, ok := deal.Field("cf_private_to_this_read"); ok {
		t.Fatal("a custom column from one resolve appeared in another's vocabulary")
	}

	// The check above only bites while the memoized slice has spare capacity
	// for the append to land in. Pin the property itself, so a contract whose
	// field count happens to fill the allocation exactly cannot make this
	// suite pass against the bug it describes.
	for entity, record := range contractRecords {
		if len(contractFields(record)) == 0 {
			t.Fatalf("%s derives no fields, so the aliasing check below is vacuous", entity)
		}
		if &contractFields(record)[0] == &walkedContracts()[record][0] {
			t.Errorf("%s: contractFields hands out the memoized slice itself; a caller's append reaches every other caller", entity)
		}
	}
}

// A Field travels out of this package, so its operator set must not be the
// shared map's own slice — one caller's edit would rewrite what every field
// of that kind admits, for every later caller.
func TestAFieldsOperatorsAreNotTheSharedSetItself(t *testing.T) {
	first := newField("a", KindNumber)
	if len(first.Ops) == 0 {
		t.Fatal("a number field admits no operators")
	}
	if &first.Ops[0] == &operatorsByKind[KindNumber][0] {
		t.Fatal("a Field hands out the kind's own operator slice; a caller's edit would reach every other field")
	}
	first.Ops[0] = "mutated"
	if newField("b", KindNumber).Ops[0] == "mutated" {
		t.Error("editing one field's operators changed what the next field admits")
	}
}

// maskedReaderFor is readerFor with a field mask on the deal, the only object
// this build can withhold columns of.
//
// The row scope is narrowed off RowScopeAll deliberately: a principal reading
// every row is unbounded, and an unbounded principal reads every column by
// design. Left wide, this fixture would assert nothing and pass.
func maskedReaderFor(field string, objects ...string) context.Context {
	ctx := readerFor(objects...)
	p, _ := principal.Actor(ctx)
	p.Permissions.RowScope = principal.RowScopeOwn
	p.Permissions.FieldMasks = []principal.FieldMask{{Object: fieldmask.Deal, Field: field}}
	return principal.WithActor(ctx, p)
}

// A predicate is a read of the value it tests, so a masked column is not
// askable.
//
// `amount_minor >= N` never returns the amount and does not have to: the hit
// or the miss answers the question, and a caller who may not read the figure
// recovers it by bisection. The list read has refused a sort or filter over a
// masked column all along; this is the plan vocabulary asking the same
// question, which is where the agent surface reaches the same columns.
//
// The whole GROUP goes, not the configured name: masking the amount while the
// ARR stays filterable leaves the size of the deal recoverable from the field
// beside it.
func TestAMaskedColumnIsNotAskable(t *testing.T) {
	resolver := NewVocabularyResolver()

	wide, err := resolver.Resolve(readerFor("deal"), "deal")
	if err != nil {
		t.Fatal(err)
	}
	deal, ok := wide.Target("deal")
	if !ok {
		t.Fatal("the deal is missing from an unmasked reader's vocabulary")
	}
	for _, field := range fieldmask.Maskable(fieldmask.Deal) {
		if _, askable := deal.Field(field); !askable {
			// Not every maskable field is a stored scalar the vocabulary
			// publishes; only the ones that are can be dropped from it.
			continue
		}
		masked, err := resolver.Resolve(maskedReaderFor(field, "deal"), "deal")
		if err != nil {
			t.Fatal(err)
		}
		narrow, ok := masked.Target("deal")
		if !ok {
			t.Fatal("a masked column removed the whole record type from the vocabulary")
		}
		for _, withheld := range fieldmask.Withheld(fieldmask.Deal, []string{field}) {
			if _, askable := narrow.Field(withheld); askable {
				t.Errorf("a mask on %q left %q askable — a predicate over it is an oracle for a "+
					"value this caller may not read", field, withheld)
			}
		}
	}
}

// A mask on a reader who reads every row takes nothing out of their vocabulary.
//
// Reading every row is reading every column — auth.Unbounded says so, and every
// mask arm in the tree returns "nothing withheld" for such a principal. The
// same masks that emptied a bounded reader's fields above must leave this one's
// whole, which is what makes the arm a narrowing of the caller rather than of
// the contract.
func TestAMaskOnAnUnboundedReaderNarrowsNothing(t *testing.T) {
	resolver := NewVocabularyResolver()

	// Counted, and asserted below. The baseline is itself resolved through the
	// arm under test, so a broken arm empties it, every field is skipped, and
	// this test passes having checked nothing.
	checked := 0
	for _, field := range fieldmask.Maskable(fieldmask.Deal) {
		plain, err := resolver.Resolve(readerFor("deal"), "deal")
		if err != nil {
			t.Fatal(err)
		}
		unmasked, ok := plain.Target("deal")
		if !ok {
			t.Fatal("the deal is missing from a reader's vocabulary")
		}
		if _, askable := unmasked.Field(field); !askable {
			continue
		}
		checked++
		// readerFor leaves RowScopeAll, so this principal carries the mask AND
		// reads every row.
		ctx := readerFor("deal")
		p, _ := principal.Actor(ctx)
		p.Permissions.FieldMasks = []principal.FieldMask{{Object: fieldmask.Deal, Field: field}}
		wide, err := resolver.Resolve(principal.WithActor(ctx, p), "deal")
		if err != nil {
			t.Fatal(err)
		}
		deal, ok := wide.Target("deal")
		if !ok {
			t.Fatal("a mask removed the record type from an unbounded reader's vocabulary")
		}
		if _, askable := deal.Field(field); !askable {
			t.Errorf("a mask on %q narrowed a reader who reads every row — they already read every "+
				"column, so the arm has taken away something no rule withholds", field)
		}
	}
	if checked == 0 {
		t.Fatal("no maskable field was askable even before a mask was applied — the vocabulary is " +
			"empty or the mask arm drops everything, and either way this test asserted nothing")
	}
}

// With nobody bound to the context, the vocabulary REFUSES.
//
// The mask arm cannot answer "does this caller read this column" without a
// caller, and the tempting failure is to treat "no masks found" as "nothing is
// masked" — which publishes every column to a request that never proved who
// made it. It fails closed instead, and the error travels rather than being
// swallowed into a narrowing nobody asked for.
func TestAVocabularyWithNoActorRefusesRatherThanPublishingEveryField(t *testing.T) {
	t.Parallel()

	fields := []Field{newField("amount_minor", KindNumber)}
	if _, err := admittedFields(context.Background(), "deal", fields); err == nil {
		t.Error("an unauthenticated resolve returned a vocabulary — a predicate compiled off it " +
			"would read a column nobody established the caller may see")
	}
}
