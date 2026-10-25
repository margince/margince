// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// categoryLink is the attachment category of a document link sent by mail: a
// row that points at a document and holds no bytes.
const categoryLink = "link"

// linkPartPrefix starts the part identity of a link row, so a link's identity
// cannot equal a file's "part:N".
const linkPartPrefix = "link:"

// CapturedLink is one document link a message carried: the canonical address
// and the text the sender showed it under, empty when none was shown.
type CapturedLink struct {
	URL  string
	Text string
}

// RecordCapturedLinks writes an attachment row for each document link of a
// newly captured activity, inside the capture transaction.
//
// The row hangs off the activity alone, so the mail's audience decides who sees
// it. It has bytes_withheld and no storage key, so every byte reader refuses it.
// The unique index on message and address hash stops a second row on a replay
// and keeps an archived link archived.
func (s *Store) RecordCapturedLinks(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
	from CapturedFileSource, links []CapturedLink,
) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	if len(links) == 0 {
		return nil
	}
	from.Category = categoryLink
	for _, link := range links {
		address := link.URL
		sum := sha256.Sum256([]byte(address))
		row := capturedFileRow{
			id: ids.NewV7(),
			file: CapturedFile{
				PartID:   linkPartPrefix + hex.EncodeToString(sum[:]),
				Filename: linkFilename(link),
			},
			withheld: true,
			url:      &address,
		}
		if err := insertCapturedAttachment(ctx, tx, activityID, from, row); err != nil {
			return err
		}
	}
	return nil
}

// linkFilename is the name a link row shows: the sender's anchor text cleaned of
// control characters, else the host.
func linkFilename(link CapturedLink) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, link.Text)
	if name = strings.Join(strings.Fields(name), " "); name != "" {
		return name
	}
	return linkHostOf(link.URL)
}

// linkHostOf is the lowercased host of an address, or "link" for one that does
// not parse, so a row always has a name.
func linkHostOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Hostname() == "" {
		return "link"
	}
	return strings.ToLower(parsed.Hostname())
}
