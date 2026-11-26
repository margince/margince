// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordowner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// ErrOwnerCannotBeAsked answers a request to a member who is suspended or has
// left. Each request says so in its own words.
var ErrOwnerCannotBeAsked = errors.New("recordowner: the member cannot be asked")

// AskableTx refuses a request to a member who is suspended or has left.
func AskableTx(ctx context.Context, tx pgx.Tx, member ids.UUID) error {
	askable, err := approvals.CanBeAsked(ctx, tx, member)
	if err != nil {
		return err
	}
	if !askable {
		return ErrOwnerCannotBeAsked
	}
	return nil
}

// Request is one request a colleague puts to another about a record.
type Request struct {
	Kind       string
	RecordType string
	RecordID   ids.UUID
	// Payload is the staged change; Identity names the fields of it that make
	// two presses the same request.
	Payload    any
	Identity   map[string]ids.UUID
	Summary    string
	SystemText *systemtext.Message
	TTL        time.Duration
	// OnceDeclined files nothing after the same request was declined.
	OnceDeclined bool
}

// FileTx files the request in its decider's inbox. A second press joins the
// open one. filed is false only when OnceDeclined found a decline.
func FileTx(ctx context.Context, tx pgx.Tx, staging *approvals.Service, req Request) (bool, error) {
	body, err := json.Marshal(req.Payload)
	if err != nil {
		return false, fmt.Errorf("recordowner: encoding a %s request: %w", req.Kind, err)
	}
	identityKey, err := json.Marshal(req.Identity)
	if err != nil {
		return false, fmt.Errorf("recordowner: encoding a %s request's identity: %w", req.Kind, err)
	}
	digest := sha256.Sum256(body)
	ttl := req.TTL
	in := approvals.StageInput{
		Why:            approvals.HeadlineSaysWhy,
		Kind:           req.Kind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     req.RecordType,
		TargetID:       req.RecordID,
		Summary:        req.Summary,
		SystemText:     req.SystemText,
		JoinPending:    true,
		Identity:       identityKey,
		TTL:            &ttl,
	}
	filed := true
	if req.OnceDeclined {
		_, filed, err = staging.StageUnlessDeclinedTx(ctx, tx, in)
	} else {
		_, err = staging.StageOrJoinPendingInTx(ctx, tx, in)
	}
	if errors.Is(err, approvals.ErrNoDecider) {
		return false, ErrOwnerCannotBeAsked
	}
	return filed, err
}
