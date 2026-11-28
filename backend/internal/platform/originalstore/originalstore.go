// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package originalstore preserves large provider originals outside the database.
package originalstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// InlineLimit bounds the bytes any inline original read can retain.
const (
	InlineLimit   = 1 << 20
	objectTimeout = 2 * time.Minute
)

// Reference separates trusted storage metadata from opaque provider JSON.
type Reference struct {
	Key             string        `json:"key"`
	SHA256          string        `json:"sha256"`
	Bytes           int64         `json:"bytes"`
	Subjects        *SubjectHints `json:"subjects,omitempty"`
	RFCMessageID    *string       `json:"rfc_message_id,omitempty"`
	MatchedSubjects []string      `json:"matched_subjects,omitempty"`
}

// Service keeps original-object access bound to its installation database.
type Service struct {
	db     *database.DB
	blob   blobstore.Store
	ledger *storedobjects.Ledger
	// storedKey holds the installation's stored-key id once read; it never changes.
	storedKey atomic.Pointer[ids.UUID]
}

type contextKey struct{}

// Bind scopes original access for both request and background transactions.
func Bind(ctx context.Context, db *database.DB, blob blobstore.Store) context.Context {
	s := &Service{db: db, blob: blob, ledger: storedobjects.NewLedger(db)}
	// Read the id here, before any caller holds a connection, so a later read
	// inside an open transaction needs no second one. A failure, or an
	// installation with no company yet, leaves the cache empty and the first use
	// reads again and reports it.
	if db != nil {
		if id, err := s.readStoredKeyOnHandle(ctx); err == nil {
			s.storedKey.Store(&id)
		}
	}
	return context.WithValue(ctx, contextKey{}, s)
}

// Objects is the object store Bind put on ctx. A background reader restores a
// slimmed original's moved parts from it.
//
//nolint:ireturn // the store has several providers behind one interface, and this only hands back what Bind was given.
func Objects(ctx context.Context) (blobstore.Store, error) {
	s, err := serviceOf(ctx)
	if err != nil {
		return nil, err
	}
	return s.blob, nil
}

func serviceOf(ctx context.Context) (*Service, error) {
	s, ok := ctx.Value(contextKey{}).(*Service)
	if !ok || s == nil || s.blob == nil {
		return nil, errors.New("original store is unavailable; configure object storage and retry")
	}
	return s, nil
}

// requireInstallation refuses an original access while no company is live.
func (s *Service) requireInstallation(ctx context.Context) error {
	if s.db == nil {
		return errors.New("original store requires a database")
	}
	if _, err := s.db.Workspace(ctx); err != nil {
		return err
	}
	return nil
}

// prefix is where every original of this installation is stored. It names the
// installation's stored-key id, the one every object already stored was
// written under, and never the id the request happens to carry.
func (s *Service) prefix(ctx context.Context, tx pgx.Tx) (string, error) {
	ws, err := s.storedKeyWorkspace(ctx, tx)
	if err != nil {
		return "", err
	}
	return blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](ws), "raw-original", ""), nil
}

// storedKeyWorkspace reads the id from installation_state once per handle.
func (s *Service) storedKeyWorkspace(ctx context.Context, tx pgx.Tx) (ids.UUID, error) {
	if s.db == nil {
		return ids.UUID{}, errors.New("original store requires a database")
	}
	if cached := s.storedKey.Load(); cached != nil {
		return *cached, nil
	}
	var id ids.UUID
	var err error
	if tx != nil {
		id, err = storekit.StoredKeyWorkspaceTx(ctx, tx)
	} else {
		id, err = s.readStoredKeyOnHandle(ctx)
	}
	if err != nil {
		return ids.UUID{}, fmt.Errorf("original store: reading the installation's stored-key id: %w", err)
	}
	s.storedKey.Store(&id)
	return id, nil
}

// readStoredKeyOnHandle reads the id in a transaction of its own, for a caller
// that holds none.
func (s *Service) readStoredKeyOnHandle(ctx context.Context) (ids.UUID, error) {
	var id ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		id, err = storekit.StoredKeyWorkspaceTx(ctx, tx)
		return err
	})
	return id, err
}

func (s *Service) validate(ctx context.Context, ref Reference) error {
	root, err := s.prefix(ctx, nil)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(ref.Key, root) || strings.Contains(strings.TrimPrefix(ref.Key, root), "/") ||
		len(ref.Key) == len(root) || ref.Bytes <= 0 || len(ref.SHA256) != sha256.Size*2 {
		return errors.New("stored original reference is invalid")
	}
	return nil
}

func writableServiceOf(ctx context.Context) (*Service, error) {
	service, err := serviceOf(ctx)
	if err != nil {
		return nil, err
	}
	if service.db == nil {
		return nil, errors.New("original store requires a database")
	}
	return service, nil
}

// Prepare retains provisional cleanup authority when the later domain write fails.
func Prepare(ctx context.Context, payload []byte) (inline []byte, ref *Reference, err error) {
	if len(payload) <= InlineLimit {
		return PrepareTx(ctx, nil, payload)
	}
	service, err := writableServiceOf(ctx)
	if err != nil {
		return nil, nil, err
	}
	err = service.db.Tx(ctx, func(tx pgx.Tx) error { inline, ref, err = PrepareTx(ctx, tx, payload); return err })
	return inline, ref, err
}

// PrepareTx uses the caller's transaction; rewrites keep their original-row
// lock through staging so erasure cannot leave a later upload behind.
func PrepareTx(ctx context.Context, tx pgx.Tx, payload []byte) ([]byte, *Reference, error) {
	if !json.Valid(payload) {
		return nil, nil, errors.New("provider original must be encoded as JSON")
	}
	if len(payload) <= InlineLimit {
		return payload, nil, nil
	}
	s, err := writableServiceOf(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := s.requireInstallation(ctx); err != nil {
		return nil, nil, err
	}
	root, err := s.prefix(ctx, tx)
	if err != nil {
		return nil, nil, err
	}
	var folded string
	var messageID, sender, chat *string
	if tx == nil {
		return nil, nil, errors.New("large original preparation requires a transaction")
	}
	if err := tx.QueryRow(ctx, `SELECT lower(value::text), value->>'rfc_message_id',
   value->'message'->'from'->>'id', value->'my_chat_member'->'chat'->>'id'
 FROM (SELECT @payload::jsonb AS value) original`, pgx.NamedArgs{"payload": payload}).Scan(&folded, &messageID, &sender, &chat); err != nil {
		return nil, nil, fmt.Errorf("validate provider original encoding: %w", err)
	}
	var channels []string
	for _, id := range []*string{sender, chat} {
		if id != nil {
			channels = append(channels, *id)
		}
	}
	sum := sha256.Sum256(payload)
	ref := &Reference{Key: root + ids.NewV7().String(), SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(payload)), Subjects: subjectHints(folded, channels), RFCMessageID: messageID}
	ctx, cancel := context.WithTimeout(ctx, objectTimeout)
	defer cancel()
	if err := s.ledger.Record(ctx, storedobjects.KindRawOriginal, ref.Key); err != nil {
		return nil, nil, err
	}
	if err := s.blob.Put(ctx, ref.Key, bytes.NewReader(payload), ref.Bytes, "application/json"); err != nil {
		return nil, nil, fmt.Errorf("store provider original: %w", err)
	}
	return placeholder(payload), ref, nil
}

// The inline value retains its JSON type for candidate selectors, while readers
// obtain the evidence through the separate, trusted object column.
func placeholder(payload []byte) []byte {
	text := bytes.TrimSpace(payload)
	if len(text) == 0 {
		return []byte("null")
	}
	switch text[0] {
	case '{':
		return []byte("{}")
	case '[':
		return []byte("[]")
	case '"':
		return []byte(`""`)
	case 't', 'f':
		return []byte("false")
	case 'n':
		return []byte("null")
	default:
		return []byte("0")
	}
}

// Adopt clears the provisional intent only in the owning domain transaction.
func Adopt(ctx context.Context, tx pgx.Tx, ref *Reference) error {
	if ref == nil {
		return nil
	}
	return storedobjects.Clear(ctx, tx, ref.Key)
}

// SQL is used only with compile-time table aliases. The wrapper is never read
// from provider JSON, so a provider cannot supply an object-store reference.
func SQL(alias string) string {
	return "CASE WHEN " + alias + ".id IS NULL THEN NULL ELSE jsonb_build_object('inline', " + alias + ".payload, 'object', " + alias + ".payload_object) END"
}

// Decode accepts a trusted storage projection, never provider-controlled references.
func Decode(ctx context.Context, wrapper []byte) ([]byte, error) {
	if len(wrapper) == 0 {
		return nil, nil
	}
	var stored struct {
		Inline json.RawMessage `json:"inline"`
		Object *Reference      `json:"object"`
	}
	if err := json.Unmarshal(wrapper, &stored); err != nil || stored.Inline == nil {
		return nil, errors.New("stored original envelope is invalid")
	}
	if stored.Object == nil {
		return stored.Inline, nil
	}
	return Read(ctx, *stored.Object)
}
