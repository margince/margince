// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The tag verbs (🟢 read + write, 🟡 archive): the workspace's own vocabulary
// for grouping records, which the tool surface could not touch at all.
//
// `tag` and `taggable` are real tables the web app uses, and no tool could
// list, create, apply or remove one — so the payoff of a capture flow ("add
// tag: K5 Conference 2026") could be described and never performed. The tag IS
// the outcome in those flows; without it the assistant reports work it did not
// do.
//
// Tags are a vocabulary, not records: workspace-shared, no owner, no row
// scope. That is why they get their own verbs rather than a `tag` record_type
// on create_record — the contract used to declare exactly that, naming tools
// which never served it.

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Tag is one word in the workspace's vocabulary.
type Tag struct {
	TagID ids.UUID `json:"tag_id"`
	Name  string   `json:"name"`
	Color string   `json:"color,omitempty"`
	// Archived says the word was retired. apply_tag will NOT reuse it — a
	// name whose only holder is archived is a conflict, not a match — so a
	// caller that cannot see this would read a refusal as a bug.
	Archived bool `json:"archived,omitempty"`
}

// TagDetail is one word with its weight: how many records of each advertised
// type carry it. The counts answer "what does retiring this cost", which is
// the question an agent asks before proposing a cleanup.
type TagDetail struct {
	Tag
	People    int `json:"people"`
	Companies int `json:"companies"`
	Deals     int `json:"deals"`
}

// Tags is the seam onto the collections module's tag paths.
type Tags interface {
	// ListTags answers the workspace's vocabulary. Read before applying:
	// apply_tag REFUSES a word the workspace does not hold, so a caller who
	// cannot see the existing ones asks for a near-duplicate.
	ListTags(ctx context.Context, includeArchived bool) (tags []Tag, truncated bool, err error)
	// GetTag answers one word with how much of the workspace carries it.
	GetTag(ctx context.Context, tagID ids.UUID) (TagDetail, error)
	// EnsureTaggable refuses a record the caller cannot tag, before a tag is
	// created for it. Same check ApplyTag makes at the end of its own
	// transaction; asked earlier so a failed apply leaves nothing behind.
	EnsureTaggable(ctx context.Context, entityType string, entityID ids.UUID) error
	// FindTag answers the id of the live workspace tag with this name, or
	// ok=false when there is none. Remove uses it: a name that names nothing
	// means the tagging is already absent.
	FindTag(ctx context.Context, name string) (ids.UUID, bool, error)
	// ResolveTag answers the id of an EXISTING workspace tag with this name
	// and refuses when there is none. It never creates: the vocabulary is
	// governed, so a tool that coined a word on a name it did not recognise
	// would hand every agent the authority only Admin and Ops hold.
	ResolveTag(ctx context.Context, name string) (ids.UUID, error)
	ApplyTag(ctx context.Context, tagID ids.UUID, entityType string, entityID ids.UUID) error
	RemoveTag(ctx context.Context, tagID ids.UUID, entityType string, entityID ids.UUID) error
	// TaggableTypes answers the record types a tagging may name — the store's
	// own vocabulary, so the apply/remove schemas advertise exactly what the
	// `taggable` table admits rather than a copy of that list kept here.
	TaggableTypes() []string
}

// RegisterTagTools joins the tag verbs to the surface; a nil seam registers
// nothing.
func RegisterTagTools(r *Registry, tags Tags) {
	if tags == nil {
		return
	}
	r.Register(listTags{tags: tags})
	r.Register(getTag{tags: tags})
	r.Register(applyTag{tags: tags})
	r.Register(removeTag{tags: tags})
}

// taggingSchema is apply's and remove's argument shape, spelled once: they name
// the same arguments, and two copies is two places for them to drift. The
// record_type enum comes from the seam rather than a literal here, so the
// schema advertises what the store admits by construction.
func taggingSchema(taggableTypes []string) string {
	quoted := make([]string, len(taggableTypes))
	for i, t := range taggableTypes {
		// strconv.Quote, not raw splicing: Go and JSON string quoting agree on
		// every character these plain type names carry, and quoting has no
		// error path where a marshal call would invite one.
		quoted[i] = strconv.Quote(t)
	}
	return `{"type":"object","required":["record_type","record_id"],"properties":{
	"tag_id":{"type":"string","format":"uuid"},
	"tag_name":{"type":"string","maxLength":64,"description":"Instead of tag_id: the name of a tag the workspace ALREADY has. An unknown name is refused, never created"},
	"record_type":{"type":"string","enum":[` + strings.Join(quoted, ",") + `]},
	"record_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`
}

// --- list_tags (🟢 read) ---

type listTags struct{ tags Tags }

func (t listTags) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "list_tags", Title: "List tags", Version: toolVersionV1,
		Description:   listTagsCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "listTags",
		InputSchema: schema(`{"type":"object","properties":{
			"include_archived":{"type":"boolean","description":"Also list retired words; they cannot be applied"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ListTagsResult](),
	}
}

func (t listTags) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		IncludeArchived bool `json:"include_archived"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	tags, truncated, err := t.tags.ListTags(ctx, args.IncludeArchived)
	if err != nil {
		return nil, err
	}
	// No noteEvidence, for the reason list_colleagues gives: a tag is
	// vocabulary, not a record the answer rests on. Stamping one would put
	// every word in the workspace into the evidence list of a call that only
	// asked what the words are.
	if tags == nil {
		tags = []Tag{}
	}
	return json.Marshal(ListTagsResult{Tags: tags, Truncated: truncated})
}

// --- get_tag (🟢 read) ---

type getTag struct{ tags Tags }

func (t getTag) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "get_tag", Title: "Get a tag", Version: toolVersionV1,
		Description:   getTagCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getTag",
		InputSchema: schema(`{"type":"object","required":["tag_id"],"properties":{
			"tag_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
		OutputSchema: schemaFor[TagDetail](),
	}
}

func (t getTag) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		TagID ids.UUID `json:"tag_id"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	detail, err := t.tags.GetTag(ctx, args.TagID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(detail)
}

// --- apply_tag / remove_tag (🟢 write) ---

type applyTag struct{ tags Tags }

func (t applyTag) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "apply_tag", Title: "Apply a tag to a record", Version: toolVersionV1,
		Description:   applyTagCopy.render(),
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp:    "applyTag",
		InputSchema:  schema(taggingSchema(t.tags.TaggableTypes())),
		OutputSchema: schemaFor[TagAppliedResult](),
	}
}

func (t applyTag) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	args, err := decodeTagging(in)
	if err != nil {
		return nil, err
	}
	// A name rather than an id is the capture flow's shape: "add tag: K5
	// Conference 2026" is one act to the person asking, and making them call a
	// create verb first only to pass its answer back is a second call that
	// exists for the surface's convenience rather than theirs. Reuse first —
	// an existing word wins over a new one, so tagging twice does not leave
	// two spellings of the same tag.
	if args.TagID.IsZero() {
		if args.TagName == "" {
			return nil, &BadArgsError{Cause: errors.New("give tag_id, or tag_name naming a tag the workspace already has")}
		}
		// The TARGET is authorized first, so a caller naming a record they
		// cannot reach is refused for that reason rather than learning which
		// tags exist from the resolution error.
		if err := t.tags.EnsureTaggable(ctx, args.RecordType, args.RecordID); err != nil {
			return nil, err
		}
		resolved, err := t.tags.ResolveTag(ctx, args.TagName)
		if err != nil {
			return nil, err
		}
		args.TagID = resolved
	}
	if err := t.tags.ApplyTag(ctx, args.TagID, args.RecordType, args.RecordID); err != nil {
		return nil, err
	}
	noteEvidence(ctx, datasource.EntityType(args.RecordType), args.RecordID)
	return json.Marshal(TagAppliedResult{Applied: true, TagID: args.TagID,
		RecordType: args.RecordType, RecordID: args.RecordID})
}

type removeTag struct{ tags Tags }

func (t removeTag) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "remove_tag", Title: "Take a tag off a record", Version: toolVersionV1,
		Description:   removeTagCopy.render(),
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp:    "removeTag",
		InputSchema:  schema(taggingSchema(t.tags.TaggableTypes())),
		OutputSchema: schemaFor[TagAppliedResult](),
	}
}

func (t removeTag) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	args, err := decodeTagging(in)
	if err != nil {
		return nil, err
	}
	// A name resolves here too — the schema offers it, and offering an
	// argument that is silently ignored is worse than not offering it: the
	// call passed validation with a zero tag id and answered a misleading 404.
	//
	// It LOOKS UP, never creates. Minting a tag in order to remove it is
	// nonsense, and a name that names nothing means the tagging is already
	// absent, which is the state the caller asked for.
	if args.TagID.IsZero() {
		if args.TagName == "" {
			return nil, &BadArgsError{Cause: errors.New("give tag_id or tag_name")}
		}
		found, ok, err := t.tags.FindTag(ctx, args.TagName)
		if err != nil {
			return nil, err
		}
		if !ok {
			return json.Marshal(TagAppliedResult{Applied: false,
				RecordType: args.RecordType, RecordID: args.RecordID})
		}
		args.TagID = found
	}
	if err := t.tags.RemoveTag(ctx, args.TagID, args.RecordType, args.RecordID); err != nil {
		return nil, err
	}
	noteEvidence(ctx, datasource.EntityType(args.RecordType), args.RecordID)
	return json.Marshal(TagAppliedResult{Applied: false, TagID: args.TagID,
		RecordType: args.RecordType, RecordID: args.RecordID})
}

type taggingArgs struct {
	TagID      ids.UUID `json:"tag_id"`
	TagName    string   `json:"tag_name"`
	RecordType string   `json:"record_type"`
	RecordID   ids.UUID `json:"record_id"`
}

// decodeTagging is shared so apply and remove cannot drift into reading the
// same three arguments differently.
func decodeTagging(in json.RawMessage) (taggingArgs, error) {
	var args taggingArgs
	if err := decodeArgs(in, &args); err != nil {
		return taggingArgs{}, err
	}
	return args, nil
}
