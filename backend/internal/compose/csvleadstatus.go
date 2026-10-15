// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A lead row's status cell, read the way the leads store takes it. The store
// refuses promoted and disqualified on create and edit, because those are
// reached through the promote and disqualify actions. A file is held to the
// same ladder: the vocabulary is the store's own open list.

import (
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// terminalLeadStatuses are the statuses a lead reaches through its own actions.
var terminalLeadStatuses = []string{"promoted", "disqualified"}

// checkLeadStatus types a mapped status cell as one of the open statuses, in
// any case. An empty cell says nothing: a new lead starts as new and a stored
// one keeps its status.
func checkLeadStatus(fields map[string]string, typed map[string]any) (map[string]any, *cellProblem) {
	raw, mapped := fields[leadTargetStatus]
	text := strings.ToLower(strings.TrimSpace(raw))
	if !mapped || text == "" {
		return typed, nil
	}
	allowed := contacts.OpenLeadStatuses()
	switch {
	case slices.Contains(allowed, text):
		if typed == nil {
			typed = map[string]any{}
		}
		typed[leadTargetStatus] = text
		return typed, nil
	case slices.Contains(terminalLeadStatuses, text):
		return nil, problemOf(leadTargetStatus, raw, migration.Why(migration.ReasonLeadStatusNotImportable,
			"value", strings.TrimSpace(raw), "allowed", strings.Join(allowed, ", ")))
	}
	return nil, problemOf(leadTargetStatus, raw, migration.Why(migration.ReasonLeadStatusUnknown,
		"value", strings.TrimSpace(raw), "allowed", strings.Join(allowed, ", ")))
}
