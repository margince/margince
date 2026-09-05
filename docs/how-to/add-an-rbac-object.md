# Add an RBAC object

For introducing a new kind of record into the permission model — a new name in
the closed set of objects a role document may grant. For adding an operation to
an existing module, use [add-an-endpoint.md](add-an-endpoint.md); for the whole
capability, [add-a-module.md](add-a-module.md). What the finished vocabulary
looks like: [reference/rbac-matrix.md](../reference/rbac-matrix.md); why it works
this way: [explanation/rbac-roles-and-teams.md](../explanation/rbac-roles-and-teams.md).

The change spans **five places**, and skipping any one of them fails in a
different way. Two of them are held by merge-blocking gates, so you will find out
before you push. The migration (step 3) — the backfill — has **no compile-time or
review-time signal at all** on a fresh database, and getting it wrong 403s every
existing installation forever.

## 1. Add the object to the policy vocabulary

In `backend/internal/modules/identity/internal/policy/policy.go`:

1. Append the name to `coreObjects`. This is the closed set `Parse` validates
   against, so nothing outside it can ever be granted.
2. In `defaults.go`, decide for **each of the six system roles** — `admin`,
   `management`, `manager`, `rep`, `read_only`, `ops` — whether the new object
   matches that role's baseline. If it does, there is nothing to write: `grid`
   gives every core object the base. If it does not, add one line naming the
   object and its grant to that role's override map.

   **Silence is now the default, and that is the trap.** The old positional form
   made a forgotten role a compile error — an argument was missing. `grid` gives
   that role the base instead and tells nobody. So decide for all six
   deliberately, and confirm the result in the regenerated matrix at step 4;
   that page is the only thing that will tell you a role inherited a grant you
   never thought about.

**Nothing is positional.** `grid(base, overrides)` seeds every object in
`coreObjects` with the base and then applies the overrides by NAME, so the
object a grant governs is written beside it. An override naming an object that
does not exist panics at package init rather than being ignored, which is what
makes a typo a build failure instead of a role that silently governs nothing.

A role's override map is therefore the list of places that role DEPARTS from its
own posture, which is the sentence a reviewer wants: not "what does rep hold on
all 44 objects" but "where is rep not the record posture". `managerObjects` is
still one variable shared by `manager` and `management` so the two grids cannot
drift; only their row scope differs.

Pick the posture from an existing precedent rather than inventing one — the
comment block above `defaults` records the reasoning for each family (record
posture, pipeline-config posture, admin/ops-owned config), and matching one of
them is how a reviewer checks your choice.

## 2. Add the object to the contract enum

Add the same string to `RbacObject` in `backend/api/crm.yaml`.

**The server derives nothing from it.** `oapi-codegen` emits no Go constants for
a top-level standalone string enum, so `policy.coreObjects` is maintained by hand
and this enum is maintained by hand. What the enum *does* buy is the web client:
`openapi-typescript` renders it as a string union, so a capability check against
a misspelled object is a TypeScript error rather than a check that compiles and
silently denies forever.

What keeps the two halves equal is a merge-blocking parity test,
**`TestContractObjectEnumMatchesPolicyVocabulary`** in
`backend/gates/rbacvocabulary_test.go`. Both sides are derived — the object list is
AST-parsed out of `policy.go` by `coreObjectsFromSource`
(`backend/gates/rbacvocabularysource_test.go`), the contract side is read from the YAML
— so the test never becomes a third place to keep current. Editing the enum alone
changes what clients can *express*, never what the server *enforces*.

## 3. Write the backfill migration — this is the step that bites

**Role seeding runs once, at workspace creation, and never re-syncs.**
`identity.seedSystemRoles` writes `policy.MustDefaultJSON(role.key)` into
`role.permissions` when the workspace is bootstrapped
(`backend/internal/modules/identity/service.go`), and no code path ever reconciles
a stored document against the compiled-in defaults afterwards. Authentication
reads the *stored* document: `loadGrants` selects `role.permissions` and merges
it. So an object added to `coreObjects` without a migration is granted to nobody
who bootstrapped earlier. It works on your fresh database and 403s everywhere
else, permanently — which is exactly how `saved_view`, `webhook_subscription`,
`relationship`, `partner`, `list` and `tag` reached production ungranted.

Write the pair in `backend/migrations/core/` following
[apply-migrations.md](apply-migrations.md) for numbering and lane conventions,
and copy the shape of the most recent RBAC backfill,
**`0183_list_tag_rbac.up.sql`**:

- One `UPDATE role SET permissions = jsonb_set(permissions, '{objects,<name>}', '<grant>'::jsonb)`
  per distinct grant, grouped by the roles that share it.
- Guard every statement with `WHERE is_system AND ... AND NOT permissions->'objects' ? '<name>'`.
  The only-if-absent guard is what makes the migration free where it is not
  needed and non-destructive where an operator has already edited a role.
- The grants must reproduce, exactly, what step 1 seeds — the replay in step 6
  compares the upgraded end state against the seeded matrix, verb by verb.

**The `down` is a documented no-op.** Reversing the schema does not remove the
object from `policy.coreObjects`, so deleting the grant on rollback cannot
restore an earlier correct state — it can only recreate the permanent 403 the
migration exists to fix, and it would do so on workspaces that legitimately held
the grant already (where the up's guard wrote nothing, and left no trace
distinguishing those rows from the ones it did write). A forward-only data repair
has no meaningful inverse. `0183_list_tag_rbac.down.sql` says exactly this, and
is the file to copy.

### The typo that locks people out of login

`policy.Parse` **rejects** an unknown object key:

```go
for object := range doc.Objects {
    if !IsCoreObject(object) {
        return Document{}, fmt.Errorf("policy: unknown object %q in permissions document", object)
    }
}
```

`Parse` is called from `loadGrants`, which runs on the **login** path, on session
resolution, and on the agent-authority re-derivation in `identity/authority.go`.
A role carrying an invalid document is treated as a data defect to surface, not
as a reason to silently downgrade to no access — so the whole authentication
fails.

That makes a typo in the migration's JSON path far worse than a missing grant. If
you write `'{objects,webook_subscription}'`, the migration succeeds, the object is
never granted, **and every user holding that role can no longer log in** — the
document now names an object outside the closed set. Spell the path from the same
string literal you added to `coreObjects`, and prove it by running the integration
lane in step 6 rather than by reading it twice — a typo'd path leaves the object
ungranted, which is what the convergence arms fail on.

## 4. Regenerate the published matrix

[reference/rbac-matrix.md](../reference/rbac-matrix.md) is generated from the
seeded documents. From `backend/`:

```bash
go test ./internal/modules/identity/ -run RBAC -update
```

The same test — **`TestPublishedRBACMatrixMatchesTheSeededRoleDocuments`** in
`backend/internal/modules/identity/rbacmatrix_test.go` — runs on every build
*without* `-update` and fails when the page and the seeded values disagree, so the
two cannot drift apart unnoticed. Read the regenerated row: it renders each
role's grant as `CRUD` letters, which is the resolved answer rather than the
base-plus-overrides you wrote — so it is where you confirm that the role you did
NOT write a line for inherited the grant you meant it to.

## 5. Gate the store, then the UI

**Server side.** Every exported method on the owning `*Store` or `*Service` that
touches the new object calls `auth.Require(ctx, "<object>", principal.ActionX)`,
plus `auth.EnsureVisible` / `auth.ScopeClauseFor` for row scope — see
[reference/platform-toolkit.md](../reference/platform-toolkit.md#platformauth--the-admission-point).
This is not optional politeness: `TestEveryStoreEntryPointIsAuthGated`
(`backend/gates/rbacgate_test.go`) derives the entry-point set from the tree and fails
an ungated one, because an ungated store method is a door into tenant data that
is reachable by any transport wired to it and invisible to review.

**Client side.** Bind the affordance to the **grant**, never to a role name. The
hooks are in `frontend/src/app/capability.ts` and take the generated `RbacObject`
union, so a misspelled object is a compile error:

| Hook | Use it for |
|---|---|
| `useCan(object, action)` | One specific request. Object RBAC only — no seat ceiling. |
| `useCanWrite(object, action)` | A control that issues a **mutating** request (the common case): grant ∧ seat. |
| `useCanUpsert(object)` | A control whose endpoint inserts *or* replaces, so the needed verb is not knowable client-side. |
| `useHoldsWriteGrant(object)` | An authoring *surface* — a nav entry, a section heading — where any write verb justifies showing it. |
| `useCanMutate()` | The licensing seat ceiling alone. |

The answers come from the server (`GET /me` carries the merged grants it
computed) and only the vocabulary comes from the contract, which is what keeps
the client honest on a workspace whose stored grants have drifted. This is UX
honesty, never enforcement — the server's `auth.Require` is the authority on
every call, and a client that gets it wrong shows the wrong button, not the wrong
data.

## 6. Verify

Run both lanes, and know which one proves what:

**`make check`** — the merge gate.

- `TestContractObjectEnumMatchesPolicyVocabulary` — the contract enum and
  `coreObjects` are the same set. Catches step 2 missing, or misspelled.
- `TestPublishedRBACMatrixMatchesTheSeededRoleDocuments` — the published page
  matches the seeded documents. Catches step 4 not run.
- `TestEveryStoreEntryPointIsAuthGated` — no ungated store entry point. Catches
  step 5 missing on the server.
- `make check-fe` — the TypeScript build proves the new object is expressible in
  the capability hooks.

**`make test-integration`** — the real-Postgres lane, and **the only thing that
proves the backfill landed on an old install**. All three gates live in
`backend/internal/compose/integration/rbacseedparity_integration_test.go` and
they EXECUTE the obligation rather than scanning for it, which is why no list of
objects and no grep for `'{objects,<name>}'` decides any of them:

- `TestTheRealBootstrapSeedsTheDocumentedMatrix` — the real bootstrap writes the
  documented matrix. Catches step 1 or step 4 not landing on a fresh install.
- `TestEveryRBACBackfillConvergesOnTheSeededMatrix` — each backfill, replayed
  against today's matrix minus its own objects, converges back onto the matrix.
  This is the arm that ISOLATES: a failure names your migration. It is where a
  typo'd jsonb path from step 3, a wrong verb, or a `WHERE` clause that matches
  no rows shows up.
- `TestTheBackfillsComposeFromTheOldestUpgradableInstallation` — every backfill
  replayed in version order over the documents an installation bootstrapped at
  the migration baseline really held. **This is the arm that catches step 3
  missing entirely**, for any object added since that baseline. The isolating arm
  cannot: it derives its starting state from the migrations, so an object no
  migration mentions is never absent from that state and its missing backfill is
  invisible.

The composed arm's starting state is a committed fixture,
`backend/migrations/testdata/rbac_baseline_era_defaults.json` — and the reason it
is safe to commit is a second gate, not the file itself. Editing that fixture is
exactly how a backfill that does not work is made to look like one that does: move
the object into the starting state and the convergence it never delivered is
already there. So `TestBaselineEraFixtureIsTheMatrixTheBaselineSeeded`
(`backend/gates/rbacbaselineerafixture_test.go`, unit lane) pins it to
`git show <baseline>:backend/migrations/testdata/rbac_seeded_defaults.json` —
compared as decoded JSON, so re-indentation is not a difference but any changed
key or value is — and
proves that commit really is the consolidation floor rather than a commit somebody
named — otherwise the pin could be moved forward instead of the fixture being
edited, with the same effect. Regenerate it with the command that gate's failure
message prints; never by hand.

It lives in the unit lane because reading history needs a full checkout, and the
integration shards check out shallow.

Commit the policy change, the contract, the migration pair, the regenerated
matrix and the UI binding together — they are one change, and any one of them
alone is a broken state.
