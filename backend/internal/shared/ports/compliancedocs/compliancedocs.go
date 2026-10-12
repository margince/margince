// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package compliancedocs is the core-internal registry of the documents packs
// ship. The contract lives on the published surface
// backend/pkg/extension/compliancedocs.
package compliancedocs

import (
	"fmt"
	"slices"
	"sync"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	pub "github.com/margince/margince/backend/pkg/extension/compliancedocs"
)

// Document is the published document shape, aliased.
type Document = pub.Document

var (
	mu   sync.RWMutex
	docs = map[jurisdiction.Code][]Document{}
)

// Register adds one jurisdiction's documents at boot. The compose preflight
// has already validated the set; the panic here is the backstop.
func Register(set []Document) {
	mu.Lock()
	defer mu.Unlock()
	if err := pub.ValidateSet(set); err != nil {
		panic(fmt.Sprintf("compliancedocs: %v", err))
	}
	for _, d := range set {
		for _, held := range docs[d.Jurisdiction] {
			if held.ID == d.ID {
				panic(fmt.Sprintf("compliancedocs: document %q registered twice for %q", d.ID, d.Jurisdiction))
			}
		}
		docs[d.Jurisdiction] = append(docs[d.Jurisdiction], d)
	}
}

// For returns a jurisdiction's documents in declaration order.
func For(code jurisdiction.Code) []Document {
	mu.RLock()
	defer mu.RUnlock()
	return slices.Clone(docs[code])
}
