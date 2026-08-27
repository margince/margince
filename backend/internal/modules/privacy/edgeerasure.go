// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The erasure boundary of a LINK, which is neither end's alone.
//
// Every other reader of the boundary asks it of the record the audit row sits
// on. An edge row sits on ('relationship', edge_id), and no write path in this
// tree ever records a scrub verb against `relationship` — the erasure cascade
// scrubs the participants, the ghosts and the edges themselves, and tombstones
// the RECORDS it scrubbed. So the row's own identity answers "never erased" for
// every link there has ever been, and a reversal judged by it alone would replay
// a role and a pair of dates that an Art. 17 certificate said were gone.
//
// What the image actually holds is the two records' relationship to each other,
// so the boundary that bounds it is the LATER of their two — and an erasure of
// either end is what this asks about.
//
// This is a BOUNDARY and not the read's own endpoint filter, which is why the two
// exist side by side rather than sharing one clause. The read withholds an edge
// whose other end carries a tombstone at all, in either direction, because every
// row of that edge is as much the erased end's data as the anchor's. Here the
// question is narrower and ordered: may THIS entry's image be written back? An
// entry newer than the tombstone describes a link made after the erasure, and
// refusing it would leave every such link permanently un-reversible.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EdgeBehindErasureBoundary reports whether one LINK audit row lies behind an
// erasure boundary: its own, or that of either record the link joins.
//
// The endpoint columns come from edgeEndpoints, the same slice the history read
// searches from and the same one gates/edgeendpointcensus_test.go holds against
// `relationship`'s shape constraints. A column added to the table therefore
// enters this guard with the read rather than being remembered separately —
// which is the failure this whole predicate exists to answer, one level up.
//
// auditID must name an edge row; the caller dispatches on entity_type and a row
// that is not one is a fault rather than an answer, because a record row's
// boundary is UnscrubbedImageSQL's and silently returning that would hide the
// dispatch having gone wrong.
func EdgeBehindErasureBoundary(ctx context.Context, tx pgx.Tx, auditID ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + fmt.Sprint(len(args)) }
	idPlaceholder := arg(auditID)
	typePlaceholder := arg(EdgeEntityType)
	verbs := arg(ScrubVerbs())

	ends := make([]string, 0, len(edgeEndpoints))
	for _, endpoint := range edgeEndpoints {
		// The entity type is a compile-time literal from edgeEndpoints and the
		// column an identifier from the same place; the id and the verb list are
		// bound. A NULL endpoint column matches no scrub row, which is how a kind
		// that does not occupy this column answers.
		ends = append(ends, "EXISTS ("+ScrubNewerThanRowSQL(
			"'"+endpoint.entityType+"'", "r."+endpoint.column, "a", verbs)+")")
	}

	var behind bool
	err := tx.QueryRow(ctx, `
		SELECT NOT (`+UnscrubbedImageSQL("a", verbs)+`)
		    OR EXISTS (
		         SELECT 1 FROM relationship r
		          WHERE r.id = a.entity_id
		            AND (`+strings.Join(ends, "\n			         OR ")+`))
		FROM audit_log a
		WHERE a.id = `+idPlaceholder+` AND a.entity_type = `+typePlaceholder,
		args...).Scan(&behind)
	if err != nil {
		return false, fmt.Errorf("privacy: whether link entry %s is behind an erasure: %w", auditID, err)
	}
	return behind, nil
}
