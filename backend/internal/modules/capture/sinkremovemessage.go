// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The Sink's mail verb for an absence: the owner deleted, at the provider, a
// message this workspace had already captured.
//
// Capture's usual answer to mail it does not want is to write nothing, and for
// a message never captured that is right. It is wrong for one already held on
// the owner's behalf: the copy they threw away is still here, invisible to
// their colleagues and unreferenced by anything, and no later pull mentions it
// again. This is the write that collects it.
//
// What a removal MEANS — whether the captured copy follows the provider's — is
// not decided here. A colleague may hold the same message, and the law may
// require keeping it; both are questions about rows capture can see and duties
// it cannot, so the decision travels in on MessagePurger the way every other
// cross-module answer travels.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// MessagePurger decides and carries out what a provider-side deletion does to
// the captured copy. Compose injects it, because destroying a message is
// privacy's to perform and whose-claim-is-it is capture's to answer — neither
// module may import the other, so the composed seam owns the pair.
//
// Nil is a Sink that captures mail and acts on no deletions: what every fixture
// is until it says otherwise.
type MessagePurger func(ctx context.Context, seat ids.UUID, sourceSystem, sourceID string) error

// WithMessagePurger returns a copy that acts on provider-side deletions.
func (s *Sink) WithMessagePurger(purge MessagePurger) *Sink {
	c := *s
	c.purgeRemoved = purge
	return &c
}

// RemoveMessage acts on the owner deleting a captured message at the provider,
// satisfying connector.MessageRemover.
//
// The seat comes from the principal, not from the connector: a sync runs as the
// contact whose connection it is, and that is the only seat whose copy a
// removal may reach. A connector naming its own owner would be naming a seat it
// inferred from an address.
//
// A Sink composed without the seam does nothing and says so by succeeding — the
// connector's alternative is failing a whole mail pull over a verb this
// deployment never wired.
func (s *Sink) RemoveMessage(ctx context.Context, key connector.NaturalKey) error {
	if s.purgeRemoved == nil {
		return nil
	}
	if key.SourceSystem == "" || key.SourceID == "" {
		return fmt.Errorf("capture: acting on a provider-side deletion needs a natural key")
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		// No seat on the context is a wiring fault, not a message to drop: a
		// removal acted on under nobody's authority would destroy whichever
		// copy the query happened to find.
		return fmt.Errorf("capture: a provider-side deletion arrived with no seat on the context")
	}
	return s.purgeRemoved(ctx, actor.UserID, key.SourceSystem, key.SourceID)
}
