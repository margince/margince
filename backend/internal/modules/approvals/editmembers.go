// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
)

// editableMembers names, per kind, the only top-level members of a staged
// change that a human's edit may change. Every other member must stay as
// staged. A kind not listed keeps the general rule in editscope.go: content is
// editable and references are pinned.
//
// A LinkedIn match card's strings are the export's own spelling and the
// contact's values when it was staged. The apply compares them with the rows,
// so an edit to one of them would turn that check into whatever the editor
// typed. The two boxes are the only choice the card offers.
var editableMembers = map[string][]string{
	kindLinkedInMatch: {"use_as_title", "use_as_employer"},
	// A handover is answered yes or no, and its effect trusts every member.
	approvalsubject.KindOwnershipTransfer: {},
}

// assertEditableMembers refuses an edit that changes a member its kind does not
// list, or that adds or drops a listed one. A box a card was staged without is
// one the apply was never meant to write. So a card staged before the boxes
// existed takes no edit at all. A listed member stays a boolean.
func assertEditableMembers(kind string, original, edited json.RawMessage) error {
	allowed, listed := editableMembers[kind]
	if !listed {
		return nil
	}
	before, err := jsonObjectMembers(original)
	if err != nil {
		return &InvalidEditError{Cause: fmt.Errorf("the staged change is not a JSON object: %w", err)}
	}
	after, err := jsonObjectMembers(edited)
	if err != nil {
		return &InvalidEditError{Cause: fmt.Errorf("the edit is not a JSON object: %w", err)}
	}
	free := map[string]bool{}
	for _, member := range allowed {
		free[member] = true
	}
	var fixed []string
	for member := range memberUnion(before, after) {
		was, had := before[member]
		now, has := after[member]
		if free[member] && had && has {
			if !isJSONBool(now) {
				return &InvalidEditError{Cause: fmt.Errorf("%s must be true or false", member)}
			}
			continue
		}
		same, err := sameJSONValue(was, now)
		if err != nil {
			return &InvalidEditError{Cause: err}
		}
		if had != has || !same {
			fixed = append(fixed, member)
		}
	}
	if len(fixed) == 0 {
		return nil
	}
	sort.Strings(fixed)
	if len(allowed) == 0 {
		return &InvalidEditError{Cause: fmt.Errorf("a %s takes no edit; %s must stay as staged",
			kind, strings.Join(fixed, ", "))}
	}
	return &InvalidEditError{Cause: fmt.Errorf("a %s edit may change only %s; %s must stay as staged",
		kind, strings.Join(allowed, " and "), strings.Join(fixed, ", "))}
}

func memberUnion(before, after map[string]json.RawMessage) map[string]struct{} {
	members := make(map[string]struct{}, len(before)+len(after))
	for member := range before {
		members[member] = struct{}{}
	}
	for member := range after {
		members[member] = struct{}{}
	}
	return members
}

func isJSONBool(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false"))
}
