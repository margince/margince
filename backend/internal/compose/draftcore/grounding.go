// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Ground is one record a draft's input was folded from, its id spelled as the
// input carries it.
type Ground struct {
	Type datasource.EntityType
	ID   string
}

// Grounding spells what a surface's input names as references a caller can
// re-read. Each surface still decides which records those are; this only keeps
// an unparseable id from passing as a record nobody can re-check.
func Grounding(named []Ground) ([]datasource.EntityRef, error) {
	out := make([]datasource.EntityRef, 0, len(named))
	for _, ground := range named {
		id, err := ids.Parse(ground.ID)
		if err != nil {
			return nil, fmt.Errorf("draft grounding: %s id %q: %w", ground.Type, ground.ID, err)
		}
		out = append(out, datasource.EntityRef{Type: ground.Type, ID: id})
	}
	return out, nil
}
