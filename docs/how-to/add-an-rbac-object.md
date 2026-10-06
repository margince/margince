# Add an RBAC object

For introducing a new kind of record into the permission model: a new name in
the closed set of objects a role document may grant. For adding an operation to
an existing module, use [add-an-endpoint.md](add-an-endpoint.md); for the whole
capability, [add-a-module.md](add-a-module.md). What the finished vocabulary
looks like: [reference/rbac-matrix.md](../reference/rbac-matrix.md); why it works
this way: [explanation/rbac-roles-and-teams.md](../explanation/rbac-roles-and-teams.md).

The change spans **five places**, and skipping any one of them fails in a
different way. Merge-blocking gates hold two of them, so you will find out
before you push. The backfill migration (step 3) gives **no compile-time or
review-time signal at all** on a fresh database, and getting it wrong 403s every
existing installation forever.

## 1. Add the object to the policy vocabulary

In `backend/internal/modules/identity/internal/policy/policy.go`:

1. Append the name to `coreObjects`. This is the closed set `Parse` validates
   against, so nothing outside it can ever be granted.
2. In `defaults.go`, decide for **each system role** (`admin`, `management`,
   `manager`, `rep`, `read_only`, `ops`) whether the new object matches that
   role's baseline. If it does, there is nothing to write: `grid` gives every
   core object the base. If it does not, add one line naming the object and its
   grant to that role's override map.

   An object you do not override gets the role's base grant, with no warning.
   Decide for every role and check the result in the matrix at step 4; that page
   is the only thing that shows a role inherited a grant you never thought about.

**Nothing is positional.** `grid(base, overrides)` seeds every object in
`coreObjects` with the base and then applies the overrides by name, so the
object a grant governs is written beside it. An override naming an object that
does not exist panics at package init, so a typo is a build failure instead of a
role that governs nothing.

A role's override map is therefore the list of places that role departs from its
own posture. A reviewer reads "where is rep not the record posture" instead of
"what does rep hold on every object". `managerObjects` is one variable shared by
`manager` and `management` so the two grids cannot drift; only their row scope
differs.

Pick the posture from an existing precedent instead of inventing one. The
comment block above `defaults` records the reasoning for each family (record
posture, pipeline-config posture, admin/ops-owned config), and matching one of
them is how a reviewer checks your choice.

## 2. Add the object to the contract enum

Add the same string to `RbacObject` in `backend/api/crm.yaml`.

**The server derives nothing from it.** `oapi-codegen` emits no Go constants for
a top-level standalone string enum, so `policy.coreObjects` and this enum are
both maintained by hand. The enum serves the web client: `openapi-typescript`
renders it as a string union, so a capability check against a misspelled object
is a TypeScript error instead of a check that compiles and denies forever.

A merge-blocking parity test keeps the two halves equal:
**`TestContractObjectEnumMatchesPolicyVocabulary`** in
`backend/gates/rbacvocabulary_test.go`. Both sides are derived: the object list is
AST-parsed out of `policy.go` by `coreObjectsFromSource`
(`backend/gates/rbacvocabularysource_test.go`), and the contract side is read from
the YAML, so the test never becomes a third place to keep current. Editing the
enum alone changes what clients can *express*, never what the server *enforces*.

## 3. Write the backfill migration (the step that bites)

Role seeding runs **once**, at workspace creation, and never re-syncs.
`identity.seedSystemRoles` writes `policy.MustDefaultJSON(role.key)` into
`role.permissions` when the workspace is bootstrapped
(`backend/internal/modules/identity/service.go`), and no code path reconciles a
stored document against the compiled-in defaults afterwards. Authentication
reads the *stored* document: `loadGrants` selects `role.permissions` and merges
it. So an object added to `coreObjects` without a migration is granted to nobody
who bootstrapped earlier. It works on your fresh database and 403s everywhere
else, permanently.

Write the pair in `backend/migrations/core/` following
[apply-migrations.md](apply-migrations.md) for numbering and lane conventions,
and copy the shape of the most recent RBAC backfill in `backend/migrations/core/`
(`ls *_rbac.up.sql | tail -1`):

- One `UPDATE role SET permissions = jsonb_set(permissions, '{objects,<name>}', '<grant>'::jsonb)`
  per distinct grant, grouped by the roles that share it.
- Guard every statement with `WHERE is_system AND ... AND NOT permissions->'objects' ? '<name>'`.
  The only-if-absent guard makes the migration free where it is not needed and
  non-destructive where an operator has already edited a role.
- The grants must reproduce what step 1 seeds. The replay in step 6 compares the
  upgraded end state against the seeded matrix, verb by verb.

**The `down` removes the key.** The grant goes with the object it names. A role
document left naming an object outside the closed set would fail every login
(see below). Copy the down half of the same backfill pair.

### The typo that locks users out of login

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
A role carrying an invalid document is treated as a data defect to surface, and
the whole authentication fails instead of downgrading to no access.

That makes a typo in the migration's JSON path far worse than a missing grant. If
you write `'{objects,webook_subscription}'`, the migration succeeds, the object is
never granted, and **every user holding that role is locked out**, because
the document now names an object outside the closed set. Spell the path from the
same string literal you added to `coreObjects`, and prove it by running the
integration lane in step 6. A typo'd path leaves the object ungranted, which the
convergence arms fail on.

## 4. Regenerate the published matrix

[reference/rbac-matrix.md](../reference/rbac-matrix.md) is generated from the
seeded documents. From `backend/`:

```bash
go test ./internal/modules/identity/ -run RBAC -update
```

The same test, **`TestPublishedRBACMatrixMatchesTheSeededRoleDocuments`** in
`backend/internal/modules/identity/rbacmatrix_test.go`, runs on every build
*without* `-update` and fails when the page and the seeded values disagree. Read
the regenerated row: it renders each role's grant as `CRUD` letters, the resolved
answer and not the base-plus-overrides you wrote. That is where you confirm that
a role you wrote no line for inherited the grant you meant it to.

## 5. Gate the store, then the UI

**Server side.** Every exported method on the owning `*Store` or `*Service` that
touches the new object calls `auth.Require(ctx, "<object>", principal.ActionX)`,
plus `auth.EnsureVisible` / `auth.ScopeClauseFor` for row scope; see
[reference/platform-toolkit.md](../reference/platform-toolkit.md#platformauth--the-admission-point).
`TestEveryStoreEntryPointIsAuthGated` (`backend/gates/rbacgate_test.go`) derives the
entry-point set from the tree and fails an ungated store method. Such a method is
a door into tenant data, reachable by any transport wired to it and invisible to
review.

**Client side.** Bind the affordance to the **grant**, never to a role name. The
hooks are in `frontend/src/app/capability.ts` and take the generated `RbacObject`
union, so a misspelled object is a compile error:

| Hook | Use it for |
|---|---|
| `useCan(object, action)` | One specific request. Object RBAC only, no seat ceiling. |
| `useCanWrite(object, action)` | A control that issues a **mutating** request (the common case): grant ∧ seat. |
| `useCanUpsert(object)` | A control whose endpoint inserts *or* replaces, so the needed verb is not knowable client-side. |
| `useHoldsWriteGrant(object)` | An authoring *surface* (a nav entry, a section heading) where any write verb justifies showing it. |
| `useCanMutate()` | The licensing seat ceiling alone. |

The answers come from the server (`GET /me` carries the merged grants it
computed) and only the vocabulary comes from the contract, so the client stays
correct on a workspace whose stored grants have drifted. The hooks only shape
the UI. The server's `auth.Require` is the authority on every call, and a client
that gets it wrong shows the wrong button and never the wrong data.

## 6. Verify

Run both lanes, and know which one proves what:

**`make check`**: the merge gate.

- `TestContractObjectEnumMatchesPolicyVocabulary`: the contract enum and
  `coreObjects` are the same set. Catches step 2 missing, or misspelled.
- `TestPublishedRBACMatrixMatchesTheSeededRoleDocuments`: the published page
  matches the seeded documents. Catches step 4 not run.
- `TestEveryStoreEntryPointIsAuthGated`: no ungated store entry point. Catches
  step 5 missing on the server.
- `make check-fe`: the TypeScript build proves the new object is expressible in
  the capability hooks.

**`make test-integration`**: the real-Postgres lane, and **the only proof the
backfill landed on an old install**. All three gates live in
`backend/internal/compose/integration/rbacseedparity_integration_test.go` and
they execute the obligation instead of scanning for it, so no list of objects
and no grep for `'{objects,<name>}'` decides any of them:

- `TestTheRealBootstrapSeedsTheDocumentedMatrix`: the real bootstrap writes the
  documented matrix. Catches step 1 or step 4 not landing on a fresh install.
- `TestEveryRBACBackfillConvergesOnTheSeededMatrix`: each backfill, replayed
  against today's matrix minus its own objects, converges back onto the matrix.
  This arm isolates: a failure names your migration. A typo'd jsonb path from
  step 3, a wrong verb, or a `WHERE` clause that matches no rows shows up here.
- `TestTheBackfillsComposeFromTheOldestUpgradableInstallation`: every backfill
  replayed in version order over the documents an installation bootstrapped at
  the migration baseline held. **This arm catches step 3 missing entirely**, for
  any object added since that baseline. The isolating arm cannot: it derives its
  starting state from the migrations, so an object no migration mentions is
  never absent from that state and its missing backfill is invisible.

The composed arm's starting state is a committed fixture,
`backend/migrations/testdata/rbac_baseline_era_defaults.json`. Editing that
fixture could make a broken backfill look like a working one, so a second gate
guards it. `TestBaselineEraFixtureIsTheMatrixTheBaselineSeeded`
(`backend/gates/rbacbaselineerafixture_test.go`, unit lane) pins the fixture to
the baseline commit's `rbac_seeded_defaults.json`
(`git show <baseline>:backend/migrations/testdata/rbac_seeded_defaults.json`). It
compares decoded JSON, so re-indentation is no difference but any changed key or
value is. It also checks that the pinned commit is the consolidation baseline,
so the pin cannot be moved forward instead. Regenerate the fixture with the
command that gate's failure message prints; never by hand.

It lives in the unit lane because reading history needs a full checkout, and the
integration shards check out shallow.

Commit the policy change, the contract, the migration pair, the regenerated
matrix and the UI binding together. They are one change, and any one of them
alone is a broken state.
