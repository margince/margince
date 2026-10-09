// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// attach_document and list_documents (🟢 write, 🟢 read): a file an agent was
// handed goes onto the record as a file, through the store a human upload uses.
// Neither tool is stageable: an approval would persist the file's bytes as args.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// DocumentUpload is one file an agent attaches, already decoded.
type DocumentUpload struct {
	RecordLink
	Filename    string
	ContentType string
	Content     []byte
}

// Documents is the seam onto the attachment store, implemented in compose.
type Documents interface {
	// Attach stores the file; the store holds the type allowlist and authority.
	Attach(ctx context.Context, upload DocumentUpload) (AttachedDocument, error)
	// List answers the record's documents as its Documents tab shows them.
	List(ctx context.Context, record RecordLink, cursor string, limit int) (DocumentPage, error)
	// MaxBytes is the operator's per-file upload limit; zero or less sets none.
	MaxBytes(ctx context.Context) int64
}

// attachFramingAllowance is the share of one MCP request kept for everything
// around the file: the JSON-RPC frame, the other arguments and the retry key.
const attachFramingAllowance = 64 << 10

// maxInlineFileBytes is the largest decoded file whose base64 fits one request.
const maxInlineFileBytes = (MaxMCPRequestBytes - attachFramingAllowance) / 4 * 3

// documentParentTypes are the records an agent may file a document against.
var documentParentTypes = []string{
	string(datasource.EntityCompany), string(datasource.EntityContact), string(datasource.EntityDeal),
	string(datasource.EntityLead), string(datasource.EntityProject),
}

// The arguments and code a refusal names, as the schema spells them.
const (
	argEntityType    = "entity_type"
	argContentBase64 = "content_base64"
	codeRequired     = "required"
)

// RegisterDocumentTools joins both tools to the surface; a nil seam registers
// nothing.
func RegisterDocumentTools(r *Registry, docs Documents) {
	if docs == nil {
		return
	}
	r.Register(attachDocument{docs: docs})
	r.Register(listDocuments{docs: docs})
}

func documentParentSchema() string {
	quoted := make([]string, len(documentParentTypes))
	for i, t := range documentParentTypes {
		quoted[i] = `"` + t + `"`
	}
	return `"entity_type":{"type":"string","enum":[` + strings.Join(quoted, ",") + `]},
	"entity_id":{"type":"string","format":"uuid"}`
}

func requireDocumentParent(entityType string) error {
	if slices.Contains(documentParentTypes, entityType) {
		return nil
	}
	return &BadArgsError{
		Cause:    fmt.Errorf("entity_type %q cannot hold a document", entityType),
		Field:    argEntityType,
		Guidance: "a document is filed on one of: " + strings.Join(documentParentTypes, ", "),
	}
}

// --- attach_document (🟢 write) ---

type attachDocument struct{ docs Documents }

func (t attachDocument) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "attach_document", Title: "Attach a file to a record", Version: toolVersionV1,
		Description:   attachDocumentCopy.render(),
		Instead:       attachDocumentCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp:    "uploadAttachment",
		MaxArgsBytes: MaxMCPRequestBytes,
		InputSchema: schema(`{"type":"object",
			"required":["entity_type","entity_id","filename","content_type","content_base64"],
			"properties":{` + documentParentSchema() + `,
			"filename":{"type":"string","description":"The file's name, with its extension"},
			"content_type":{"type":"string","description":"The file's media type, e.g. application/pdf"},
			"content_base64":{"type":"string","description":"The whole file, standard base64 with = padding"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[AttachedDocument](),
	}
}

type attachArgs struct {
	EntityType    string   `json:"entity_type"`
	EntityID      ids.UUID `json:"entity_id"`
	Filename      string   `json:"filename"`
	ContentType   string   `json:"content_type"`
	ContentBase64 string   `json:"content_base64"`
}

func (t attachDocument) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args attachArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := requireDocumentParent(args.EntityType); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Filename) == "" {
		return nil, &BadArgsError{
			Field: "filename", Code: codeRequired,
			Cause: errors.New("`filename` is blank; give the file's name with its extension"),
		}
	}
	if err := requireMediaType(args.ContentType); err != nil {
		return nil, err
	}
	content, err := decodeFile(args.ContentBase64, t.maxBytes(ctx))
	if err != nil {
		return nil, err
	}
	parent := RecordLink{EntityType: args.EntityType, EntityID: args.EntityID}
	stored, err := t.docs.Attach(ctx, DocumentUpload{
		RecordLink: parent, Filename: args.Filename, ContentType: args.ContentType, Content: content,
	})
	if err != nil {
		return nil, refusedKind(err)
	}
	// A replay is a receipt of a past write: it re-proves the parent, not the file,
	// which a human may since have removed and no replay reader resolves.
	noteEvidence(ctx, datasource.EntityType(parent.EntityType), parent.EntityID)
	return json.Marshal(stored)
}

// errFileKindRefused marks a file whose kind the store refuses. No argument
// fixes it: a model told to correct its call packed the file into a zip.
var errFileKindRefused = errors.New("this kind of file is not accepted")

// unsupportedFileTypeCode is the store's field code for a refused kind.
const unsupportedFileTypeCode = "unsupported_file_type"

func refusedKind(err error) error {
	if fault, ok := errors.AsType[apperrors.FieldFault](err); ok {
		if _, code, _ := fault.FieldFault(); code == unsupportedFileTypeCode {
			return fmt.Errorf("%w: %w", errFileKindRefused, err)
		}
	}
	return err
}

// maxBytes is the smaller of what one request can carry and what the operator allows.
func (t attachDocument) maxBytes(ctx context.Context) int64 {
	if operator := t.docs.MaxBytes(ctx); operator > 0 && operator < maxInlineFileBytes {
		return operator
	}
	return maxInlineFileBytes
}

func requireMediaType(declared string) error {
	media, _, err := mime.ParseMediaType(declared)
	if err == nil && strings.Contains(media, "/") {
		return nil
	}
	return &BadArgsError{
		Field:    "content_type",
		Cause:    fmt.Errorf("`content_type` %q is not a media type", declared),
		Guidance: "send type/subtype, such as application/pdf or image/png",
	}
}

// decodeFile measures the encoded text against the cap before decoding it, so
// an oversize file is refused without a second copy of it in memory.
func decodeFile(encoded string, maxBytes int64) ([]byte, error) {
	// Line breaks are skipped by the decoder, so they are not counted as content.
	chars := len(encoded) - strings.Count(encoded, "\n") - strings.Count(encoded, "\r")
	if int64(chars) > int64(base64.StdEncoding.EncodedLen(int(maxBytes))) {
		return nil, tooLargeToAttach(int64(base64.StdEncoding.DecodedLen(chars)), maxBytes)
	}
	content, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, &BadArgsError{
			Field:    argContentBase64,
			Cause:    errors.New("`content_base64` is not standard base64"),
			Guidance: "encode the whole file with the standard alphabet and = padding, not the URL-safe one",
		}
	}
	if len(content) == 0 {
		return nil, &BadArgsError{
			Field: argContentBase64, Code: codeRequired,
			Cause: errors.New("`content_base64` decodes to an empty file; send the file's bytes"),
		}
	}
	if int64(len(content)) > maxBytes {
		return nil, tooLargeToAttach(int64(len(content)), maxBytes)
	}
	return content, nil
}

func tooLargeToAttach(size, maxBytes int64) error {
	return &BadArgsError{
		Field: argContentBase64,
		Cause: fmt.Errorf("the file is %s, over the %s an agent may attach",
			httperr.Megabytes(size), httperr.Megabytes(maxBytes)),
		Guidance: "a larger file is uploaded in the Margince app; do not save its text as a note instead",
	}
}

// --- list_documents (🟢 read) ---

type listDocuments struct{ docs Documents }

func (t listDocuments) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "list_documents", Title: "List a record's documents", Version: toolVersionV1,
		Description:   listDocumentsCopy.render(),
		Instead:       listDocumentsCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "listAttachments",
		InputSchema: schema(`{"type":"object","required":["entity_type","entity_id"],
			"properties":{` + documentParentSchema() + `,
			"cursor":{"type":"string","description":"next_cursor from a previous page"},
			"limit":{"type":"integer","minimum":1,"maximum":50}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[DocumentPage](),
	}
}

func (t listDocuments) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		EntityType string   `json:"entity_type"`
		EntityID   ids.UUID `json:"entity_id"`
		Cursor     string   `json:"cursor"`
		Limit      int      `json:"limit"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := requireDocumentParent(args.EntityType); err != nil {
		return nil, err
	}
	parent := RecordLink{EntityType: args.EntityType, EntityID: args.EntityID}
	page, err := t.docs.List(ctx, parent, args.Cursor, args.Limit)
	if err != nil {
		return nil, err
	}
	noteEvidence(ctx, datasource.EntityType(parent.EntityType), parent.EntityID)
	if page.Documents == nil {
		page.Documents = []AttachedDocument{}
	}
	return json.Marshal(page)
}
