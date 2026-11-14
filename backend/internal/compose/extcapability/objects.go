// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// ErrObjectsUnwired is a role that bound no object store.
var ErrObjectsUnwired = errors.New("extension: this process has no object store bound")

// ErrObjectsFailed is what a unit sees of a failed store call.
var ErrObjectsFailed = errors.New("extension: the installation's object store could not answer")

// kind is the key segment, below the workspace, that holds every unit's objects.
// The core's own kinds never start with it.
const kind = "ext/"

// Prefix is the key prefix one unit's objects share in one workspace. It ends at
// the separator, as DeletePrefix requires.
func Prefix(ws ids.WorkspaceID, unit string) string {
	return blobstore.WorkspaceKey(ws, kind+unit, "")
}

// Objects is extension.Objects over a store. Gate checks the unit's declaration
// and returns the invocation's context, which carries its workspace.
type Objects struct {
	Store blobstore.Store
	Unit  string
	Gate  func(context.Context) (context.Context, error)
}

// key is where the named object lives for this invocation's workspace.
func (o Objects) key(ctx context.Context, name string) (context.Context, string, error) {
	ctx, err := o.Gate(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := extension.ValidateObjectName(name); err != nil {
		return nil, "", err
	}
	if o.Store == nil {
		return nil, "", ErrObjectsUnwired
	}
	raw, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, "", ErrObjectsFailed
	}
	return ctx, Prefix(ids.From[ids.WorkspaceKind](raw), o.Unit) + name, nil
}

// OpenStore opens the configured object store, or answers nil when the
// installation has none.
//
//nolint:ireturn // the store has three providers behind one interface.
func OpenStore(ctx context.Context, settings *config.Config) (blobstore.Store, error) {
	store, configured, err := blobstore.FromConfig(ctx, settings)
	if err != nil || !configured {
		return nil, err
	}
	return store, nil
}

// Put stores content under name. The content type is the one the bytes say.
func (o Objects) Put(ctx context.Context, name string, content []byte) error {
	ctx, key, err := o.key(ctx, name)
	if err != nil {
		return err
	}
	if len(content) > extension.MaxObjectBytes {
		return fmt.Errorf("%w: an object holds at most %d bytes", extension.ErrInvalid, extension.MaxObjectBytes)
	}
	if err := o.Store.Put(ctx, key, bytes.NewReader(content), int64(len(content)), extension.SniffContentType(content)); err != nil {
		return o.fault(ctx, err)
	}
	return nil
}

// Get reads the object, or answers ErrNotFound.
func (o Objects) Get(ctx context.Context, name string) (extension.Object, error) {
	ctx, key, err := o.key(ctx, name)
	if err != nil {
		return extension.Object{}, err
	}
	body, meta, err := o.Store.Get(ctx, key)
	if errors.Is(err, blobstore.ErrNotFound) {
		return extension.Object{}, extension.ErrNotFound
	}
	if err != nil {
		return extension.Object{}, o.fault(ctx, err)
	}
	content, err := io.ReadAll(io.LimitReader(body, extension.MaxObjectBytes+1))
	if closeErr := body.Close(); err == nil {
		err = closeErr
	}
	if err != nil || len(content) > extension.MaxObjectBytes {
		return extension.Object{}, o.fault(ctx, err)
	}
	return extension.Object{Content: content, ContentType: meta.ContentType}, nil
}

// Delete removes the object. A missing one is not an error.
func (o Objects) Delete(ctx context.Context, name string) error {
	ctx, key, err := o.key(ctx, name)
	if err != nil {
		return err
	}
	if err := o.Store.Delete(ctx, key); err != nil {
		return o.fault(ctx, err)
	}
	return nil
}

func (o Objects) fault(ctx context.Context, err error) error {
	slog.ErrorContext(ctx, "extcapability: an extension object call failed", "err", err, "unit", o.Unit)
	return ErrObjectsFailed
}

// Gone names the units recorded as composed at some time and not composed now.
func Gone(ever []string, composed []extension.Extension) []string {
	var gone []string
	for _, name := range ever {
		if !slices.ContainsFunc(composed, func(e extension.Extension) bool { return string(e.Name) == name }) {
			gone = append(gone, name)
		}
	}
	return gone
}

// SweepGone deletes the objects of every unit the installation recorded as
// composed (in the system_log rows of observeAction) and no longer composes. It
// derives the set from that history on every call, so a boot with no store, or
// a delete that failed, is made good by the next boot.
func SweepGone(ctx context.Context, pool *pgxpool.Pool, store blobstore.Store, observeAction, installation string, composed []extension.Extension) error {
	var ever []string
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT unit->>'name'
			  FROM system_log, jsonb_array_elements(detail->'extensions') AS unit
			 WHERE action = $1 AND detail->>'installation' = $2`, observeAction, installation)
		if err != nil {
			return err
		}
		ever, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return fmt.Errorf("extcapability: reading the units this installation has composed: %w", err)
	}
	return Sweep(ctx, store, Gone(ever, composed))
}

// Sweep deletes every object of each named unit in the context's workspace. It
// is idempotent, and a failure is returned so the caller can try again.
func Sweep(ctx context.Context, store blobstore.Store, removed []string) error {
	raw, ok := principal.WorkspaceID(ctx)
	if store == nil || !ok {
		return nil
	}
	for _, unit := range removed {
		if _, err := store.DeletePrefix(ctx, Prefix(ids.From[ids.WorkspaceKind](raw), unit)); err != nil {
			return fmt.Errorf("extcapability: deleting the objects of removed extension %q: %w", unit, err)
		}
	}
	return nil
}

// Preflight refuses a need that names no offered capability, one without a
// one-line reason, and one listed twice.
func Preflight(e extension.Extension) error {
	seen := make([]extension.Capability, 0, len(e.Needs))
	for _, need := range e.Needs {
		if err := need.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if slices.Contains(seen, need.Capability) {
			return fmt.Errorf("compose: extension %q declares the need %q twice", e.Name, need.Capability)
		}
		seen = append(seen, need.Capability)
	}
	return nil
}
