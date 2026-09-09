// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The Art. 15 assembly the integration lane needs, driven through the REAL
// seam and nothing else.
//
// It lives behind the integration tag so it is never linked into cmd/api or
// cmd/worker: the lane compiles this package with the tag, so it exercises the
// same subjectAccessSeam the server wires, while the shipped binaries carry no
// exported surface with no product caller.
//
// The seam rather than restoreSAROriginals directly, because the defect this
// guards is the WIRING: a restore that works and is never called leaves the
// export handing out stanzas, and a test that calls it itself cannot tell.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AssembleSARForTest returns the serialized Art. 15 package for one subject,
// through the same assembler the server binds when an object store is present.
func AssembleSARForTest(
	ctx context.Context, db *database.DB, blob blobstore.Store, personID ids.UUID,
) ([]byte, error) {
	return newSubjectAccessAssembler(db).withBlobstore(blob).AssemblePackage(ctx, personID)
}

// ExportedRawCapturePayloadsForTest returns the raw_capture payloads the export
// discloses, as the strings a subject would receive. A withheld payload comes
// back as the empty string, which is how a caller tells "restored" from
// "listed but unavailable".
func ExportedRawCapturePayloadsForTest(
	ctx context.Context, db *database.DB, blob blobstore.Store, personID ids.UUID,
) ([]string, error) {
	body, err := AssembleSARForTest(ctx, db, blob, personID)
	if err != nil {
		return nil, err
	}
	var pkg struct {
		RawCapture []map[string]any `json:"raw_capture"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return nil, fmt.Errorf("compose: reading back the test export: %w", err)
	}
	out := make([]string, 0, len(pkg.RawCapture))
	for _, row := range pkg.RawCapture {
		payload, disclosed := row["payload"].(string)
		if !disclosed {
			out = append(out, "")
			continue
		}
		out = append(out, payload)
	}
	return out, nil
}
