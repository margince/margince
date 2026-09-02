// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The binding between the subject-request queue and the Art. 15 export.
//
// consent owns the queue: which requests exist, who they name, when they are
// due. privacy owns the export: what the installation actually holds about a
// person and which of it Art. 15 owes back. Neither may import the other, so
// the edge is here — the same shape the erase path already uses, because
// answering an access request is the same species of act as answering an
// erasure one: the queue records the decision, another module carries it out.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectAccessSeam adapts privacy.AssembleSAR to the interface consent asks
// for.
//
// It serializes here rather than handing the struct across, because the package
// gains a section every time a new table starts holding subject data — a type
// mirrored in consent would be a second declaration of what Art. 15 owes, and it
// would drift one release behind the one piicoverage_test.go checks.
type subjectAccessSeam struct {
	db *database.DB
}

// newSubjectAccessAssembler binds the Art. 15 export over one database.
func newSubjectAccessAssembler(db *database.DB) *subjectAccessSeam {
	return &subjectAccessSeam{db: db}
}

// AssemblePackage answers the serialized Art. 15 package for one subject.
//
// Every authority check lives in AssembleSAR and none is repeated here: it takes
// the person.delete grant, refuses a non-human principal whatever its passport
// carries, and requires unbounded row scope because Art. 15 owes the subject
// everything held rather than the slice one colleague may see. A second copy of
// those checks here would be a second answer to who may read a subject's whole
// record.
func (s *subjectAccessSeam) AssemblePackage(ctx context.Context, personID ids.UUID) ([]byte, error) {
	pkg, err := privacy.AssembleSAR(ctx, s.db, ids.From[ids.PersonKind](personID))
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(pkg)
	if err != nil {
		return nil, fmt.Errorf("compose: serializing a subject-access package: %w", err)
	}
	return body, nil
}
