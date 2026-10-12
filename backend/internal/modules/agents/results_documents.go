// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AttachedDocument is one file stored on a record, as the Documents tab lists
// it. The storage key is never part of it: bytes are fetched in the app.
type AttachedDocument struct {
	RecordLink
	AttachmentID ids.UUID  `json:"attachment_id"`
	Filename     string    `json:"filename"`
	ContentType  string    `json:"content_type"`
	ByteSize     int64     `json:"byte_size"`
	Checksum     string    `json:"checksum"`
	CapturedBy   string    `json:"captured_by"`
	CreatedAt    time.Time `json:"created_at"`
	// ContractID is the agreement the file was filed against, absent for none.
	ContractID *ids.UUID `json:"contract_id,omitempty"`
}

// DocumentPage is one page of a record's documents, newest first.
type DocumentPage struct {
	Documents []AttachedDocument `json:"documents"`
	// Absent rather than empty on the last page, as on every paged tool.
	NextCursor string `json:"next_cursor,omitempty"`
}
