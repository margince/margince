// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A host's friendly booking address, such as /book/alex, beside the random
// slug. Rows live in booking_alias. A changed, cleared or replaced alias is
// retired, not deleted, so the address cannot move to another host while
// guests may still hold it.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	bookingAliasMin = 3
	// bookingAliasMax stays below the 32-character random slug, so an alias
	// can never equal or shadow one.
	bookingAliasMax = 30
	// bookingAliasCooldown is how long a retired alias stays unavailable to
	// any other host.
	bookingAliasCooldown = 90 * 24 * time.Hour
	aliasField           = "alias"
)

var bookingAliasShape = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// reservedAliasWords are the api's own words under /v1/public/booking/. The
// app's reserved ids and prefixes are reservedBookingIDs and
// reservedBookingPrefixes, owned by frontend/src/lib/bookingaddress.ts.
var reservedAliasWords = []string{"company-logo"}

func aliasRefusal(code, message string) *SchedulingArgumentError {
	return &SchedulingArgumentError{Field: aliasField, Code: code, Message: message}
}

// normalizeBookingAlias trims and lowercases. A random slug is never
// normalized: it is resolved as typed.
func normalizeBookingAlias(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// validateBookingAlias checks a normalized, non-empty alias.
func validateBookingAlias(alias string) error {
	switch length := utf8.RuneCountInString(alias); {
	case length < bookingAliasMin:
		return aliasRefusal("too_short", fmt.Sprintf("Use at least %d characters", bookingAliasMin))
	case length > bookingAliasMax:
		return aliasRefusal("too_long", fmt.Sprintf("Use at most %d characters", bookingAliasMax))
	}
	if !bookingAliasShape.MatchString(alias) {
		return aliasRefusal("invalid", "Use letters, digits and hyphens, starting and ending with a letter or digit")
	}
	if bookingSlugReserved(alias) || slices.Contains(reservedAliasWords, alias) {
		return aliasRefusal("reserved", "This address is reserved. Choose another one")
	}
	return nil
}

var (
	errAliasTaken        = aliasRefusal("taken", "Another booking page uses this address. Choose another one")
	errAliasRecentlyUsed = aliasRefusal("recently_used", "Another booking page used this address recently. Choose another one")
	errAliasNoPage       = aliasRefusal("no_booking_link", "Create your booking link before you choose an address")
)

// bookingAddressAudit is the before and after image of an alias change.
type bookingAddressAudit struct {
	Alias string `json:"alias"`
}

// SetSchedulingAlias claims, changes or clears the acting host's own alias.
// An empty value clears it.
func (s *Store) SetSchedulingAlias(ctx context.Context, raw string) (crmcontracts.SchedulingProfile, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return crmcontracts.SchedulingProfile{}, err
	}
	alias := normalizeBookingAlias(raw)
	if alias != "" {
		if err := validateBookingAlias(alias); err != nil {
			return crmcontracts.SchedulingProfile{}, err
		}
	}
	if err := s.tx(ctx, func(tx pgx.Tx) error { return s.setBookingAliasTx(ctx, tx, host, alias) }); err != nil {
		return crmcontracts.SchedulingProfile{}, err
	}
	return s.SchedulingProfile(ctx)
}

func (s *Store) setBookingAliasTx(ctx context.Context, tx pgx.Tx, host ids.UserID, alias string) error {
	if err := storekit.LockWriteIdentity(ctx, tx, "booking_page", host.String()); err != nil {
		return err
	}
	args := schedulingArgs{}
	var page ids.UUID
	var policy *crmcontracts.SchedulingProfile
	err := tx.QueryRow(ctx, `SELECT id, scheduling_policy FROM booking_page WHERE host_user_id=`+args.add(host)+
		` AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, args...).Scan(&page, &policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return errAliasNoPage
	}
	if err != nil {
		return err
	}
	current, err := liveBookingAlias(ctx, tx, host)
	if err != nil || current == alias {
		return err
	}
	if alias != "" {
		if err := s.claimBookingAlias(ctx, tx, host, page, alias); err != nil {
			return err
		}
	} else if err := s.retireBookingAliasTx(ctx, tx, host); err != nil {
		return err
	}
	audit, err := storekit.Audit(ctx, tx, "update", "booking_page", page, bookingAddressAudit{Alias: current}, bookingAddressAudit{Alias: alias})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, audit, page, crmcontracts.InternalEventBookingPageUpdated{Enabled: policy != nil && policy.Enabled})
}

// liveBookingPageAlias is the host's live alias on their current page, or "".
func liveBookingPageAlias(ctx context.Context, tx pgx.Tx, host ids.UserID) (string, error) {
	args := schedulingArgs{}
	var alias string
	err := tx.QueryRow(ctx, `SELECT a.alias FROM booking_alias a JOIN booking_page p ON p.id = a.booking_page_id
   WHERE a.host_user_id=`+args.add(host)+` AND a.retired_at IS NULL AND p.revoked_at IS NULL`, args...).Scan(&alias)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return alias, err
}

func liveBookingAlias(ctx context.Context, tx pgx.Tx, host ids.UserID) (string, error) {
	args := schedulingArgs{}
	var alias string
	err := tx.QueryRow(ctx, `SELECT alias FROM booking_alias WHERE host_user_id=`+args.add(host)+` AND retired_at IS NULL FOR UPDATE`, args...).Scan(&alias)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return alias, err
}

// retireBookingAliasTx retires the host's live alias, if any. Changing,
// clearing and Replace address all end an alias this way.
func (s *Store) retireBookingAliasTx(ctx context.Context, tx pgx.Tx, host ids.UserID) error {
	args := schedulingArgs{}
	_, err := tx.Exec(ctx, `UPDATE booking_alias SET retired_at=`+args.add(s.now())+` WHERE host_user_id=`+args.add(host)+` AND retired_at IS NULL`, args...)
	return err
}

// claimBookingAlias gives alias to host's page. The unique key on alias
// decides a race: the loser of two concurrent first claims gets "taken".
func (s *Store) claimBookingAlias(ctx context.Context, tx pgx.Tx, host ids.UserID, page ids.UUID, alias string) error {
	args := schedulingArgs{}
	var slugClash bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM booking_page WHERE slug=`+args.add(alias)+`)`, args...).Scan(&slugClash); err != nil {
		return err
	}
	if slugClash {
		return errAliasTaken
	}
	args = schedulingArgs{}
	var id ids.UUID
	var owner *ids.UserID
	var retired *time.Time
	err := tx.QueryRow(ctx, `SELECT id, host_user_id, retired_at FROM booking_alias WHERE alias=`+args.add(alias)+` FOR UPDATE`, args...).Scan(&id, &owner, &retired)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return s.insertBookingAlias(ctx, tx, host, page, alias)
	}
	switch {
	case retired == nil:
		return errAliasTaken
	case (owner == nil || *owner != host) && s.now().Before(retired.Add(bookingAliasCooldown)):
		return errAliasRecentlyUsed
	}
	if err := s.retireBookingAliasTx(ctx, tx, host); err != nil {
		return err
	}
	args = schedulingArgs{}
	_, err = tx.Exec(ctx, `UPDATE booking_alias SET host_user_id=`+args.add(host)+`, booking_page_id=`+args.add(page)+
		`, created_at=`+args.add(s.now())+`, retired_at=NULL WHERE id=`+args.add(id), args...)
	return err
}

func (s *Store) insertBookingAlias(ctx context.Context, tx pgx.Tx, host ids.UserID, page ids.UUID, alias string) error {
	if err := s.retireBookingAliasTx(ctx, tx, host); err != nil {
		return err
	}
	args := schedulingArgs{}
	tag, err := tx.Exec(ctx, `INSERT INTO booking_alias (alias, host_user_id, booking_page_id, created_at) VALUES (`+
		args.add(alias)+`, `+args.add(host)+`, `+args.add(page)+`, `+args.add(s.now())+`) ON CONFLICT (alias) DO NOTHING`, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errAliasTaken
	}
	return nil
}
