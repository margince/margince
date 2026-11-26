// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const memberNameArgument = "name"

const (
	memberParameter  = "member"
	memberEmailField = "email"
	// memberDomainField carries the login's folded domain, because Postgres
	// cannot fold IDNA and the stored column is folded on write.
	memberDomainField = "domain_folded"
	mergedFromField   = "merged_from"
)

// The three ways an account counts as used. unusedMemberSQL, the roster's
// refusal reasons and the retirement preview all read these fragments, so the
// offer and the reason it gives come from one definition.
func passwordSetSQL(alias string) string { return alias + `.password_hash IS NOT NULL` }

func signedInSQL(alias string) string {
	return `(` + alias + `.last_active_at IS NOT NULL
 OR EXISTS (SELECT 1 FROM session used WHERE used.user_id = ` + alias + `.id))`
}

func providerLinkedSQL(alias string) string {
	return `EXISTS (SELECT 1 FROM federated_identity linked WHERE linked.user_id = ` + alias + `.id)`
}

func unusedMemberSQL(alias string) string {
	return `NOT (` + passwordSetSQL(alias) + ` OR ` + signedInSQL(alias) + ` OR ` + providerLinkedSQL(alias) + `)`
}

// memberUsage is which of the three signs of use an account shows.
type memberUsage struct{ PasswordSet, SignedIn, ProviderLinked bool }

func (u memberUsage) unused() bool { return !u.PasswordSet && !u.SignedIn && !u.ProviderLinked }

// memberUsageColumns selects the three flags in memberUsage's field order.
func memberUsageColumns(alias string) string {
	return passwordSetSQL(alias) + `, ` + signedInSQL(alias) + `, ` + providerLinkedSQL(alias)
}

func requireUnusedMemberEmail(ctx context.Context, tx pgx.Tx, member ids.UserID) error {
	var unused bool
	if err := tx.QueryRow(ctx, `SELECT `+unusedMemberSQL("app_user")+` FROM app_user WHERE id = @member`, pgx.StrictNamedArgs{memberParameter: member}).Scan(&unused); err != nil {
		return err
	}
	if !unused {
		return fmt.Errorf("%w: this sign-in address has already been used and cannot be changed here", apperrors.ErrConflict)
	}
	return nil
}

var errRemovedEmail = fmt.Errorf("%w: this address belongs to a permanently removed member and remains reserved for historical attribution", apperrors.ErrConflict)

func refuseRemovedEmail(ctx context.Context, tx pgx.Tx, email string) error {
	var removed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_user u
		 WHERE `+storekit.SeatLoginIsSQL("u", "@"+memberEmailField, "@"+memberDomainField)+`
		   AND u.removed_at IS NOT NULL)`, memberLoginArgs(email)).Scan(&removed); err != nil {
		return err
	}
	if removed {
		return errRemovedEmail
	}
	return nil
}

// errAlreadyInvited refuses an invite to a member who is still waiting on one.
// That member exists, and a resend from the roster reaches them.
var errAlreadyInvited = fmt.Errorf("%w: this address already has an invitation waiting", apperrors.ErrConflict)

func refuseWaitingInvitation(ctx context.Context, tx pgx.Tx, email string) error {
	var waiting bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_user u
		 WHERE `+storekit.SeatLoginIsSQL("u", "@"+memberEmailField, "@"+memberDomainField)+`
		   AND `+ActivatableMemberSQL("u")+` AND u.status = '`+userStatusInvited+`'
		   AND NOT u.is_agent AND u.removed_at IS NULL)`, memberLoginArgs(email)).Scan(&waiting); err != nil {
		return err
	}
	if waiting {
		return errAlreadyInvited
	}
	return nil
}

// typedLoginAddress lowercases and trims the address, and leaves its domain
// un-IDNA-folded.
//
// The raw arm is the only one that answers for a seat whose
// email_domain_folded is unwritten, and FoldLoginDomainsTx writes those a page
// at a time.
func typedLoginAddress(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

// sameLoginIdentity reports whether two spellings name one seat login. The
// local parts match lowercased and the domains match once folded, as
// SeatLoginIsSQL reads them. A Unicode domain, its punycode form and a trailing
// dot are one address.
func sameLoginIdentity(a, b string) bool {
	a, b = typedLoginAddress(a), typedLoginAddress(b)
	if a == b {
		return true
	}
	domainA, domainB := owndomain.OfAddress(a), owndomain.OfAddress(b)
	if domainA == "" || domainA != domainB {
		return false
	}
	return a[:strings.LastIndex(a, "@")] == b[:strings.LastIndex(b, "@")]
}

// memberLoginArgs binds one typed address for a seat-login comparison: the
// address as typed, and its domain folded the way the column stores it.
func memberLoginArgs(email string) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{
		memberEmailField:  typedLoginAddress(email),
		memberDomainField: owndomain.OfAddress(email),
	}
}
