// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The standard fields a seller filters on: who a record is, where it is, when
// it arrived and when it last moved, and what a deal is worth. They join each
// engine's ownership and account leaves in vocab.go, and this file also holds
// what a caller's own authority does to the vocabulary once it is resolved.

import (
	"context"
	"errors"
	"maps"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/contactaddress"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
)

// The wire names, shared by the engines that carry the same fact.
const (
	nameFilterField   = "name"
	emailField        = "email"
	titleField        = "title"
	companyIDField    = "company_id"
	countryField      = "country"
	cityField         = "city"
	createdAtField    = "created_at"
	lastActivityField = "last_activity_at"
	amountField       = "amount"
)

// dayOf types a timestamp column as the calendar day it falls on, so `eq` on a
// date means that day and {"days_ago": N} compares day to day.
func dayOf(column string) storekit.Field {
	return storekit.Field{Expr: "t." + column, Type: storekit.FieldDate, Instant: true}
}

func textOf(column string) storekit.Field {
	return storekit.Field{Expr: "t." + column, Type: storekit.FieldText}
}

// countryOf compares the ISO code case-insensitively: nothing normalises the
// column, and `de` asks the same question as `DE`.
func countryOf() storekit.Field {
	return storekit.Field{Expr: "t.address_country", Type: storekit.FieldText, FoldCase: true}
}

// contactEmailExpr is the address the contact list's Email column prints: the
// live address first in ReachableOrder.
const contactEmailExpr = `(SELECT ce.email FROM contact_email ce
	WHERE ce.contact_id = t.id AND ce.archived_at IS NULL` + contactaddress.ReachableOrder + ` LIMIT 1)`

// contactEmployerLink is the contact list's employer filter as a link leaf:
// the one current primary employment edge, which uq_rel_current_primary_employer
// keeps unique per contact. The contacts module spells the same predicate for
// its list parameter; both read employment.CurrentPrimarySQL, so "works there
// today" has one definition, and a module never imports its sibling.
var contactEmployerLink = "EXISTS (SELECT 1 FROM relationship rel WHERE rel.contact_id = t.id" +
	" AND rel.kind = 'employment' AND " + employment.CurrentPrimarySQL("rel") +
	" AND rel.archived_at IS NULL AND %s)"

// contactEmployerField is withheld until bindEmployerScope admits the caller:
// an edge discloses a contact and a company as a pair, which neither record's
// grant covers, so the leaf reads nothing on anybody's behalf by default.
var contactEmployerField = storekit.Field{
	Expr: "rel.company_id", Type: storekit.FieldID, References: storekit.RefCompany,
	Link: contactEmployerLink, Withheld: true,
}

var contactStandardFields = map[string]storekit.Field{
	nameFilterField:   textOf("full_name"),
	emailField:        {Expr: contactEmailExpr, Type: storekit.FieldText, FoldCase: true},
	titleField:        textOf("title"),
	companyIDField:    contactEmployerField,
	countryField:      countryOf(),
	cityField:         textOf("address_city"),
	createdAtField:    dayOf("created_at"),
	lastActivityField: dayOf("last_activity_at"),
}

var companyStandardFields = map[string]storekit.Field{
	nameFilterField:   textOf("display_name"),
	countryField:      countryOf(),
	cityField:         textOf("address_city"),
	createdAtField:    dayOf("created_at"),
	lastActivityField: dayOf("last_activity_at"),
}

// unpricedDealAmount stands for the deal amount until SegmentEngine binds the
// expression compose injects (WithDealAmount): converting money reads the rate
// sheet, which this module does not own, so an unwired store offers the field
// and compiles it to FALSE.
var unpricedDealAmount = storekit.Field{Expr: "NULL::bigint", Type: storekit.FieldCurrency, Withheld: true}

var dealStandardFields = map[string]storekit.Field{
	nameFilterField:       textOf("name"),
	amountField:           unpricedDealAmount,
	"expected_close_date": {Expr: "t.expected_close_date", Type: storekit.FieldDate},
	createdAtField:        dayOf("created_at"),
	lastActivityField:     dayOf("last_activity_at"),
}

// leadStandardFields' score is the effective one, override included; the cast lets a fractional
// operand compare rather than fail to bind into a smallint.
var leadStandardFields = map[string]storekit.Field{
	nameFilterField: textOf("full_name"),
	emailField:      {Expr: "t.email", Type: storekit.FieldText, FoldCase: true},
	"source":        textOf("source"),
	"score":         {Expr: "t.score::numeric", Type: storekit.FieldNumber},
	createdAtField:  dayOf("created_at"),
}

// standardFields is each engine's standard set, keyed like segmentEngines.
var standardFields = map[string]map[string]storekit.Field{
	typeContact: contactStandardFields,
	typeCompany: companyStandardFields,
	typeDeal:    dealStandardFields,
	typeLead:    leadStandardFields,
}

// withFields is one engine's vocabulary assembled from its parts, a later part
// winning a name. TestNoEngineLeafShadowsAStandardField holds that no name is
// claimed twice, so the order never decides anything.
func withFields(parts ...map[string]storekit.Field) map[string]storekit.Field {
	out := map[string]storekit.Field{}
	for _, part := range parts {
		maps.Copy(out, part)
	}
	return out
}

// WithDealAmount injects a deal's worth in the installation's base currency,
// over the deal aliased t, for the filter builder's amount leaf. An open deal
// the rate sheet cannot price has no worth and matches only `exists: false`, so
// the leaf never compares two currencies.
func (s *Store) WithDealAmount(expr string) *Store {
	s.dealAmount = expr
	return s
}

// WithBaseCurrency injects the reader of the installation's base currency,
// which a core money field is counted in. identity owns the setting.
func (s *Store) WithBaseCurrency(read func(ctx context.Context, tx pgx.Tx) (string, error)) *Store {
	s.baseCurrencyOf = read
	return s
}

// readableBaseCurrency is the base currency when the vocabulary carries a core
// money field and this reader may read the setting, and "" otherwise: an
// unknown currency leaves the operand in minor units, it does not fail the read.
func (s *Store) readableBaseCurrency(ctx context.Context, fields map[string]storekit.Field) (string, error) {
	if s.baseCurrencyOf == nil || fields[amountField].Type != storekit.FieldCurrency {
		return "", nil
	}
	var base string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		base, err = s.baseCurrencyOf(ctx, tx)
		return err
	})
	var unset settings.UnsetValue
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.As(err, &unset) {
		return "", nil
	}
	return base, err
}

// bindDealAmount prices the amount leaf with the injected expression.
func (s *Store) bindDealAmount(resource string, fields map[string]storekit.Field) {
	if resource != typeDeal || s.dealAmount == "" {
		return
	}
	fields[amountField] = storekit.Field{Expr: s.dealAmount, Type: storekit.FieldCurrency}
}

// maskedAs names the masks a filter field answers to where they are not the
// field's own name. The deal amount compares the base value, so it answers to
// the masks fieldmask.DealBaseValueInputs names.
var maskedAs = map[string]map[string][]string{
	typeDeal: {amountField: fieldmask.DealBaseValueInputs()},
}

// withholdFromCaller stamps every field this caller may not filter by. A masked
// field compiles to FALSE rather than to its comparison, so neither a count, a
// membership nor a why can hand back a value the record read withholds.
func withholdFromCaller(ctx context.Context, resource string, fields map[string]storekit.Field) {
	if employer, ok := fields[companyIDField]; ok && resource == typeContact {
		fields[companyIDField] = bindEmployerScope(ctx, employer)
	}
	if tag, ok := fields[tagFilterField]; ok && tagWordsWithheld(ctx) {
		tag.Withheld = true
		fields[tagFilterField] = tag
	}
	for name, field := range fields {
		maskNames, aliased := maskedAs[resource][name]
		if !aliased {
			maskNames = []string{name}
		}
		for _, maskName := range maskNames {
			if maskedForCaller(ctx, resource, maskName) {
				field.Withheld = true
				fields[name] = field
			}
		}
	}
}

// maskedForCaller answers whether any mask of this caller's reaches the field.
// A context carrying no principal has no masks to read, and is answered as
// masked: that is the direction that cannot disclose.
func maskedForCaller(ctx context.Context, object, field string) bool {
	masked, err := auth.MasksAnyRowOf(ctx, object, field)
	return err != nil || masked
}

// bindEmployerScope admits the employer leaf for a caller who may read edges,
// bounded to the edges they may read — the gate the contact list's employer
// filter takes. A caller refused the edge keeps the field, withheld.
func bindEmployerScope(ctx context.Context, field storekit.Field) storekit.Field {
	if auth.EdgeReadAdmitted(ctx) != nil {
		return field
	}
	field.Withheld = false
	field.LinkScope = func(arg func(any) int) (string, error) {
		return auth.EdgeReadScope(ctx, "rel", arg)
	}
	return field
}
