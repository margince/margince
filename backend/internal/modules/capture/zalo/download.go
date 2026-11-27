// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/safedownload"
)

// DefaultMediaHosts are the hosts Zalo serves message attachments from. A
// pattern "*.example.com" matches any subdomain of example.com.
var DefaultMediaHosts = []string{"*.zdn.vn", "*.zadn.vn", "*.zaloapp.com", "*.zalo.me"}

// Downloadable reports whether an attachment is a file Zalo hosts at a URL. A
// link, a location and a business card carry no file.
func (a Attachment) Downloadable() bool {
	switch a.Type {
	case "image", "file", "gif", "audio", "video":
		return a.URL != ""
	}
	return false
}

// Download fetches the attachment's bytes from its payload URL, which must be
// https on a Zalo host. The URL is a signed link and takes no access token, so
// none is sent. Zalo's checksum, when it is a hex MD5 or SHA-256, must match.
func (c *Client) Download(ctx context.Context, a Attachment, maxBytes int64) ([]byte, error) {
	if a.Size > maxBytes {
		return nil, ErrMediaTooLarge
	}
	resp, err := c.cdn.Get(ctx, c.media, a.URL, nil, maxBytes)
	switch {
	case errors.Is(err, safedownload.ErrTooLarge):
		return nil, ErrMediaTooLarge
	case errors.Is(err, safedownload.ErrRefused):
		return nil, fmt.Errorf("zalo: the attachment url or a redirect is outside the allowed hosts: %w", ErrRejected)
	case err != nil:
		return nil, fmt.Errorf("zalo: the attachment could not be fetched: %w", ErrUnreachable)
	}
	switch {
	case resp.Status == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case resp.Status >= http.StatusInternalServerError:
		return nil, fmt.Errorf("zalo: the attachment host answered %d: %w", resp.Status, ErrUnreachable)
	case resp.Status < 200 || resp.Status >= 300:
		return nil, fmt.Errorf("zalo: the attachment host answered %d: %w", resp.Status, ErrRejected)
	}
	if !checksumHolds(resp.Body, a.Checksum) {
		return nil, ErrMediaChecksum
	}
	return resp.Body, nil
}

// checksumHolds checks the digest Zalo stated. Zalo documents no algorithm, so
// the length tells: 32 hex digits are MD5 and 64 are SHA-256. A value in any
// other form is no claim this code can test, and the file is kept.
func checksumHolds(data []byte, want string) bool {
	if _, err := hex.DecodeString(want); err != nil {
		return true
	}
	switch len(want) {
	case 32:
		return safedownload.MD5Matches(data, want)
	case 64:
		return safedownload.SHA256Matches(data, want)
	}
	return true
}
