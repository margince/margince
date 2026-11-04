// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Filing a chat screenshot. The user confirms the text a reading copied out of
// the image. One message activity then lands on the contact, with the image.
// Whose words the body is rides captured_by, as in the deal accept. The
// reading's own text is the reader's; a typed or changed body is the user's.

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChatCaptureFile executes fileChatCapture.
type ChatCaptureFile struct {
	attachments *activities.Store
}

// NewChatCaptureFile wires the engine over the shared pool.
func NewChatCaptureFile(pool *pgxpool.Pool) *ChatCaptureFile {
	return &ChatCaptureFile{attachments: activities.NewStore(InstallationDB(pool))}
}

// File resolves the reading the user was shown, decides whose words the body
// is, and files the chat.
func (c *ChatCaptureFile) File(
	ctx context.Context, attachmentID ids.UUID, req crmcontracts.FileChatCaptureRequest,
) (crmcontracts.FiledChatCapture, error) {
	readID := ids.UUID(req.ExtractionId)
	if readID == (ids.UUID{}) {
		return crmcontracts.FiledChatCapture{}, &activities.ChatCaptureError{
			Field: "extraction_id", Code: "required", Message: "name the reading the chat was read by"}
	}
	read, err := c.attachments.GetExtractionRead(ctx, attachmentID, readID)
	if err != nil {
		return crmcontracts.FiledChatCapture{}, err
	}
	body := ""
	if req.Body != nil {
		body = *req.Body
	}
	filing := activities.ChatCaptureFiling{
		AttachmentID: attachmentID,
		ReadingID:    readID,
		Channel:      string(req.ChannelProvider),
		Body:         body,
	}
	if bodyIsTheReading(read, body) {
		filing.WriteAs = asChatReader
	}
	filed, err := c.attachments.FileChatCapture(ctx, filing)
	if err != nil {
		return crmcontracts.FiledChatCapture{}, err
	}
	return crmcontracts.FiledChatCapture{ActivityId: openapi_types.UUID(filed)}, nil
}

// bodyIsTheReading reports whether the body is the text the reading grounded,
// unchanged. An empty body is nobody's words, so it stays the user's.
func bodyIsTheReading(read activities.ExtractionRead, body string) bool {
	if strings.TrimSpace(body) == "" {
		return false
	}
	for _, f := range read.Fields {
		if f.Field == chatFieldText && !f.Omitted {
			return strings.TrimSpace(f.Value) == strings.TrimSpace(body)
		}
	}
	return false
}

// asChatReader is the principal an unedited body is written under: the reader,
// acting for the user who confirmed it.
func asChatReader(ctx context.Context) context.Context {
	human, ok := principal.Actor(ctx)
	if !ok {
		return ctx
	}
	return principal.WithActor(ctx, principal.Principal{
		Type:       principal.PrincipalSystem,
		ID:         extractionExecutorID,
		UserID:     human.UserID,
		OnBehalfOf: human.UserID,
	})
}

// chatCaptureHandlers is the transport for fileChatCapture.
type chatCaptureHandlers struct {
	file *ChatCaptureFile
}

func (h chatCaptureHandlers) FileChatCapture(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.FileChatCaptureRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.file.File(r.Context(), ids.UUID(id), req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
