// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEachVerbNamesItsOneParameterAndNoOther(t *testing.T) {
	tag, owner, list := ids.NewV7(), ids.NewV7(), ids.NewV7()
	note := "why"
	task := &crmcontracts.BulkTask{Subject: "Call back"}
	blank := &crmcontracts.BulkTask{Subject: "  "}
	one := []crmcontracts.BulkItem{{Id: openapi_types.UUID(ids.NewV7()), Version: 1}}
	for name, tc := range map[string]struct {
		change bulkChange
		field  string
	}{
		"add_tag without a tag":        {bulkChange{verb: crmcontracts.BulkVerbAddTag, items: one}, "tag_id"},
		"remove_tag with an owner":     {bulkChange{verb: crmcontracts.BulkVerbRemoveTag, items: one, tagID: &tag, ownerID: &owner}, "owner_id"},
		"add_tag with a note":          {bulkChange{verb: crmcontracts.BulkVerbAddTag, items: one, tagID: &tag, note: &note}, "note"},
		"create_task without a task":   {bulkChange{verb: crmcontracts.BulkVerbCreateTask, items: one}, "task"},
		"create_task with a blank one": {bulkChange{verb: crmcontracts.BulkVerbCreateTask, items: one, task: blank}, "task.subject"},
		"archive with a Shortlist":     {bulkChange{verb: crmcontracts.BulkVerbArchive, items: one, listID: &list}, "list_id"},
		"reassign with a task":         {bulkChange{verb: crmcontracts.BulkVerbReassignOwner, items: one, ownerID: &owner, task: task}, "task"},
		"a list verb with its note":    {bulkChange{verb: crmcontracts.BulkVerbAddToList, items: one, listID: &list, note: &note}, ""},
		"a well-formed tag change":     {bulkChange{verb: crmcontracts.BulkVerbAddTag, items: one, tagID: &tag}, ""},
		"a well-formed task change":    {bulkChange{verb: crmcontracts.BulkVerbCreateTask, items: one, task: task}, ""},
		"complete with a tag":          {bulkChange{verb: crmcontracts.BulkVerbComplete, items: one, tagID: &tag}, "tag_id"},
		"a well-formed completion":     {bulkChange{verb: crmcontracts.BulkVerbComplete, items: one}, ""},
	} {
		err := validateBulkChange(tc.change)
		var refused *httperr.DetailedError
		switch {
		case tc.field == "" && err != nil:
			t.Errorf("%s: refused with %v", name, err)
		case tc.field != "" && (!errors.As(err, &refused) || refused.Fields[0].Field != tc.field):
			t.Errorf("%s: %v, want a refusal naming %s", name, err, tc.field)
		}
	}
}

// A Worklist item takes complete and nothing else, and no record type takes
// complete: a verb named for the wrong kind is refused before any row is tried.
func TestCompleteIsAWorklistItemsVerbAlone(t *testing.T) {
	worklist, contact := worklistBulkTarget{}, contactBulkTarget{}
	for name, tc := range map[string]struct {
		target  bulkTarget
		verb    crmcontracts.BulkVerb
		refused bool
	}{
		"complete on a worklist item": {worklist, crmcontracts.BulkVerbComplete, false},
		"archive on a worklist item":  {worklist, crmcontracts.BulkVerbArchive, true},
		"reassign on a worklist item": {worklist, crmcontracts.BulkVerbReassignOwner, true},
		"complete on a contact":       {contact, crmcontracts.BulkVerbComplete, true},
		"archive on a contact":        {contact, crmcontracts.BulkVerbArchive, false},
	} {
		err := admitKind(tc.target, bulkChange{verb: tc.verb})
		var refused *httperr.DetailedError
		switch {
		case !tc.refused && err != nil:
			t.Errorf("%s: refused with %v", name, err)
		case tc.refused && (!errors.As(err, &refused) || refused.Fields[0].Code != "verb_not_for_record_type"):
			t.Errorf("%s: %v, want verb_not_for_record_type", name, err)
		}
	}
}
