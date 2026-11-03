// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeedBookingPageTx mints a host's public page inside the caller's
// transaction (the workspace bootstrap seeds one for the admin). The
// slug is a public identifier, not a credential. It is unguessable so the
// URL cannot be enumerated, and stored in plain text because it is the URL.
func SeedBookingPageTx(ctx context.Context, tx pgx.Tx, hostUserID ids.UserID) (string, error) {
	slug, err := mintBookingSlug(rand.Read)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_page (host_user_id, slug)
		VALUES ($1, $2)`,
		hostUserID, slug); err != nil {
		return "", fmt.Errorf("activities: seed booking page: %w", err)
	}
	return slug, nil
}

// The ids under /#/book/ that are not a host's public page. The owner is
// frontend/src/lib/bookingaddress.ts (BOOKING_HOST_IDS, BOOKING_ID_PREFIXES),
// next to the router that dispatches on them; frontend/nginx.conf refuses the
// same set. TestReservedBookingIDsMatchTheRouteOwner fails when they drift.
var (
	reservedBookingIDs      = []string{"contact", "preview"}
	reservedBookingPrefixes = []string{"contact-", "meeting-", "proposal-", "manage-"}
)

// bookingSlugReserved reports whether a slug would collide with a host
// control or a guest-token address. Base64url can spell "manage-".
func bookingSlugReserved(slug string) bool {
	if slices.Contains(reservedBookingIDs, slug) {
		return true
	}
	return slices.ContainsFunc(reservedBookingPrefixes, func(prefix string) bool { return strings.HasPrefix(slug, prefix) })
}

// mintBookingSlug draws 24 random bytes as base64url until the result is not
// reserved. A reserved draw is about one in 10^11, so the loop is bounded.
func mintBookingSlug(read func([]byte) (int, error)) (string, error) {
	for range 8 {
		var buf [24]byte
		if _, err := read(buf[:]); err != nil {
			return "", fmt.Errorf("activities: booking slug entropy: %w", err)
		}
		if slug := base64.RawURLEncoding.EncodeToString(buf[:]); !bookingSlugReserved(slug) {
			return slug, nil
		}
	}
	return "", errors.New("activities: booking slug entropy kept drawing reserved slugs")
}
