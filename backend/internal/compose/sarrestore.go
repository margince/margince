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
	"io"

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
	if blob == nil || pkg == nil {
		return
	}
	for i, row := range pkg.RawCapture {
		// pgx decodes a jsonb string to a Go string, so an RFC822 original
		// arrives here as the message text itself. A payload that is not a
		// string is one of the column's other spellings — a calendar resource
		// stored as a JSON object — and carries no stanza to restore.
		payload, isText := row["payload"].(string)
		if !isText || !partslim.IsSlimmed([]byte(payload)) {
			continue
		}
		restored, err := partslim.RestoreStoredParts([]byte(payload),
			func(ref partslim.PartRef) ([]byte, error) {
				return readStoredPart(ctx, blob, ref)
			})
		if err != nil {
			// Withheld, not handed over half-answered: a stanza in an export is
			// a reference to bytes the subject has no way to fetch.
			pkg.RawCapture[i]["payload"] = nil
			continue
		}
		pkg.RawCapture[i]["payload"] = string(restored)
	}
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
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, ref.Bytes+1))
}
