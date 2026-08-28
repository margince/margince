// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// extSchema is the one schema an extension's migrations may create in; the
// core owns public (backend/migrations/core/0213_ext_schema.up.sql).
const extSchema = "ext"

// coreTenantParent is the core table extension tables used to hang off. The
// role is granted nothing on it, or on anything else in public; the constant
// survives only so assertNoCorePrivileges can name the last exemption that was
// dropped when the tier stopped carrying a tenant column.
const coreTenantParent = "public.workspace"

// extRole is the restricted role a unit's migrations are applied as: the
// ext_<name> role a deployed installation would own its tables under.
type extRole struct {
	name string
	conn *pgx.Conn
}

// mintRole creates ext_<name> with the narrowest set of privileges a unit's
// migrations can possibly need, connects as it, and refuses to proceed if that
// role turns out to hold anything more.
//
// The privilege set IS the gate's teeth, so each piece is deliberate:
//
//   - NOSUPERUSER, NOBYPASSRLS. A superuser ignores every grant, so each
//     refusal below would be vacuous over such a connection. Asserted again
//     after connecting, because a cluster where the role already existed with
//     different attributes would otherwise turn this whole gate into a test of
//     nothing.
//
//   - CREATE, USAGE on ext and NOTHING on public. This is what converts "the
//     unit must not touch core" from something to detect into something
//     PostgreSQL refuses. It also makes an UNQUALIFIED create fail: the default
//     search_path is "$user", public, there is no schema named ext_<name>, and
//     the role cannot create in public — which is precisely the claim
//     gen-composition's textual rule makes when it accepts a bare
//     `CREATE TABLE ext_<name>_thing`.
//
//   - NOTHING ON PUBLIC AT ALL, which became reachable when the tier stopped
//     carrying a tenant column. The role held REFERENCES (id) on workspace for
//     exactly as long as a unit table was required to declare a foreign key
//     onto it; with no such key to declare there is no core object a unit's
//     migration may name, and PostgreSQL refuses the rest. The older shape,
//     kept here because the reasoning still binds anything that would restore
//     it: a key onto core is not an inert declaration — it takes a lock on core
//     writes and can refuse a core delete forever after, which
//     is what lets assertWorkspaceForeignKeys skip checking confkey.
//
//     Accepted residue: a foreign key is an existence oracle. A role holding it
//     learns from a constraint violation whether a given workspace UUID exists.
//     That is inherent to requiring the key at all and is not worth engineering
//     away — it leaks one bit about an opaque identifier the caller already had.
func mintRole(ctx context.Context, admin *pgx.Conn, namespace, dsn string) (minted *extRole, err error) {
	role := &extRole{name: namespace}

	password, err := randomPassword()
	if err != nil {
		return nil, err
	}

	// CREATE ROLE is the claim, and it is taken FIRST. A role is CLUSTER-scoped,
	// so a name derived from the unit is shared by every concurrent run on this
	// cluster, and the earlier shape here — count live sessions, then drop and
	// recreate — was a check followed by a take, which two runs can both pass
	// before either takes. CREATE cannot be won twice: PostgreSQL answers the
	// loser 42710, so exactly one run owns the name.
	//
	// A leftover from a dead run still has to be reclaimable, so a duplicate is
	// not the end of it — but reclaiming is the ONLY thing a duplicate permits,
	// and only after the role is shown to have no live session anywhere on the
	// cluster. The remaining window is small and named rather than closed: a run
	// that has created its role but not yet connected looks idle to a second run
	// arriving in those milliseconds. Closing it needs an ownership record the
	// cluster does not have (roles carry no creation time), and the cost of the
	// window is a failed CI step, not a wrong verdict.
	claimed, why, err := claimRole(ctx, admin, role.name, password)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, fmt.Errorf("role %s could not be claimed — %s; another extmigrategate run owns it, so re-run when it finishes", role.name, why)
	}
	// Armed only once the role EXISTS: before the claim there is nothing of
	// this run's to take back down, and dropping then would destroy the role of
	// whichever run actually holds it. After it, every failure has to drop —
	// a half-minted LOGIN role is the same standing credential as a leaked one.
	defer func() {
		if err == nil {
			return
		}
		closeQuietly(ctx, role.conn)
		if dropErr := role.drop(ctx, admin); dropErr != nil {
			err = fmt.Errorf("%w (and cleaning up afterwards: %w)", err, dropErr)
		}
	}()
	for _, statement := range []string{
		`GRANT CREATE, USAGE ON SCHEMA ` + extSchema + ` TO ` + role.name,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			return nil, fmt.Errorf("preparing the %s role (%s): %w", role.name, statement, err)
		}
	}

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing the throwaway DSN: %w", err)
	}
	config.User, config.Password = role.name, password
	if role.conn, err = pgx.ConnectConfig(ctx, config); err != nil {
		return nil, fmt.Errorf("connecting as %s: %w", role.name, err)
	}
	if err = role.assertRestricted(ctx); err != nil {
		return nil, err
	}
	return role, nil
}

// claimRole creates the role, reclaiming a leaked one if — and only if — no
// session anywhere on the cluster is using it. It reports whether this run now
// owns the name; false means another run does, and `why` says which of the two
// ways that was established — the two are a different thing for an author to
// read, and only one of them ("a live session") tells them to go look at what
// else is running.
//
// Dropped and recreated rather than adopted: a role left behind by an earlier
// run may have been granted anything since, and inheriting it would quietly
// weaken every refusal that rests on what this role cannot do.
func claimRole(ctx context.Context, admin *pgx.Conn, name, password string) (claimed bool, why string, err error) {
	create := `CREATE ROLE ` + name + ` LOGIN PASSWORD '` + password + `' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOREPLICATION`
	err = exec(ctx, admin, create)
	if err == nil {
		return true, "", nil
	}
	if !isDuplicateRole(err) {
		return false, "", fmt.Errorf("creating the %s role: %w", name, err)
	}

	var sessions int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM pg_stat_activity WHERE usename = $1`, name,
	).Scan(&sessions); err != nil {
		return false, "", fmt.Errorf("looking for live %s sessions: %w", name, err)
	}
	if sessions > 0 {
		return false, fmt.Sprintf("it already exists and has %d live session(s) on this cluster", sessions), nil
	}

	for _, statement := range []string{
		`DROP OWNED BY ` + name + ` CASCADE`,
		`DROP ROLE IF EXISTS ` + name,
	} {
		if err := exec(ctx, admin, statement); err != nil && !isMissingRole(err) {
			if isOwnedElsewhere(err) { //nolint:nestif // the two failure modes read better named than flattened
				// DROP OWNED BY reaches only the CURRENT database, while the
				// role is cluster-scoped: a run against another database on
				// this cluster that died before its cleanup leaves objects the
				// admin connection here cannot see, let alone drop. Say which
				// situation this is, because the statement's own message
				// ("cannot be dropped because some objects depend on it") reads
				// as a bug in this gate rather than as residue somewhere else
				// on the cluster.
				return false, "", fmt.Errorf("role %s still owns objects in ANOTHER database on this cluster, so it cannot be reclaimed here — "+
					"connect to that database and run `DROP OWNED BY %s CASCADE`, or point this gate at a cluster of its own: %w",
					name, name, err)
			}
			return false, "", fmt.Errorf("reclaiming the leaked %s role (%s): %w", name, statement, err)
		}
	}

	// The second CREATE is the same claim again, and losing it here means a
	// concurrent run took the name in the moment this one spent reclaiming it.
	// That is the loss reported, not an error: the other run owns it fairly.
	switch err := exec(ctx, admin, create); {
	case err == nil:
		return true, "", nil
	case isDuplicateRole(err):
		return false, "another run took the name while this one was reclaiming it", nil
	default:
		return false, "", fmt.Errorf("recreating the reclaimed %s role: %w", name, err)
	}
}

func exec(ctx context.Context, conn *pgx.Conn, statement string) error {
	_, err := conn.Exec(ctx, statement)
	return err
}

// assertRestricted proves the role is the restricted thing this gate assumes
// before a single migration runs. Without it, a cluster that had already
// granted the role something — or a Postgres old enough to still grant CREATE
// on public to PUBLIC — would let every migration pass for the wrong reason.
func (r *extRole) assertRestricted(ctx context.Context) error {
	var (
		super, bypass       bool
		createPublic        bool
		usagePublic         bool
		createExt, usageExt bool
	)
	if err := r.conn.QueryRow(ctx, `
		SELECT rolsuper, rolbypassrls,
		       has_schema_privilege('public', 'CREATE'),
		       has_schema_privilege('public', 'USAGE'),
		       has_schema_privilege($1, 'CREATE'),
		       has_schema_privilege($1, 'USAGE')
		  FROM pg_roles WHERE rolname = current_user`, extSchema,
	).Scan(&super, &bypass, &createPublic, &usagePublic, &createExt, &usageExt); err != nil {
		return fmt.Errorf("reading the %s role's privileges: %w", r.name, err)
	}
	switch {
	case super:
		// A superuser bypasses every permission check outright, including the
		// ACL checks assertNoCorePrivileges and assertNoWiderReferences run
		// below — so every refusal they would report is unearned.
		return fmt.Errorf("role %s holds rolsuper=true — a superuser is exempt from the ACL checks every refusal below rests on, so the gate would prove nothing", r.name)
	case bypass:
		// BYPASSRLS invalidates only row-level-security policy checks; it
		// leaves the ACL checks below fully enforced. The role is still
		// refused, because this tier's guarantee is that the runtime role
		// holds neither attribute — not because BYPASSRLS would undermine the
		// assertions that follow.
		return fmt.Errorf("role %s holds rolbypassrls=true — this tier's guarantee is that the role holds neither rolsuper nor rolbypassrls, "+
			"so it is refused even though BYPASSRLS invalidates only row-level-security policy checks and not the ACL checks below", r.name)
	case createPublic:
		return fmt.Errorf("role %s holds CREATE on schema public — the namespace wall is that it does not, and with it a migration can create a core-schema table that this gate would then have to detect rather than have refused", r.name)
	case !usagePublic:
		// Not a violation of the role's own shape, and no longer about a key a
		// unit declares — none may, and assertNoForeignKeysOutOfExt refuses one.
		// It is what makes the probes below MEAN anything: without USAGE the
		// role cannot name a core object at all, so "this role cannot reference
		// public.workspace" would be true of a cluster's schema permissions
		// rather than of the grant surface this gate exists to police, and the
		// gate would report a wall it never tested.
		return fmt.Errorf("role %s cannot USE schema public, so this gate cannot tell a role that "+
			"may not reach %s from a cluster where nobody may — grant USAGE on public to PUBLIC "+
			"here (CREATE stays revoked)", r.name, coreTenantParent)
	case !createExt || !usageExt:
		return fmt.Errorf("role %s lacks CREATE/USAGE on schema %s — migration 0206 creates that schema; is this database migrated to head?", r.name, extSchema)
	}
	return r.assertNoCorePrivileges(ctx)
}

// assertNoCorePrivileges proves the role can neither read nor write any core
// relation.
//
// Schema-level CREATE is what the checks above pin, and it is not the same
// question. The gate's refusal of a unit writing rows into public.person rests
// entirely on PostgreSQL denying the statement, which in turn rests on the role
// holding no privilege on that table. A single
// `GRANT INSERT ON ALL TABLES IN SCHEMA public TO PUBLIC` on the cluster would
// make that refusal quietly disappear while every test still passed, so the
// premise is asserted rather than assumed.
//
// (The example is described rather than quoted on purpose: the gates package's
// identity-spine fitness test greps the tree for the literal statement, and a
// comment carrying it reads to that gate as a new unsanctioned mint site.)
//
// TRIGGER is in the probe alongside the four DML verbs and TRUNCATE, because it
// is a write verb wearing another name: a role holding it can install a trigger
// function of its own on a core table, and from then on every core write runs
// that function. Nothing else in this gate would notice.
//
// REFERENCES is still probed separately, and now with no exemption at all: the
// gate grants none, so any column a unit's role can reference is a widening the
// cluster brought, not one this gate made. A foreign key onto core is not an
// inert declaration — it takes a lock on core writes and can refuse a core
// delete forever after.
func (r *extRole) assertNoCorePrivileges(ctx context.Context) error {
	var reachable string
	err := r.conn.QueryRow(ctx, `
		SELECT n.nspname || '.' || c.relname
		  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
		   AND (has_table_privilege(c.oid, 'SELECT') OR has_table_privilege(c.oid, 'INSERT')
		     OR has_table_privilege(c.oid, 'UPDATE') OR has_table_privilege(c.oid, 'DELETE')
		     OR has_table_privilege(c.oid, 'TRUNCATE') OR has_table_privilege(c.oid, 'TRIGGER'))
		 ORDER BY 1 LIMIT 1`).Scan(&reachable)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return fmt.Errorf("checking what %s can reach in public: %w", r.name, err)
	default:
		return fmt.Errorf("role %s can read, write or trigger on %s — this gate refuses a unit's DML on core relations by letting PostgreSQL deny it, and that denial does not happen on a cluster where the core tables are reachable", r.name, reachable)
	}
	return r.assertNoWiderReferences(ctx)
}

// assertNoWiderReferences proves the role can reference nothing in public.
//
// The probe is column-scoped, not table-scoped: has_table_privilege answers for
// the whole relation and returns false for a single-column grant, which would
// make a table-level probe blind to a widening on any one column.
func (r *extRole) assertNoWiderReferences(ctx context.Context) error {
	var reachable string
	err := r.conn.QueryRow(ctx, `
		SELECT n.nspname || '.' || c.relname || '(' || a.attname || ')'
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
		 WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
		   AND has_column_privilege(c.oid, a.attnum, 'REFERENCES')
		 ORDER BY 1 LIMIT 1`).Scan(&reachable)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("checking what %s can reference in public: %w", r.name, err)
	}
	return fmt.Errorf("role %s can declare a foreign key against %s — a unit takes no core dependency at all since %s(id) stopped being one, and a key onto core makes core deletes wait on, or refuse for, a unit's table", r.name, reachable, coreTenantParent)
}

// drop removes the role and everything it owns. A failure is returned rather
// than logged away: a login role left on the cluster owns the tables it just
// created and can rewrite or drop them, which is a standing credential, not a
// tidiness issue.
func (r *extRole) drop(ctx context.Context, admin *pgx.Conn) error {
	for _, statement := range []string{
		`DROP OWNED BY ` + r.name + ` CASCADE`,
		`REVOKE ALL ON TABLE ` + coreTenantParent + ` FROM ` + r.name,
		`DROP ROLE IF EXISTS ` + r.name,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil && !isMissingRole(err) {
			return fmt.Errorf("removing the %s role (%s): %w", r.name, statement, err)
		}
	}
	return nil
}

// undefinedObject is SQLSTATE 42704, which is what "role ... does not exist"
// arrives as. dependentObjectsStillExist is 2BP01, which is what a DROP ROLE
// blocked by objects the current database cannot see arrives as.
// duplicateObject is 42710, which is what "role ... already exists" arrives as
// — the losing side of the CREATE ROLE claim.
const (
	undefinedObject            = "42704"
	dependentObjectsStillExist = "2BP01"
	duplicateObject            = "42710"
)

// isDuplicateRole reports whether err is Postgres refusing a CREATE ROLE
// because the name is taken.
func isDuplicateRole(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == duplicateObject
}

// isOwnedElsewhere reports whether err is Postgres refusing to drop the role
// because something still depends on it. Inside this database DROP OWNED BY
// CASCADE has just run, so the only remaining source is another database on the
// same cluster.
func isOwnedElsewhere(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == dependentObjectsStillExist
}

// isMissingRole reports whether err is Postgres complaining that the role does
// not exist. DROP OWNED BY and REVOKE have no IF EXISTS spelling, so the first
// run on a clean cluster and the cleanup after a failed mint both hit this;
// any OTHER failure of those statements is real and must propagate.
func isMissingRole(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == undefinedObject
	}
	return false
}

// randomPassword mints the run's credential: hex of 16 random bytes, so there
// is nothing to quote inside the CREATE ROLE literal and nothing derivable
// from the repository.
func randomPassword() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("minting the extension role's password: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
