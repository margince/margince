// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Putting a slimmed provider original back together for the subject who is
// owed it.
//
// The part sweep (capture/partslimstore.go) takes a stored attachment's octets
// out of raw_capture.payload once the object store holds them. Art. 15 owes the
// MESSAGE, not a pointer to it, so an export that passed the column through
// would hand the subject an address they cannot resolve.
//
// It lives in compose because it joins two modules that may not import each
// other: privacy assembles the export, capture owns the spelling of the column
// and the stanza inside it. This is the same edge subjectaccessseam.go already
// is, which is why the restore is wired there rather than inside AssembleSAR —
// that function has some forty callers and takes no object store, and threading
// one through every one of them would say nothing about this change.
//
// A missing or altered object withholds that ONE payload and no more. Failing
// the package would lose a subject their entire export over one object, and the
// row stays LISTED either way: Art. 15 owes the fact that an original is held,
// which is the reasoning sarsections.go already applies to a limited message
// whose content waits for a release.

import (
	"context"
	"encoding/json"
	"io"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

// restoreSAROriginals rewrites every raw_capture payload in pkg that carries a
// reference stanza, in place.
//
// It returns nothing on purpose. There is no failure a caller could act on: an
// original this cannot rebuild has its payload cleared and its row kept, which
// is a disclosure decision rather than an error, and an error here would invite
// a future caller to fail the whole package on it.
func restoreSAROriginals(ctx context.Context, blob blobstore.Store, pkg *privacy.SARPackage) {
	if pkg == nil {
		return
	}
	for i, row := range pkg.RawCapture {
		payload, readable := storedOriginalText(row["payload"])
		if !readable || !partslim.IsSlimmed(payload) {
			continue
		}
		if blob == nil {
			// A slimmed original with no store to read from is exactly the
			// unrestorable case below, and it is withheld the same way: an api
			// running without the store the sweep wrote to must not answer
			// Art. 15 with an address the subject cannot resolve.
			pkg.RawCapture[i]["payload"] = nil
			continue
		}
		restored, err := partslim.RestoreStoredParts(payload,
			func(ref partslim.PartRef) ([]byte, error) {
				return readStoredPart(ctx, blob, ref)
			})
		if err != nil {
			// Withheld, not handed over half-answered: a stanza in an export is
			// a reference to bytes the subject has no way to fetch.
			pkg.RawCapture[i]["payload"] = nil
			continue
		}
		pkg.RawCapture[i]["payload"] = respelledOriginal(restored)
	}
}

// storedOriginalText is the original a payload holds, in either spelling a
// slimmed message can have. Text is a JSON string; bytes jsonb cannot hold as
// text are in the base64 envelope. A calendar resource has no stanza, and
// IsSlimmed passes it by.
//
//craft:ignore naked-any the payload is whatever pgx decoded from jsonb: a string or an object
func storedOriginalText(payload any) ([]byte, bool) {
	if text, isText := payload.(string); isText {
		return []byte(text), true
	}
	spelled, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	original, err := capture.DecodeStoredOriginal(spelled)
	return original, err == nil
}

// respelledOriginal stores a restored original back in the spelling the column
// gives it, so a binary message leaves in its envelope as it came in. One it
// cannot spell is withheld, like one it cannot restore.
//
//craft:ignore naked-any the value goes back into the package as decoded jsonb
func respelledOriginal(original []byte) any {
	encoded, err := capture.EncodeStoredOriginal(original)
	if err != nil {
		return nil
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil
	}
	return value
}

// readStoredPart reads one part's octets for the restore.
//
// Bounded by the stanza's own count plus one byte, so an object that has GROWN
// cannot be read unboundedly into memory here — RestoreStoredParts then rejects
// it for being the wrong length rather than trusting what it was handed.
func readStoredPart(ctx context.Context, blob blobstore.Store, ref partslim.PartRef) ([]byte, error) {
	body, _, err := blob.Get(ctx, ref.StorageKey)
	if err != nil {
		return nil, err
	}
	//craft:ignore swallowed-errors best-effort close of an object read to completion — the stanza's digest decides whether these were the right bytes
	defer func() { _ = body.Close() }()
	return io.ReadAll(io.LimitReader(body, ref.Bytes+1))
}
