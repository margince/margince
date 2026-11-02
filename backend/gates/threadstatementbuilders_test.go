// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"context"
	"go/ast"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

func (p threadSQLPackage) canonicalClauseCall(call *ast.CallExpr, name string) bool {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	qualifier, ok := method.X.(*ast.Ident)
	if !ok {
		return false
	}
	owner := p.imports[qualifier.Name]
	switch name {
	case "ActivityContentClause", "ActivityDiscoverClause", "ActivityAvailableClause", "ActivityRepairPendingClause", "AudienceWorkspaceOnly", "ConnectorSeatSQL":
		return owner == "internal/platform/auth"
	case "HeldThroughRecordLinks":
		return owner == "internal/shared/kernel/retentionscope"
	case "CurrentCommitmentClause":
		return owner == "internal/shared/kernel/provenance"
	case "ThreadSQL", "ConversationSQL", "SubjectMailSQL":
		return owner == "internal/shared/kernel/correspondence"
	case "SignatureSourceEligible":
		return owner == "internal/modules/contacts"
	case "ClassifyBacklogPredicate":
		return owner == "internal/modules/activities"
	case "CurrentTaskClause":
		return owner == "internal/modules/activities" || owner == "internal/platform/database/storekit"
	case "StoredOriginalSQL", "CurrentSourceMailClause", "CitedActivityTime", "CitedActivityStamp", "CitedActivityPrecision":
		return owner == "internal/platform/database/storekit"
	}
	return false
}

func (p threadSQLPackage) clauseValues(name string, call *ast.CallExpr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) []threadSQLValue {
	indexes := []int{0}
	switch name {
	case "ActivityContentClause", "ActivityDiscoverClause", "ActivityRepairPendingClause":
		indexes = []int{1}
	case "ThreadSQL", "ConversationSQL", "SignatureSourceEligible", "StoredOriginalSQL":
		indexes = []int{0, 1}
	case "CurrentCommitmentClause":
		indexes = []int{0, 1, 2}
	case "CurrentTaskClause":
		indexes = []int{0, 2}
	}
	alternatives := [][]string{{}}
	for _, index := range indexes {
		if index >= len(call.Args) {
			return []threadSQLValue{{unresolved: true}}
		}
		var next [][]string
		for _, previous := range alternatives {
			for _, value := range p.values(call.Args[index], bindings, reached, context, depth+1) {
				if value.unresolved {
					return []threadSQLValue{{unresolved: true}}
				}
				next = append(next, append(append([]string(nil), previous...), value.text))
				if len(next) > threadSQLMaxAlternatives {
					return []threadSQLValue{{unresolved: true}}
				}
			}
		}
		alternatives = next
	}
	var out []threadSQLValue
	for _, arguments := range alternatives {
		statement, err := threadProductionClause(name, arguments)
		if err != nil {
			return []threadSQLValue{{unresolved: true}}
		}
		out = append(out, threadSQLValue{text: statement})
	}
	return out
}

func threadProductionClause(name string, args []string) (string, error) {
	instant := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	count := 0
	argument := func(any) int { count++; return count }
	switch name {
	case "ActivityContentClause", "ActivityDiscoverClause":
		return "(" + auth.ActivityAvailableClause(args[0]) + ")", nil
	case "ActivityRepairPendingClause":
		ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "thread-census", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
		return auth.ActivityRepairPendingClause(ctx, args[0], argument)
	case "ActivityAvailableClause":
		return "(" + auth.ActivityAvailableClause(args[0]) + ")", nil
	case "AudienceWorkspaceOnly":
		return auth.AudienceWorkspaceOnly(args[0]), nil
	case "ThreadSQL":
		return correspondence.ThreadSQL(args[0], args[1]), nil
	case "ConversationSQL":
		return correspondence.ConversationSQL(args[0], args[1]), nil
	case "SubjectMailSQL":
		return correspondence.SubjectMailSQL(args[0]), nil
	case "SignatureSourceEligible":
		return contacts.SignatureSourceEligible(args[0], args[1]), nil
	case "ClassifyBacklogPredicate":
		return activities.ClassifyBacklogPredicate(args[0]), nil
	case "CitedActivityTime":
		return storekit.CitedActivityTime(args[0]), nil
	case "CitedActivityStamp":
		return storekit.CitedActivityStamp(args[0]), nil
	case "CitedActivityPrecision":
		return storekit.CitedActivityPrecision(args[0]), nil
	case "CurrentCommitmentClause":
		return provenance.CurrentCommitmentClause(args[0], args[1], args[2], instant, argument), nil
	case "CurrentTaskClause":
		return storekit.CurrentTaskClause(args[0], instant, args[1], argument), nil
	case "ConnectorSeatSQL":
		return auth.ConnectorSeatSQL(args[0]), nil
	case "StoredOriginalSQL":
		return storekit.StoredOriginalSQL(args[0], args[1]), nil
	case "HeldThroughRecordLinks":
		return retentionscope.HeldThroughRecordLinks(args[0]), nil
	case "CurrentSourceMailClause":
		return storekit.CurrentSourceMailClause(args[0], instant, argument), nil
	}
	return "?", nil
}
