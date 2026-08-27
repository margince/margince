// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// One projection of a record's audit spine: the columns, the joins that resolve
// the names in them, and the decode that reads them back.
//
// All three live together because they are one thing. The scanner is shared by
// both readers, so a column added to a hand-written list in one query and not
// the other breaks the other's scan on arity — the column list has to be as
// shared as the decode is, or the pair only agrees on the day it was written.

import "fmt"

// recordAuditColumns is the projection both readers of a record's audit spine
// select. One spelling, because the SCANNER below is shared: a column added to
// one query and not the other breaks the other's scan on arity, and the two
// would otherwise agree only on the day somebody wrote them.
const recordAuditColumns = `
		a.id, a.actor_type, a.actor_id, a.on_behalf_of, a.action, a.occurred_at,
		a.authorization_rule, a.before, a.after, a.passport_id,
		actor_user.display_name AS actor_display_name,
		obo.display_name AS on_behalf_of_display_name, oc.client_name,
		` + reversalLinkColumn

// auditActorNameJoins resolves the two display names every audit read owes its
// reader: the human who acted, and the human whose authority a machine acted
// under. It is ONE spelling shared by the two audit read paths in this package —
// the per-record history and the workspace-wide compliance log in auditlog.go —
// because two surfaces resolving attribution differently is how a reader ends up
// trusting one and doubting the other.
//
// The audit row is aliased `a`; the caller supplies that alias and selects
// `actor_user.display_name, obo.display_name` in that order.
//
// The actor join builds the prefixed key FROM app_user ('human:' || id) rather
// than casting actor_id, so a non-uuid actor id (agent:*, connector:*, system)
// simply resolves to no name instead of raising a cast error. A LEFT JOIN both
// times, and matching app_user.id (a primary key) both times: a deactivated or
// deleted member still has audit rows, no name is honest where an invented one
// would not be, and neither join can duplicate or drop an audit row.
const auditActorNameJoins = `
		LEFT JOIN app_user actor_user
		  ON a.actor_type = 'human' AND a.actor_id = 'human:' || actor_user.id::text
		LEFT JOIN app_user obo ON obo.id = a.on_behalf_of`

// agentClientNameJoin resolves the NAME of the tool a delegated change was
// typed through — "Claude", "Cursor" — from the passport the row recorded.
//
// Three hops, all of them nullable, and each null means something different:
// a passport minted by hand in Settings has no oauth_grant_id (a person made
// it for themselves, and its own label is whatever they typed); a grant whose
// client row was deleted resolves nothing; and a row with no passport at all
// was not a delegated write. Every one of those falls back to the generic
// "via an agent", which is why this is a LEFT JOIN chain and not a filter.
//
// The join is on client_id ALONE. oauth_client used to be keyed
// (workspace_id, client_id) and this join said so; migration 1787109970 dropped
// the workspace column from every credential table, so client_id is the key
// now and naming the old one made every record-history read a 500.
const agentClientNameJoin = `
		LEFT JOIN passport p ON p.id = a.passport_id
		LEFT JOIN oauth_grant g ON g.id = p.oauth_grant_id
		LEFT JOIN oauth_client oc ON oc.client_id = g.client_id`

// auditRowScanner is what both readers of this projection have in common: a
// pgx.Row and a pgx.Rows both scan. The column list is recordAuditColumns and
// the decode is here, so the projection has ONE spelling end to end: a column
// added to the list is scanned by both readers or by neither, where two
// hand-written lists would drift the moment one of them grew.
type auditRowScanner interface {
	Scan(dest ...any) error
}

func scanRecordAuditRow(src auditRowScanner, r *recordAuditRow) error {
	return scanRecordAuditRowWith(src, r)
}

// scanRecordAuditRowWith decodes the shared projection and, after it, whatever
// columns the caller's own SELECT appended — the record-history window carries
// the edge subject there. Trailing rather than woven in, because the projection
// above is what the two readers share and only one of them widens its window: a
// column spliced into the middle would make the narrow reader's scan a
// positional guess.
func scanRecordAuditRowWith(src auditRowScanner, r *recordAuditRow, trailing ...any) error {
	var beforeJSON, afterJSON []byte
	var reversalLink *string
	dests := []any{
		&r.id, &r.actorType, &r.actorID, &r.onBehalfOf, &r.action, &r.occurredAt,
		&r.authorizationRule, &beforeJSON, &afterJSON, &r.passportID,
		&r.actorDisplayName, &r.onBehalfOfName, &r.agentClientName, &reversalLink,
	}
	if err := src.Scan(append(dests, trailing...)...); err != nil {
		return err
	}
	undid, err := reversalLinkFromColumn(reversalLink)
	if err != nil {
		return fmt.Errorf("audit row %s reversal link: %w", r.id, err)
	}
	r.undidAuditLogID = undid
	if err := unmarshalJSONBMap(beforeJSON, &r.before); err != nil {
		return fmt.Errorf("audit row %s before: %w", r.id, err)
	}
	if err := unmarshalJSONBMap(afterJSON, &r.after); err != nil {
		return fmt.Errorf("audit row %s after: %w", r.id, err)
	}
	return nil
}
