// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// System reads for a job tick or a bus delivery. No caller's grants narrow
// them, so each answers ErrAttendedRead wherever a user gets the result.
// That guards the call, not a unit that stores what it read and serves it
// later. A unit asks Authority().CanRead before it shows a colleague anything,
// and CanRead answers a signed-in human about themselves on any call.

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrAttendedRead is a system read from an invocation that is not a job tick or
// a bus delivery.
var ErrAttendedRead = errors.New("extension: a system read runs on a job tick or a bus delivery only")

// ErrUndeclaredRead is a system read the unit's Extension.SystemReads does not
// list.
var ErrUndeclaredRead = errors.New("extension: this unit does not declare the system read in Extension.SystemReads")

// SystemRead names one group of system reads a unit declares it makes.
type SystemRead string

const (
	// SystemReadDeals admits Deals().Get and Deals().Responsible.
	SystemReadDeals SystemRead = "deals"
	// SystemReadAuthority admits Authority().CanRead and Authority().CanWrite.
	SystemReadAuthority SystemRead = "authority"
	// SystemReadDirectory admits Directory().Emails.
	SystemReadDirectory SystemRead = "directory"
)

// Validate refuses a value that names no group of system reads.
func (r SystemRead) Validate() error {
	switch r {
	case SystemReadDeals, SystemReadAuthority, SystemReadDirectory:
		return nil
	}
	return fmt.Errorf("extension: %q is not a system read; declare %q, %q or %q",
		string(r), SystemReadDeals, SystemReadAuthority, SystemReadDirectory)
}

// MaxResponsible bounds what Deals().Responsible answers and how many users one
// Directory().Emails call takes, so the second always fits what the first gave.
const MaxResponsible = 400

// DealSummary is one deal as a background reader sees it, archived or not.
//
// It is not the contract's Deal. That shape answers per caller (masked_fields,
// writable), and a read with no caller has nobody to answer for. It also names
// the stage by id, where a reader shows its name.
type DealSummary struct {
	ID        string
	Name      string
	StageName string
	// Status is open, won or lost.
	Status string
	// AmountMinor is in minor units of Currency; both are nil on a deal with
	// no amount.
	AmountMinor *int64
	Currency    *string
	// AmountDisplay is the amount as the product prints it in English
	// ("12,500.00 EUR", "120,000 JPY"), so a unit needs no table of each
	// currency's decimals. Empty on a deal with no amount.
	AmountDisplay string
	// ExpectedClose is a calendar date, carried at midnight UTC.
	ExpectedClose *time.Time
	// OwnerID is empty for a deal with no owner.
	OwnerID  UserID
	Archived bool
}

// Responsible is one colleague a deal names.
type Responsible struct {
	UserID UserID
	// Owner marks the deal's owner; everyone else is assigned to it, alone or
	// through a team.
	Owner bool
}

// Deals reads the installation's deals for a job tick or a bus delivery.
type Deals interface {
	// Get reads one deal. A deal that does not exist, or was erased, answers
	// ErrNotFound; an id that is not a canonical UUID answers ErrInvalid.
	Get(ctx context.Context, dealID string) (DealSummary, error)

	// Responsible answers the colleagues a deal names: its owner, the users
	// assigned to it, and the members of the teams assigned to it. Agents,
	// members who cannot sign in, archived assignments and archived teams are
	// left out. The owner comes first, then the rest by email, and the list
	// stops at MaxResponsible.
	Responsible(ctx context.Context, dealID string) ([]Responsible, error)
}

// Authority answers what one colleague may read, under their own grants.
type Authority interface {
	// CanRead reports whether user may open record and read every one of
	// fields, named as the contract names them (`owner_id`). A mask that
	// lifts only where the user could write counts as withholding.
	//
	// An archived deal answers as a live one; a deal no row holds, false.
	// Only Type "deal" is answered. Another type, an unknown field or a
	// malformed id answers ErrInvalid; an agent seat or a user who cannot sign
	// in, false.
	//
	// On a call a human makes, it answers only about that human. So a route
	// can check its own caller before serving what the unit stored. An agent's
	// or a connector's call, or a question about anyone else, answers
	// ErrAttendedRead.
	CanRead(ctx context.Context, user UserID, record EntityRef, fields []string) (bool, error)

	// CanWrite reports whether user could change record now. It is live and
	// they hold a full seat. Their own grants and the row's sharing let them
	// update it. It answers on the same calls, about the same users, as CanRead.
	CanWrite(ctx context.Context, user UserID, record EntityRef) (bool, error)
}

// Directory reads colleagues' details for a job tick or a bus delivery.
type Directory interface {
	// Emails answers the address of each active human member among users. An
	// agent seat, or a user who does not exist or cannot sign in, is absent. More than
	// MaxResponsible users, or an id that is not a canonical UUID, answers
	// ErrInvalid.
	Emails(ctx context.Context, users []UserID) (map[UserID]string, error)
}
