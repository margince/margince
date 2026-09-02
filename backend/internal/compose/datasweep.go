// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a data reset does to Postgres: which tables it sweeps, the order it
// discovers at runtime, the outbox drain, the overlay-mode revert, and the
// cf_* column drop that runs on the owner pool afterwards. datareset.go holds
// the transport and the orchestration that calls these; datareset_runtime.go
// holds the non-Postgres surfaces.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// preservedResetTables are the tables a reset must NOT delete. Everything else
// in the public schema is domain/config data: swept, then re-seeded.
//
// This list is the whole definition: what a reset must KEEP is a decision
// someone has to make, and what it deletes follows. A derivation narrowed by
// any column instead would silently stop sweeping a table that dropped it, and
// the reset would then fail re-seeding rows it had not deleted.
//
// Four kinds are kept:
//
//   - identity and auth, so every user (the admin above all) stays logged in
//     and the installation survives its own reset;
//   - the append-only ledgers, whose immutability trigger forbids DELETE and
//     whose operational history should outlive a data reset;
//   - installation configuration and secrets, which are not this workspace's
//     records — a reset that cleared them would leave an installation unable
//     to reach the providers and mailboxes it is still configured for;
//   - the job runtime, which is River's to manage and not ours to truncate
//     underneath a running worker.
var preservedResetTables = map[string]bool{
	// identity and auth
	objectWorkspace: true, "app_user": true, "role": true, "role_assignment": true,
	"team": true, "team_membership": true,
	"session": true, "passport": true, "auth_token": true,
	// append-only ledgers
	"audit_log": true, "system_log": true,
	// installation configuration and secrets
	"setting": true, "vault_secret": true, "ai_call_config": true,
	"embed_store_binding": true,
	// The installation's system-of-record mode, on the same footing as
	// `setting`: one row a migration seeds, not a record of anybody's
	// customers. The sweep must not DELETE it — overlay.RevertToNative runs
	// after the sweep and returns it to native, and an UPDATE against a row the
	// sweep had removed would touch nothing, report "not reverted", and leave
	// the installation with no mode at all for the dispatcher to read.
	"overlay_mode": true,
	// The derived channel-provider registry: installation-global reference data,
	// not this workspace's records, on the SAME footing as `setting` above — a
	// reset that cleared it would leave the installation unable to recognise the
	// connectors it has compiled in, and the sweep's unconditional,
	// non-tenant-scoped DELETE would hit the activity_kind_fkey and
	// activity_channel_provider_fkey constraints and abort the whole sweep
	// transaction outright.
	"activity_kind": true, "channel_provider": true,
	// The two administered lead vocabularies — sources and disqualification
	// reasons — are installation configuration on the same footing as
	// `setting`: the migration seeds the built-ins and an administrator shapes
	// the rest, and neither is a record of this workspace's customers.
	"lead_source": true, "lead_disqualify_reason": true,
	// How many minor units each currency has, where that is not two: ISO
	// reference data a migration seeds, not anybody's records. Two reasons it
	// is here rather than swept and re-seeded, and either alone is sufficient.
	// The rows are the SQL mirror of values.currencyMinorDigits, so a reset
	// that emptied them would leave every foreign conversion in SQL scaling at
	// two digits — right for most currencies, a hundredfold wrong for a yen
	// one, and invisible. And the sweep runs as the application role, which
	// holds SELECT alone on this table by design, so the DELETE is refused
	// outright and aborts the whole reset transaction.
	"currency_minor_digits": true,
	// in-flight delivery: drained by the outbox pass, not deleted under it
	"event_outbox": true,
	// The retention floor's evidence (A165, migration 0289). Preserved from the
	// sweep's own DELETE because a direct one is REFUSED outright: the row goes
	// only with the activity it substantiates, through the FK's CASCADE, and
	// its guard tells the two apart by whether the parent is already gone.
	// Sweeping it directly would abort the reset transaction on the first row.
	//
	// It is still cleared by a reset — `activity` is swept, and the cascade takes
	// the evidence with it. Preserved here means "not a target", never "kept".
	"activity_retention_evidence": true,
}

// resetTargetTables lists every public base table a reset sweeps: all of them,
// less the preserved set above, the migration ledgers, and River's own schema.
// Derived from the catalog so a newly added table is swept automatically rather
// than escaping a hand-kept list — the burden of naming falls on what must be
// KEPT, where forgetting an entry fails loudly (the admin loses their session,
// the installation loses its config) instead of quietly leaving a tenant's rows
// behind after a reset that reported success.
func resetTargetTables(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND c.relname NOT LIKE 'schema_migrations%'
		  AND c.relname NOT LIKE 'river_%'
		ORDER BY c.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !preservedResetTables[name] {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// sweepWorkspaceData deletes every row of the target tables. Running as the non-superuser app role, it cannot disable FK
// triggers, so it discovers a safe order at runtime: each pass tries every
// still-populated table behind a savepoint and defers the ones a child FK still
// blocks to the next pass, until all are clear. A pass with no progress means an
// unbreakable FK cycle — surfaced explicitly, never silently skipped.
func sweepWorkspaceData(ctx context.Context, tx pgx.Tx, tables []string) error {
	remaining := append([]string(nil), tables...)
	for len(remaining) > 0 {
		var stuck []string
		progressed := false
		for _, t := range remaining {
			if _, err := tx.Exec(ctx, "SAVEPOINT reset_sp"); err != nil {
				return err
			}
			_, delErr := tx.Exec(ctx, `DELETE FROM `+pgx.Identifier{t}.Sanitize())
			if delErr == nil {
				if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT reset_sp"); err != nil {
					return err
				}
				progressed = true
				continue
			}
			if !storekit.IsForeignKeyViolation(delErr) {
				return delErr
			}
			if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT reset_sp"); err != nil {
				return err
			}
			// A rollback leaves the savepoint defined but unusable for a
			// subsequent SAVEPOINT of the same name until it's released —
			// without this, repeated passes over a slow-to-clear table
			// would pile up shadowed savepoints for the life of the tx.
			if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT reset_sp"); err != nil {
				return err
			}
			stuck = append(stuck, t)
		}
		if !progressed {
			return fmt.Errorf("data reset: unresolved foreign-key cycle among %v", stuck)
		}
		remaining = stuck
	}
	return nil
}

// clearWorkspaceOutbox removes the staged events, so the reset does not leave
// events pointing at rows it just deleted.
//
// event_outbox is infra-owned and has no workspace_id column. Tenancy used to
// live in the envelope, and the delete matched on it; the envelope carries no
// tenant now (ADR-0091 §6), and under one installation there is no other
// tenant's event to spare — every staged row belongs to the installation being
// reset.
func clearWorkspaceOutbox(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `DELETE FROM event_outbox`)
	return err
}

// clearOutbox drains the staged events in a transaction of its OWN, before the
// stream purge, rather than inside the sweep's transaction after it.
//
// The relay is not the job fleet: quiescing the queues does not stop it shipping
// event_outbox rows onto the streams. Staged rows left in place while the
// streams are purged are re-published seconds later into streams that were just
// emptied, and a subscriber then works events against rows the sweep is deleting.
//
// It narrows the window rather than closing it. The relay claims a batch FOR
// UPDATE SKIP LOCKED, so this DELETE waits on whatever is in flight — but that
// batch has already XADDed, and rows any concurrent request commits afterwards
// still reach the bus. The residual is one relay batch wide, not seconds wide.
//
// Running in its own transaction has a price the sweep's did not: a reset that
// fails later has already dropped this workspace's staged events, and they are
// never delivered.
func (h dataResetHandlers) clearOutbox(ctx context.Context) error {
	return database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
		return clearWorkspaceOutbox(ctx, tx)
	})
}

// credentialHandleSuffix is how a column that holds a keyvault handle is
// spelled in this schema — `credential_ref` on the connection tables,
// `vault_ref` on extension_secret, `signing_secret_ref` on
// webhook_subscription.
//
// A SUFFIX and not a name, because the name was wrong. This used to be the
// single string "credential_ref", above a comment calling it "the one spelling
// every table uses". There were three, so extension_secret's and
// webhook_subscription's ciphertext was never collected and outlived every
// reset — resident and unreachable, which is the exact failure
// collectWorkspaceSecretRefs' comment says it exists to prevent.
//
// The suffix over-matches on purpose: thirteen distinct `_ref` column names
// exist in this schema and only three are handles. Over-matching costs nothing
// because the VAULT decides — a candidate value is collected only if
// vault_secret holds it, so an `evidence_ref` or a `pdf_asset_ref` contributes
// nothing. Under-matching is what cost something, and it is the direction a
// derivation like this must not fail in.
const credentialHandleSuffix = "_ref"

// collectWorkspaceSecretRefs reads every sealed-credential handle in the
// installation, BEFORE the sweep deletes the rows that name them.
//
// It has to run first because vault_secret is deliberately not swept: it
// carries no RLS (migrations/core/0062), since the tenant lives inside the ref
// and inside the AES-256-GCM AAD. The sweep therefore never sees it, and a
// reset that did not collect these first would leave the ciphertext resident
// and unreachable forever — credential material outliving the wipe that was
// supposed to clear it.
//
// The join to vault_secret is what makes the wide column match safe, and it is
// also what makes the collection EXACT: a handle is a value the vault holds,
// which is the vault's own answer rather than this file's guess about naming.
func collectWorkspaceSecretRefs(ctx context.Context, tx pgx.Tx) ([]string, error) {
	columns, err := credentialHandleColumns(ctx, tx)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, c := range columns {
		table, column := pgx.Identifier{c.table}.Sanitize(), pgx.Identifier{c.column}.Sanitize()
		rows, err := tx.Query(ctx,
			`SELECT h.`+column+` FROM `+table+` h
			 JOIN vault_secret v ON v.ref = h.`+column+`
			 WHERE h.`+column+` IS NOT NULL`)
		if err != nil {
			return nil, fmt.Errorf("data reset: reading credential handles from %s.%s: %w", c.table, c.column, err)
		}
		for rows.Next() {
			var ref string
			if err := rows.Scan(&ref); err != nil {
				rows.Close()
				return nil, fmt.Errorf("data reset: reading a credential handle from %s.%s: %w", c.table, c.column, err)
			}
			refs = append(refs, ref)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("data reset: reading credential handles from %s.%s: %w", c.table, c.column, err)
		}
	}
	return refs, nil
}

// likeEscaped makes a literal safe as a LIKE pattern, using `!` as the escape
// character rather than a backslash.
//
// `_` is a LIKE WILDCARD, so the suffix went in as "any character followed by
// ref" — which finds `href` and `pref` as readily as `_ref`. Harmless only
// because the vault decides what is a handle, and that is not a reason to ask
// the wrong question.
//
// `!` and not `\`: a backslash has to survive a Go string, an SQL string and
// LIKE itself, and Postgres rejects an ESCAPE that is not exactly one
// character — which is what the first attempt produced. `!` cannot appear in a
// pg_attribute name, so nothing needs escaping that is not being escaped here.
func likeEscaped(s string) string {
	return strings.NewReplacer("!", "!!", "_", "!_", "%", "!%").Replace(s)
}

// handleColumn is one place a keyvault handle can be written down.
type handleColumn struct{ table, column string }

// credentialHandleColumns lists them, derived from the catalog for the same
// reason resetTargetTables is: a new one enrols itself. It asks only about the
// handle column — a derivation narrowed by any other column collects a subset
// and leaves sealed credentials resident after a reset that reported success.
//
// vault_secret itself is excluded: its `ref` IS the handle rather than a
// reference to one, and joining the table to itself would collect every secret
// in the installation whether or not anything still points at it.
func credentialHandleColumns(ctx context.Context, tx pgx.Tx) ([]handleColumn, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	suffixPos := arg("%" + likeEscaped(credentialHandleSuffix))
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.relname, a.attname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid
		JOIN pg_type t ON t.oid = a.atttypid
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND c.relname <> 'vault_secret'
		  AND a.attname LIKE %s ESCAPE '!'
		  AND t.typname IN ('text', 'varchar')
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY c.relname, a.attname`, suffixPos), args...)
	if err != nil {
		return nil, fmt.Errorf("data reset: listing columns that can hold a credential handle: %w", err)
	}
	defer rows.Close()
	var out []handleColumn
	for rows.Next() {
		var c handleColumn
		if err := rows.Scan(&c.table, &c.column); err != nil {
			return nil, fmt.Errorf("data reset: listing columns that can hold a credential handle: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// dropResetCustomFieldColumns drops every runtime cf_* column via the owner
// pool (the ONE sanctioned ALTER TABLE chokepoint). DROP COLUMN CASCADE also
// removes each column's generated cf_<slug>_check constraint.
func dropResetCustomFieldColumns(ctx context.Context, schemaPool *pgxpool.Pool) error {
	rows, err := schemaPool.Query(ctx, `
		SELECT quote_ident(table_name), quote_ident(column_name)
		FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name LIKE 'cf\_%'`)
	if err != nil {
		return err
	}
	type col struct{ table, name string }
	var cols []col
	for rows.Next() {
		var c col
		if err := rows.Scan(&c.table, &c.name); err != nil {
			rows.Close()
			return err
		}
		cols = append(cols, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range cols {
		if _, err := schemaPool.Exec(ctx, `ALTER TABLE `+c.table+` DROP COLUMN `+c.name+` CASCADE`); err != nil {
			return err
		}
	}
	return nil
}
