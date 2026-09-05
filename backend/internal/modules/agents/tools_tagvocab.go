// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The vocabulary verbs: coining a word and editing it.
//
// They sit apart from apply_tag and remove_tag because they act on the WORD
// rather than on a record's use of it, and because they answer to a different
// authority: the store takes `tag.create` / `tag.update` / `tag.delete`, which
// the seeded roles give Admin and Ops alone. A rep's passport reaches apply and
// remove and is refused here — by the store, not by the tool, so the rule is
// the same one the UI obeys rather than a second copy of it.
//
// NEITHER STAMPS EVIDENCE, and that has a consequence worth knowing: the
// surface adds `idempotency_key` to every tool, and a replay re-checks
// authority by re-reading the records the recorded answer names. An answer
// naming none is refused, so retrying one of these with the same key answers
// not-found rather than the original result. A tag is vocabulary rather than a
// record — the same reason list_tags gives for stamping nothing — so there is
// no record here to name, and inventing an entity type for one would put a
// word into a list that means "records this answer rests on". Tracked as
// fast-track debt; the fix belongs to how the surface replays a record-less
// write, not to these two tools.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// --- create_tag (🟢 write, gated on tag.create) ---

type createTag struct{ tags Tags }

func (t createTag) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "create_tag", Title: "Create a tag", Version: toolVersionV1,
		Description:   createTagCopy.render(),
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		// A tag is a WORD, not a row with a scope, so this answer names no
		// record and the replay has nothing to re-read. The grant is what the
		// handler checked; it is what the replay checks. Without it a retry
		// after a timeout was told the call never happened, and an agent could
		// coin a second word or re-issue an edit it had already made.
		ReplayGrant: &mcp.ReplayGrant{Object: tagRecordType, Action: principal.ActionCreate},
		OpenAPIOp:   "createTag",
		// No `description`, though CreateTagRequest declares one and update_tag
		// takes one. The REST handler decodes it and never passes it to the
		// store (collections/handlers.go CreateTag), so the field is dropped in
		// silence today — offering it here would advertise a write that does
		// not happen. A tool schema is what a model trusts about the effect.
		// Filed as debt rather than fixed here: making the HTTP write honour it
		// is a change to that door, not to this one.
		InputSchema: schema(`{"type":"object","required":["name"],"properties":{
			"name":{"type":"string","minLength":1,"maxLength":64},
			"color":{"type":"string","enum":["teal","amber","rose","slate"]}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[Tag](),
	}
}

func (t createTag) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Name  string  `json:"name"`
		Color *string `json:"color"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	detail, err := t.tags.CreateTag(ctx, args.Name, args.Color)
	if err != nil {
		return nil, err
	}
	return json.Marshal(detail)
}

// --- update_tag (🟢 write) ---

type updateTag struct{ tags Tags }

func (t updateTag) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "update_tag", Title: "Rename or recolour a tag", Version: toolVersionV1,
		Description:   updateTagCopy.render(),
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		// A tag is a WORD, not a row with a scope, so this answer names no
		// record and the replay has nothing to re-read. The grant is what the
		// handler checked; it is what the replay checks. Without it a retry
		// after a timeout was told the call never happened, and an agent could
		// coin a second word or re-issue an edit it had already made.
		ReplayGrant: &mcp.ReplayGrant{Object: tagRecordType, Action: principal.ActionUpdate},
		OpenAPIOp:   "updateTag",
		// The REST body's own vocabulary, mirrored: an omitted field is left
		// alone and `color: "none"` clears the colour. Spelling clearing as a
		// VALUE is the contract's decision, and for the reason it gives — a
		// decoded absent field and a decoded null are the same thing, so a
		// transport that promised they differed would promise what no server
		// can honour.
		InputSchema: schema(`{"type":"object","required":["tag_id"],"properties":{
			"tag_id":{"type":"string","format":"uuid"},
			"name":{"type":"string","minLength":1,"maxLength":64},
			"color":{"type":"string","enum":["teal","amber","rose","slate","none"]},
			"description":{"type":"string"}},"additionalProperties":false}`),
		OutputSchema: schemaFor[Tag](),
	}
}

func (t updateTag) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		TagID       ids.UUID `json:"tag_id"`
		Name        *string  `json:"name"`
		Color       *string  `json:"color"`
		Description *string  `json:"description"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	// Absent leaves the field alone; a VALUE clears it — "none" for the colour,
	// "" for the text — which is the REST body's own spelling and is why these
	// are pointers rather than values. The store takes a pointer-to-pointer for
	// the two clearable fields: non-nil outer means "the caller named this",
	// nil inner means "and asked for it to be empty".
	edit := TagEdit{Name: args.Name}
	if args.Color != nil {
		cleared := *args.Color == clearColor
		edit.Color = new(*string)
		if !cleared {
			*edit.Color = args.Color
		}
	}
	if args.Description != nil {
		edit.Description = new(*string)
		if *args.Description != "" {
			*edit.Description = args.Description
		}
	}
	detail, err := t.tags.UpdateTag(ctx, args.TagID, edit)
	if err != nil {
		return nil, err
	}
	return json.Marshal(detail)
}

// clearColor is the value that REMOVES a tag's colour, spelled the way
// UpdateTagRequest spells it.
//
// Named rather than repeated because the handler and the schema above must
// agree on the word: a schema offering a value the handler does not recognise
// is a clear the caller asks for and never gets.
// TestUpdateTagOffersTheClearingValue reads the enum out of the rendered spec
// and fails when this constant is not in it, and the compose lane's
// TestEveryToolEnumMatchesTheContractItMirrors holds the same enum against
// UpdateTagRequest's own.
const clearColor = "none"

// --- merge_tags (🟡 confirm-first, gated on tag.update) ---

// mergeTags is the one vocabulary verb that reaches a human first. Coining and
// editing a word are auto-execute because the same seat can undo either from
// the vocabulary screen; a merge rewrites every record carrying the source and
// releases the source's name, and nothing records where those taggings went.
type mergeTags struct{ tags Tags }

func (t mergeTags) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "merge_tags", Title: "Fold one tag into another", Version: toolVersionV1,
		Description:   mergeTagsCopy.render(),
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired,
		OpenAPIOp: "mergeTags",
		InputSchema: schema(`{"type":"object","required":["tag_id","into_tag_id"],"properties":{
			"tag_id":{"type":"string","format":"uuid","description":"The word to retire"},
			"into_tag_id":{"type":"string","format":"uuid","description":"The word that survives"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[TagMergeResult](),
	}
}

type mergeTagsArgs struct {
	TagID     ids.UUID `json:"tag_id"`
	IntoTagID ids.UUID `json:"into_tag_id"`
}

// StageInfo decodes this door's arguments into the merge command and delegates:
// the refusals and the staged subject live in the resolver
// (commandtagvocab.go), where the REST door reaches the same ones for the same
// operation.
func (t mergeTags) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args mergeTagsArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewMergeTagsCall(t.tags, MergeTagsCommand{
		SourceID: args.TagID,
		TargetID: args.IntoTagID,
	}))
}

func (t mergeTags) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args mergeTagsArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	result, err := t.tags.MergeTags(ctx, args.TagID, args.IntoTagID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
