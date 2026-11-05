// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The go-ahead for a bulk change, asked in the chat when the host can show a
// form. Without one the agent asks in words, as the tool's description says.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const bulkConfirmKey = "confirm_bulk_change"

const bulkConfirmSchema = `{"type":"object","required":["confirm"],
	"properties":{"confirm":{"type":"boolean","title":"Go ahead","default":false}}}`

// confirmBulkInChat asks the user to confirm before an execute or an undo
// runs. It answers nil when the change may go ahead, including when this
// caller cannot be asked.
func confirmBulkInChat(ctx context.Context, args bulkUpdateRecordsArgs) error {
	reply, err := replyInChat(ctx, chatQuestion{
		Key: bulkConfirmKey, Message: bulkConfirmMessage(args),
		Schema: json.RawMessage(bulkConfirmSchema),
	})
	if err != nil || !reply.Available {
		return err
	}
	var yes bool
	if reply.Accepted && json.Unmarshal(reply.Content["confirm"], &yes) == nil && yes {
		return nil
	}
	return errDeclinedInChat
}

// bulkConfirmMessage says what will change, in the words the form shows.
func bulkConfirmMessage(args bulkUpdateRecordsArgs) string {
	if args.Mode == bulkModeUndo {
		return "Undo the bulk change? Records changed since then are left as they are."
	}
	noun := strings.ReplaceAll(args.RecordType, "_", " ")
	if noun == "worklist item" {
		noun = "Worklist task"
	}
	count := len(args.Items)
	if count != 1 {
		noun = pluralNoun(noun)
	}
	return fmt.Sprintf("%s %d %s? You can undo it afterwards.", bulkVerbWords(args.Verb), count, noun)
}

// The two verbs that share their spelling with a tool's name.
const (
	verbRemoveTag  = "remove_tag"
	verbCreateTask = "create_task"
)

func bulkVerbWords(verb string) string {
	switch verb {
	case "reassign_owner":
		return "Give a new owner to"
	case "archive":
		return "Archive"
	case "add_to_list":
		return "Add to the list"
	case "remove_from_list":
		return "Take off the list"
	case "add_tag":
		return "Tag"
	case verbRemoveTag:
		return "Remove the tag from"
	case verbCreateTask:
		return "File a task under"
	case "complete":
		return "Mark done"
	default:
		return "Change"
	}
}

// pluralNoun spells more than one of a record kind: company becomes companies.
func pluralNoun(noun string) string {
	if stem, ok := strings.CutSuffix(noun, "y"); ok {
		return stem + "ies"
	}
	return noun + "s"
}
