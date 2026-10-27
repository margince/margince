// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
)

// SegmentEngine returns the one predicate engine for a filterable resource,
// whether it is one of core's record types or a kind an extension declared.
// ok is false for a resource with no engine, such as activities and partners,
// and the caller decides what that means.
func (s *Store) SegmentEngine(ctx context.Context, resource string) (storekit.Query, bool, error) {
	if recordkinds.IsExtensionName(resource) {
		engine, declared := extensionEngine(ctx, resource)
		return engine, declared, nil
	}
	return s.coreSegmentEngine(ctx, resource)
}

// extensionVocabulary lists an extension engine's fields by name, as
// FilterVocabulary lists a core engine's.
func extensionVocabulary(engine storekit.Query) []VocabularyField {
	fields := make([]VocabularyField, 0, len(engine.Fields))
	for name, field := range engine.Fields {
		fields = append(fields, VocabularyField{Name: name, Type: string(field.Type), Operators: storekit.OperatorsFor(field)})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	return fields
}

// extensionEngine is the predicate engine of a kind an extension declared. It is
// built from the declaration on each call, because the registry is filled at
// boot. Every declared field is a text column the unit owns.
//
// Its rows carry no owner, so the kind's read right (Object) is the whole row
// scope. A kind the caller may not read answers as no kind at all.
func extensionEngine(ctx context.Context, kind string) (storekit.Query, bool) {
	declared, ok := recordkinds.Lookup(kind)
	if !ok || !auth.Allows(ctx, kind, principal.ActionRead) {
		return storekit.Query{}, false
	}
	fields := make(map[string]storekit.Field, len(declared.Fields))
	for _, field := range declared.Fields {
		fields[field.Name] = storekit.Field{Expr: "t." + pgx.Identifier{field.Name}.Sanitize(), Type: storekit.FieldText}
	}
	withholdFromCaller(ctx, kind, fields)
	return storekit.Query{
		Table:     pgx.Identifier{"ext", declared.Table}.Sanitize(),
		Object:    kind,
		BaseWhere: whereArchivedNull,
		Fields:    fields,
	}, true
}
