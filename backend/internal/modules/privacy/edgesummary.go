// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What a history line says when the row changed a LINK rather than a field.
//
// "X updated the record" is wrong twice about an edge: it was not the record,
// and it does not say which link. So the phrase names the OTHER end — the record
// a reader of this page cannot see from here — and the verb is keyed on the
// (action, entity_type) PAIR rather than the action alone, because `create` is
// already spelled "created" for every record in the product.

import (
	"fmt"
	"sort"
	"strings"
)

// auditVerbKey is one audit row's (action, entity_type) pair.
type auditVerbKey struct{ action, entityType string }

// edgePhrase is one rendered line, in the two parts a caller supplies and the
// one this file derives.
type edgePhrase struct {
	// template takes the actor's subject phrase, the other end's name, and —
	// where detail is set — whatever detail names.
	template string
	// detail says WHAT about the link the verb concerns: the role the link was
	// made with, or the fields that moved. Nil for a verb whose phrase needs
	// neither, and an unlink needs neither: the link is gone, so nothing about
	// it is the news.
	detail func(edge edgeSubject, after map[string]any) string
}

// edgePhrases render an edge audit row, keyed on (action, entity_type).
//
// The pair is what the key has to be. recordHistoryVerbs keys on the action
// alone, and the retention pair `release`/`expire` is the standing proof that
// this is not enough: both verbs already carry unrelated phrases from other
// features, so a phrase reached by action alone renders one feature's event as
// another's. That map's shape is unchanged and still has the problem; this one
// does not acquire it.
//
// Only the three verbs an edge write emits (contacts/relationshipimage.go). An
// action absent here falls back to the record phrasing, which is honest if
// clumsy — the alternative is a line that claims to know what happened.
var edgePhrases = map[auditVerbKey]edgePhrase{
	{action: actionCreate, entityType: EdgeEntityType}: {
		template: "%s linked %s as %s", detail: edgeRoleOrKind,
	},
	{action: actionUpdate, entityType: EdgeEntityType}: {
		template: "%s changed %s's %s", detail: edgeChangedFields,
	},
	{action: actionArchive, entityType: EdgeEntityType}: {
		template: "%s unlinked %s",
	},
}

// edgeSummary renders one edge row's line, or reports that this verb has no edge
// phrasing and the record sentence stands.
//
// subject is the actor phrase the record summary composes — "Uma", "Uma, via
// Claude", "System" — so attribution reads identically on an edge line and on
// every other line of the same page.
func edgeSummary(subject, action string, edge edgeSubject, after map[string]any) (string, bool) {
	phrase, known := edgePhrases[auditVerbKey{action: action, entityType: EdgeEntityType}]
	if !known {
		return "", false
	}
	args := []any{subject, edgeOtherName(edge)}
	if phrase.detail != nil {
		args = append(args, phrase.detail(edge, after))
	}
	return fmt.Sprintf(phrase.template, args...), true
}

// edgeOtherName is the other end as a reader knows it. The id is the fallback,
// never an invented name and never a blank: a record whose name column is empty
// is still a record a reader can go and look at.
func edgeOtherName(edge edgeSubject) string {
	if edge.otherLabel != nil && *edge.otherLabel != "" {
		return *edge.otherLabel
	}
	return edge.otherID.String()
}

// edgeRoleOrKind says what the link was made AS. The role when the link carries
// one ("linked Acme as cto"), the kind when it does not — a partner edge has no
// role, and "linked Acme as co_sell_with" still tells a reader which of the
// seven kinds of link this was.
func edgeRoleOrKind(edge edgeSubject, after map[string]any) string {
	if role := edgeRoleOf(after); role != "" {
		return role
	}
	return edge.kind
}

// edgeChangedFields names the fields a patch moved, from the masked after image
// the edge write already narrowed to what changed.
func edgeChangedFields(_ edgeSubject, after map[string]any) string {
	fields := edgeChangedFieldNames(after)
	// A patch that moved nothing the image reports still happened.
	if len(fields) == 0 {
		return "link"
	}
	return strings.Join(fields, ", ")
}

// edgeChangedFieldNames is the sorted field list the sentence above and the
// wire's changed_fields both read, so the two cannot name different fields.
func edgeChangedFieldNames(after map[string]any) []string {
	fields := make([]string, 0, len(after))
	for field := range after {
		if field != relationshipKindImageKey {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

// edgeRoleOf reads the role from a MASKED image. Empty means the image does not
// carry one, which a client reads as "not said" rather than "no role".
func edgeRoleOf(image map[string]any) string {
	role, _ := image[relationshipRoleImageKey].(string)
	return role
}

// The two keys an edge image always carries. Spelled here rather than imported:
// modules do not import siblings, so these are one end of a wire whose other end
// is the edge writer's image. The agreement is held end to end by
// TestEdgeHistoryStillShowsAnUnlinkedEdge, which reads a real patch back and
// expects the line to name the role — a rename on either side reds it.
const (
	relationshipKindImageKey = "kind"
	relationshipRoleImageKey = "role"
)
