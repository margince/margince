// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package vectorkit

import (
	"context"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Embedder is the embed lane a vector writer consumes. Compose injects the ai
// router (or the offline fake); a module never picks a model.
//
// EmbedIdentity is the current binding's stamp — "<provider>/<model>@<dims>" —
// and is cheap by contract, no API call, which is what lets every read, every
// job guard and the readiness probe compare against it. An empty identity means
// no embed lane is bound, which is a legitimate deployment shape rather than a
// fault.
type Embedder interface {
	Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error)
	EmbedIdentity() (identity string, dims int)
}

// IsZero reports whether every component is exactly 0. Cosine similarity
// against the zero vector is 0/0 = NaN, and a naive `ORDER BY sim DESC` sorts
// NaN FIRST — silently outranking every real match — so a zero vector must
// reach neither storage nor a query.
func IsZero(vec []float32) bool {
	for _, v := range vec {
		if v != 0 {
			return false
		}
	}
	return true
}

// Literal renders a vector in pgvector's input form. It is a value, never an
// identifier, so it rides as a bound parameter at the call site; this function
// only shapes it.
func Literal(vec []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// Unchanged reports whether a stored row is already current, so re-embedding it
// would cost a model call and change nothing.
//
// Both halves are required. Text matching under a CHANGED identity still needs
// re-embedding: the stored vector lives in a space the live query no longer
// shares, and leaving it stamped with a model that no longer serves the
// workspace makes it indistinguishable from a live row. An unembedded row
// (empty stored identity) is never unchanged — including when the live identity
// is also empty, which is what an unbound lane reports: answering otherwise
// would let a corpus holding no vectors at all read as fully embedded.
func Unchanged(storedHash, storedIdentity, newHash, liveIdentity string) bool {
	if storedIdentity == "" {
		return false
	}
	return storedHash == newHash && storedIdentity == liveIdentity
}
