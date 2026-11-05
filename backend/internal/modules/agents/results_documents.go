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
	// Category is what kind of document this is; "link" is a document link a
	// mail carried, which has no bytes.
	Category string `json:"category,omitempty"`
	// URL is the address of a link, absent for a file.
	URL string `json:"url,omitempty"`
	// ContractID is the agreement the file was filed against, absent for none.
	ContractID *ids.UUID `json:"contract_id,omitempty"`
	// Origin is the mail the file came with. Absent on an upload, and absent
	// when the caller may not read the mail.
	Origin *DocumentOrigin `json:"origin,omitempty"`
}

// DocumentOrigin names the mail a file came with, as the Documents tab names it.
type DocumentOrigin struct {
	ActivityID ids.UUID  `json:"activity_id"`
	Kind       string    `json:"kind"`
	Subject    string    `json:"subject,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	// OccurredPrecision is "day" when only OccurredAt's date is known; absent
	// on an instant. A day's date is its UTC date, so it is named unconverted.
	OccurredPrecision string `json:"occurred_precision,omitempty"`
	SenderName        string `json:"sender_name,omitempty"`
	SenderAddress     string `json:"sender_address,omitempty"`
}

// DocumentPage is one page of a record's documents, newest first.
type DocumentPage struct {
	Documents []AttachedDocument `json:"documents"`
	// Absent rather than empty on the last page, as on every paged tool.
	NextCursor string `json:"next_cursor,omitempty"`
}
