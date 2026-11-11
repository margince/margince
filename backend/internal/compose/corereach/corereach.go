// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package corereach builds the set of tables the merge and the erasure reach
// on their owners' behalf (sweepreach): every core module's declaration plus
// every composed unit's. Its own package so the boot, the integration harness
// and the gates read one list.
package corereach

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/sweepreach"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

// Tables returns the core declarations, each with its owning module.
func Tables() []sweepreach.Owned {
	var set []sweepreach.Owned
	for _, module := range []struct {
		owner  string
		tables func() []reach.Table
	}{
		{"commissions", commissions.SweepReach},
		{"finance", finance.SweepReach},
		{"weeklyplan", weeklyplan.SweepReach},
	} {
		for _, t := range module.tables() {
			set = append(set, sweepreach.Owned{Owner: module.owner, Table: t})
		}
	}
	return set
}

// Install seals the core declarations and every unit's, refusing a unit table
// outside the unit's own namespace. It is the only production caller of
// sweepreach.Install, so a sealed set always carries the core declarations.
func Install(exts []extension.Extension) error {
	set := Tables()
	for _, e := range exts {
		namespace, err := e.Name.Namespace()
		if err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		for _, t := range e.Reach {
			if !strings.HasPrefix(t.Name, namespace+"_") {
				return fmt.Errorf("compose: extension %q declares reach into %q, which is not one of its own %s_ tables", e.Name, t.Name, namespace)
			}
			set = append(set, sweepreach.Owned{Owner: string(e.Name), Schema: "ext", Table: t})
		}
	}
	return sweepreach.Install(set)
}
