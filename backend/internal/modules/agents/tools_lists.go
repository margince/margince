// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// read_lists and change_lists: Live Lists and Shortlists, read and changed as
// the user behind the agent's passport would.
//
// Two tools rather than one because one verb spends one scope: finding and
// reading lists needs read, changing them needs write, and an agent granted
// only read must still be able to read. Each carries its operations as modes,
// the shape bulk_update_records takes, so the pair costs two places in the
// tool list rather than twelve.
//
// Both reach the same collections store the /v1/lists routes answer from,
// through a seam compose implements, so a count, a reason or a refusal is the
// same on either door.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Lists is the list surface. Every method answers the contract's own shape,
// already encoded.
type Lists interface {
	ReadLists(ctx context.Context, q ListRead) (json.RawMessage, error)
	ChangeLists(ctx context.Context, c ListChange) (json.RawMessage, error)
}

// ListRead is one read_lists call.
type ListRead struct {
	Mode       string         `json:"mode"`
	ListID     *ids.UUID      `json:"list_id"`
	RecordID   *ids.UUID      `json:"record_id"`
	EntityType string         `json:"entity_type"`
	Query      string         `json:"query"`
	Definition map[string]any `json:"definition"`
	Limit      int            `json:"limit"`
	Cursor     string         `json:"cursor"`
}

// ListChange is one change_lists call.
type ListChange struct {
	Mode       string         `json:"mode"`
	ListID     *ids.UUID      `json:"list_id"`
	Name       *string        `json:"name"`
	Purpose    *string        `json:"purpose"`
	EntityType string         `json:"entity_type"`
	ListType   string         `json:"list_type"`
	Definition map[string]any `json:"definition"`
	Sharing    *string        `json:"sharing"`
	TeamID     *ids.UUID      `json:"team_id"`
	StewardID  *ids.UUID      `json:"steward_id"`
	Version    *int64         `json:"version"`
	RecordID   *ids.UUID      `json:"record_id"`
	Note       *string        `json:"note"`
}

// The read and change modes.
const (
	ListModeFind    = "find"
	ListModeGet     = "get"
	ListModeMembers = "members"
	ListModeWhy     = "why"
	ListModeHistory = "history"
	ListModePreview = "preview"

	ListModeCreate  = "create"
	ListModeUpdate  = "update"
	ListModeArchive = "archive"
	ListModeRestore = "restore"
	ListModeAdd     = "add_member"
	ListModeRemove  = "remove_member"
)

// ListsAnswer is what both tools answer: the mode, and the contract shape the
// matching /v1/lists route answers.
type ListsAnswer struct {
	Mode   string          `json:"mode"`
	Result json.RawMessage `json:"result"`
}

// RegisterListTools wires read_lists and change_lists over the list seam.
func RegisterListTools(r *Registry, lists Lists) {
	if lists == nil {
		return
	}
	r.Register(readLists{lists: lists})
	r.Register(changeLists{lists: lists})
}

type readLists struct{ lists Lists }

func (t readLists) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_lists", Title: "Find and read lists", Version: toolVersionV1,
		Description: readListsCopy.render(), Instead: readListsCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getList",
		InputSchema: schema(`{"type":"object","required":["mode"],"properties":{
			"mode":{"type":"string","enum":["find","get","members","why","history","preview"]},
			"list_id":{"type":"string","format":"uuid","description":"The list, for get, members, why and history"},
			"record_id":{"type":"string","format":"uuid","description":"For why: the record to explain"},
			"entity_type":{"type":"string","enum":["contact","company","deal","lead","project"],"description":"For find (optional) and preview (required)"},
			"query":{"type":"string","description":"For find: matches name or purpose"},
			"definition":{"type":"object","description":"For preview: a filter tree, as a Live List stores it","properties":{"and":{"type":"array","items":{"type":"object","properties":{"and":{"type":"array"},"or":{"type":"array"},"field":{"type":"string"},"op":{"type":"string"},"value":{}}}},"or":{"type":"array","items":{"type":"object","properties":{"and":{"type":"array"},"or":{"type":"array"},"field":{"type":"string"},"op":{"type":"string"},"value":{}}}},"field":{"type":"string"},"op":{"type":"string","enum":["eq","neq","gt","lt","gte","lte","in","contains","exists"]},"value":{"description":"A value, a list for in, true or false for exists, or {\"days_ago\": N} for a date"}}},
			"limit":{"type":"integer","minimum":1,"maximum":100},
			"cursor":{"type":"string","description":"For members and history: the next_cursor a page answered"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ListsAnswer](),
	}
}

func (t readLists) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args ListRead
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	switch args.Mode {
	case ListModeFind:
	case ListModeGet, ListModeMembers, ListModeHistory:
		if args.ListID == nil {
			return nil, &BadArgsError{Cause: fmt.Errorf("%s needs list_id", args.Mode)}
		}
	case ListModeWhy:
		if args.ListID == nil || args.RecordID == nil {
			return nil, &BadArgsError{Cause: fmt.Errorf("why needs list_id and record_id")}
		}
	case ListModePreview:
		if args.EntityType == "" || len(args.Definition) == 0 {
			return nil, &BadArgsError{Cause: fmt.Errorf("preview needs entity_type and definition")}
		}
	default:
		return nil, &BadArgsError{Cause: fmt.Errorf("mode %q is not a read_lists mode", args.Mode)}
	}
	return answer(args.Mode)(t.lists.ReadLists(ctx, args))
}

type changeLists struct{ lists Lists }

func (t changeLists) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "change_lists", Title: "Make and change lists", Version: toolVersionV1,
		Description: changeListsCopy.render(), Instead: changeListsCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "updateList",
		InputSchema: schema(`{"type":"object","required":["mode"],"properties":{
			"mode":{"type":"string","enum":["create","update","archive","restore","add_member","remove_member"]},
			"list_id":{"type":"string","format":"uuid","description":"The list, for every mode but create"},
			"name":{"type":"string"},
			"purpose":{"type":"string"},
			"entity_type":{"type":"string","enum":["contact","company","deal","lead","project"],"description":"For create, and the record type of record_id"},
			"list_type":{"type":"string","enum":["static","dynamic"],"description":"For create: static is a Shortlist, dynamic a Live List"},
			"definition":{"type":"object","description":"A Live List's filter tree","properties":{"and":{"type":"array","items":{"type":"object","properties":{"and":{"type":"array"},"or":{"type":"array"},"field":{"type":"string"},"op":{"type":"string"},"value":{}}}},"or":{"type":"array","items":{"type":"object","properties":{"and":{"type":"array"},"or":{"type":"array"},"field":{"type":"string"},"op":{"type":"string"},"value":{}}}},"field":{"type":"string"},"op":{"type":"string","enum":["eq","neq","gt","lt","gte","lte","in","contains","exists"]},"value":{"description":"A value, a list for in, true or false for exists, or {\"days_ago\": N} for a date"}}},
			"sharing":{"type":"string","enum":["private","team","workspace"]},
			"team_id":{"type":"string","format":"uuid"},
			"steward_id":{"type":"string","format":"uuid"},
			"version":{"type":"integer","description":"For update: the version you read"},
			"record_id":{"type":"string","format":"uuid","description":"For add_member and remove_member"},
			"note":{"type":"string","maxLength":500,"description":"For add_member and remove_member: why"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ListsAnswer](),
	}
}

func (t changeLists) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args ListChange
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := checkListChange(args); err != nil {
		return nil, err
	}
	return answer(args.Mode)(t.lists.ChangeLists(ctx, args))
}

// checkListChange refuses a change missing what its mode needs.
func checkListChange(args ListChange) error {
	switch args.Mode {
	case ListModeCreate:
		if args.Name == nil || args.EntityType == "" {
			return &BadArgsError{Cause: fmt.Errorf("create needs name and entity_type")}
		}
		return nil
	case ListModeUpdate:
		if args.ListID == nil || args.Version == nil {
			return &BadArgsError{Cause: fmt.Errorf("update needs list_id and the version you read")}
		}
		return nil
	case ListModeArchive, ListModeRestore:
		if args.ListID == nil {
			return &BadArgsError{Cause: fmt.Errorf("%s needs list_id", args.Mode)}
		}
		return nil
	case ListModeAdd, ListModeRemove:
		if args.ListID == nil || args.RecordID == nil || args.EntityType == "" {
			return &BadArgsError{Cause: fmt.Errorf("%s needs list_id, entity_type and record_id", args.Mode)}
		}
		return nil
	default:
		return &BadArgsError{Cause: fmt.Errorf("mode %q is not a change_lists mode", args.Mode)}
	}
}

// answer wraps a seam answer in the tool's envelope.
func answer(mode string) func(json.RawMessage, error) (json.RawMessage, error) {
	return func(result json.RawMessage, err error) (json.RawMessage, error) {
		if err != nil {
			return nil, err
		}
		return json.Marshal(ListsAnswer{Mode: mode, Result: result})
	}
}
