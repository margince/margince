// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"unicode/utf8"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maxBulkTaskSubject is BulkTask.subject's maxLength in the contract, in characters.
const maxBulkTaskSubject = 500

// The parameters a bulk change can name beside its verb and items.
const (
	bulkParamOwner = "owner_id"
	bulkParamList  = "list_id"
	bulkParamNote  = "note"
	bulkParamTag   = "tag_id"
	bulkParamTask  = "task"
)

// bulkVerbNeeds is, per verb, the one parameter it requires and what that
// parameter is for. Every other parameter is refused, except a list verb's note.
var bulkVerbNeeds = map[crmcontracts.BulkVerb]struct{ param, why string }{
	crmcontracts.BulkVerbReassignOwner:  {bulkParamOwner, "the colleague to hand the records to"},
	crmcontracts.BulkVerbArchive:        {},
	crmcontracts.BulkVerbAddToList:      {bulkParamList, "the Shortlist to change"},
	crmcontracts.BulkVerbRemoveFromList: {bulkParamList, "the Shortlist to change"},
	crmcontracts.BulkVerbAddTag:         {bulkParamTag, "the tag to put on"},
	crmcontracts.BulkVerbRemoveTag:      {bulkParamTag, "the tag to take off"},
	crmcontracts.BulkVerbCreateTask:     {bulkParamTask, "the task to file under each record"},
	crmcontracts.BulkVerbComplete:       {},
}

// validateBulkChange refuses what the contract refuses, for the tool door
// whose arguments no generated decoder has checked, and one thing it cannot
// say: a record named twice.
func validateBulkChange(change bulkChange) error {
	need, known := bulkVerbNeeds[change.verb]
	if !known {
		return httperr.Validation("verb", "unknown_verb", fmt.Sprintf(
			"verb %q is none of reassign_owner, archive, add_to_list, remove_from_list, add_tag, remove_tag, create_task or complete", change.verb))
	}
	if err := validateBulkParams(change, need.param, need.why); err != nil {
		return err
	}
	if len(change.items) == 0 || len(change.items) > bulkMaxItems {
		return httperr.Validation("items", "out_of_range",
			fmt.Sprintf("items names between 1 and %d records; this change names %d", bulkMaxItems, len(change.items)))
	}
	seen := make(map[openapi_types.UUID]bool, len(change.items))
	for _, item := range change.items {
		if seen[item.Id] {
			return httperr.Validation("items", "duplicate_id", fmt.Sprintf("record %s is named more than once", item.Id))
		}
		seen[item.Id] = true
	}
	return nil
}

// validateBulkParams requires the one parameter the verb needs and refuses
// every other it was handed.
func validateBulkParams(change bulkChange, need, why string) error {
	named := []struct {
		param string
		set   bool
	}{
		{bulkParamOwner, change.ownerID != nil},
		{bulkParamList, change.listID != nil},
		{bulkParamNote, change.note != nil},
		{bulkParamTag, change.tagID != nil},
		{bulkParamTask, change.task != nil},
	}
	for _, p := range named {
		switch {
		case p.param == need && !p.set:
			return httperr.Validation(need, "required", fmt.Sprintf("%s needs %s, %s", change.verb, need, why))
		case p.set && p.param != need && (p.param != bulkParamNote || !isListVerb(change.verb)):
			return httperr.Validation(p.param, "not_allowed", fmt.Sprintf("%s takes no %s", change.verb, p.param))
		}
	}
	if change.task != nil && !values.HasVisibleText(change.task.Subject) {
		return httperr.Validation("task.subject", "required", "create_task needs a subject: what has to be done")
	}
	if change.task != nil && utf8.RuneCountInString(change.task.Subject) > maxBulkTaskSubject {
		return httperr.Validation("task.subject", "too_long",
			fmt.Sprintf("a task subject holds at most %d characters", maxBulkTaskSubject))
	}
	return nil
}

// admitKind refuses a verb the record type does not take: archive for a type
// with no archive, and complete for any type but a Worklist item, which takes
// complete alone.
func admitKind(target bulkTarget, change bulkChange) error {
	_, archives := target.(bulkArchiver)
	_, worklist := target.(worklistBulkTarget)
	switch {
	case change.verb == crmcontracts.BulkVerbArchive && !archives:
		return httperr.Validation("verb", "verb_not_for_record_type",
			fmt.Sprintf("a %s has no archive; archive is none of its verbs", change.recordType))
	case worklist && change.verb != crmcontracts.BulkVerbComplete:
		return httperr.Validation("verb", "verb_not_for_record_type",
			fmt.Sprintf("a %s takes complete alone; %s is none of its verbs", change.recordType, change.verb))
	case !worklist && change.verb == crmcontracts.BulkVerbComplete:
		return httperr.Validation("verb", "verb_not_for_record_type",
			fmt.Sprintf("complete marks a worklist_item done; a %s takes other verbs", change.recordType))
	}
	return nil
}
