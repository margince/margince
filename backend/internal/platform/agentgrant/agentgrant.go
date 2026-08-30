// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package agentgrant is the seam between the rep's standing answer and the
// credential that answer commits.
//
// TWO HALVES OF ONE FACT, IN TWO MODULES. What records that a rep agreed to an
// agent working overnight is a row owned by agents/runner. What actually
// authorizes the run is a passport owned by identity. Neither module may import
// the other, and the two must commit TOGETHER: a mint committed beside a failed
// grant is live authority nothing points at, and a grant committed beside a
// failed mint claims an authority that does not exist.
//
// So the module holding the credential takes this port, and compose supplies
// the implementation. The transaction belongs to the caller for exactly that
// reason — a port that opened its own would reintroduce the two-writes problem
// it exists to remove.
package agentgrant

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The two answers a rep can give. A row exists only once they have answered,
// and the absence of one is a third, distinct state — "never asked" — that the
// product has to be able to tell from a decline, or it asks the declining rep
// again every night.
const (
	StateGranted  = "granted"
	StateDeclined = "declined"
)

// Answer is one rep's standing answer for one agent, as a reader sees it.
type Answer struct {
	Spec  string
	State string
	// PassportID is the credential the answer named, present exactly when
	// granted. Nil means the rep declined.
	PassportID *ids.PassportID
	// CredentialUsable is whether that passport is live RIGHT NOW — not
	// revoked, not expired. It is read from the passport rather than stored,
	// because it changes at a moment nothing writes to the grant row: a stored
	// copy would say "granted" about a credential that stopped working hours
	// ago.
	CredentialUsable bool
	// PassportScopes is what the credential was minted with. The grant table
	// can say what a passport HOLDS; only the module that mints them knows what
	// an agent NEEDS, so the comparison belongs on the reader's side of this
	// port rather than behind it.
	PassportScopes []string
	DecidedAt      time.Time
}

// Store is what the credential-holding module needs from the grant table.
//
// Every method reads the rep from the ACTING PRINCIPAL and takes no user id.
// That is the security property, not a convenience: an argument would let one
// person answer for another, and "the admin turned it on for you" is precisely
// what a standing grant exists not to be.
type Store interface {
	// MyAnswerTx reads the acting rep's own answer inside the caller's
	// transaction. found=false means they were never asked.
	MyAnswerTx(ctx context.Context, tx pgx.Tx, spec string) (Answer, bool, error)
	// RecordAnswerTx writes the acting rep's answer inside the caller's
	// transaction, replacing any previous one — a rep who declined and later
	// changes their mind is giving a NEW answer to the same question.
	RecordAnswerTx(ctx context.Context, tx pgx.Tx, spec, state string, passportID *ids.PassportID) error
}
