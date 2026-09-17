// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Ready means the schema this binary was built for is there.
//
// A composed binary started against a database whose migrations have not been
// applied used to become ready and publish routes and jobs that then failed
// with undefined-table errors. It was noticed on the extension tier — a unit's
// handler selecting from ext.ext_<unit>_… answers 500 — but nothing about it
// is special to extensions, so core and custom are asked the same question.
//
// UNREADY rather than a refusal to boot. A process that will not start removes
// the operator's ability to roll forward and turns a degraded deploy into a
// failed one. Unready is the honest state: the load balancer stops sending
// traffic, no route serves a wrong answer, and applying the migration clears
// it without a restart.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// SchemaAtHead is the /readyz probe over migration state: the versions this
// binary ships against the versions the database records, for core, custom and
// every composed unit that owns tables.
//
// Exported because both serving roles need it and neither may be the one that
// has it: the api mounts it beside its other probes, and the worker — whose
// dispatcher tick hits the same missing tables on its own cadence, with no
// request to fail — assembles its own check list in cmd/worker.
//
// The namespaces are assembled ONCE, when the check is built, and the database
// is what is re-read on each scrape. They come from this binary's embedded
// files and its composed set, neither of which can change while it runs, and
// re-loading them per scrape would spend a migration parse on a health check.
func SchemaAtHead(pool *pgxpool.Pool) func(context.Context) error {
	namespaces, err := composedNamespaces()
	if err != nil {
		// A binary that cannot read its own embedded migrations cannot say
		// which schema it needs, so it cannot claim the database has it. The
		// failure is reported as unready for the same reason a missing
		// migration is: the process keeps running, the operator sees why, and
		// nothing serves an answer it cannot stand behind.
		return func(context.Context) error {
			return fmt.Errorf("reading this binary's own migration set: %w", err)
		}
	}
	return func(ctx context.Context) error {
		short, err := dbmigrate.Pending(ctx, pool, namespaces...)
		if err != nil {
			return err
		}
		if len(short) == 0 {
			return nil
		}
		return fmt.Errorf("the database is behind this binary: %s — apply the migrations (cmd/migrate up); "+
			"this process serves again without a restart once they land", describeShortfalls(short))
	}
}

// composedNamespaces gathers the migration namespaces this binary carries: the
// two embedded ones and one per composed unit that owns tables.
func composedNamespaces() ([]dbmigrate.Namespace, error) {
	core, err := migrations.Core()
	if err != nil {
		return nil, err
	}
	custom, err := migrations.Custom()
	if err != nil {
		return nil, err
	}
	// ComposedExtensions is this boot's registered set, the same snapshot the
	// routes and jobs were mounted from — so the check asks about exactly the
	// units that are serving, never a set read a second time from somewhere
	// else.
	exts, err := dbmigrate.ExtensionNamespaces(ComposedExtensions())
	if err != nil {
		return nil, err
	}
	return append([]dbmigrate.Namespace{core, custom}, exts...), nil
}

// describeShortfalls names every namespace that is behind and what it is
// missing, because an operator reading a readiness failure has to know WHICH
// unit to migrate — and because a message naming only the first would report
// one defect per scrape on a database missing several.
func describeShortfalls(short []dbmigrate.Shortfall) string {
	parts := make([]string, 0, len(short))
	for _, s := range short {
		if s.Untracked {
			parts = append(parts, fmt.Sprintf("%s was never applied here (no schema_migrations_%s), %d version(s) outstanding",
				s.Namespace, s.Namespace, len(s.Missing)))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s is missing %s", s.Namespace, strings.Join(s.Missing, ", ")))
	}
	return strings.Join(parts, "; ")
}
