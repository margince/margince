<!-- prose:plain -->
# Add an RBAC object

Use this page to add a new kind of record to the access model. That is a new name in the closed set of
objects that a role document may grant. To add an operation to a module that exists, use
[add-an-endpoint.md](add-an-endpoint.md). For the whole capability, use [add-a-module.md](add-a-module.md).
What the whole set looks like: [reference/rbac-matrix.md](../reference/rbac-matrix.md). Why it works this
way: [explanation/rbac-roles-and-teams.md](../explanation/rbac-roles-and-teams.md).

The change touches **five places**, and each one you skip fails in a different way. Gates that block a merge
hold two of them, so you find out before you push. The backfill migration (step 3) gives **no sign at all**
at build or review time on a new database. If you get it wrong, every installation that exists answers 403
for good.

## 1. Add the object to the policy word list

In `backend/internal/modules/identity/internal/policy/policy.go`:

1. Add the name to `coreObjects`.
   This is the closed set that `Parse` checks against, so nothing outside it can ever be granted.
2. In `defaults.go`, decide for **each system role** if the new object matches that role's base grant.
   The roles are `admin`, `management`, `manager`, `rep`, `read_only` and `ops`.
   If it matches, there is nothing to write: `grid` gives every core object the base grant.
   If it does not match, add one line to that role's map of changes, with the object and its grant.

   An object you do not name in the map gets the role's base grant, and nothing warns you. Decide for every role,
   and check the result in the matrix at step 4. That page is the only thing that shows a role gets a grant
   you never planned.

**Nothing depends on order.** `grid(base, overrides)` gives every object in `coreObjects` the base grant,
then applies the changes by name. So each grant is written beside the object it covers. A change that names
an object that does not exist stops the package at start with a panic. A typo is then a build failure, not a
role that covers nothing.

So a role's map of changes lists the places where that role leaves its own base grant. A reviewer reads
"where does `rep` not have the record grant", not "what does `rep` hold on every object". `managerObjects` is one
value shared by `manager` and `management`, so the two cannot drift. Only their row scope is different.

Choose the grant from a case that exists; do not make up a new one. The comment block above `defaults`
records the reason for each kind of grant: record grants, pipeline settings grants, and settings owned by `admin` or
`ops`. A reviewer checks your pick by matching it to one of them.

## 2. Add the object to the contract enum

Add the same string to `RbacObject` in `backend/api/crm.yaml`.

**The server reads nothing from it.** `oapi-codegen` makes no Go values for a string enum that stands alone
at the root of the schema. So `policy.coreObjects` and this enum are both kept by hand. The enum serves the web client.
`openapi-typescript` turns it into a string union, so a check against a wrong object name is a TypeScript
error. It is not a check that compiles and then refuses for good.

A test that blocks a merge keeps the two equal:
**`TestContractObjectEnumMatchesPolicyVocabulary`** in `backend/gates/rbacvocabulary_test.go`. Both sides are
read from the source. `coreObjectsFromSource` (`backend/gates/rbacvocabularysource_test.go`) reads the
object list out of `policy.go`, and the contract side is read from the YAML. So the test never becomes a
third place to keep up to date. An edit to the enum alone changes what clients can *say*, never what the server
*checks*.

## 3. Write the backfill migration (the step that fails without a sign)

Role seeding runs **once**, when the workspace is created, and never runs again.
`identity.seedSystemRoles` writes `policy.MustDefaultJSON(role.key)` into `role.permissions` when the
workspace starts (`backend/internal/modules/identity/service.go`). No code path later compares a stored
document with the defaults built into the code. Sign-in reads the *stored* document: `loadGrants` reads
`role.permissions` and merges it. So an object added to `coreObjects` with no migration is granted to no
workspace that started earlier. It works on your new database and answers 403 in every other place, for good.

Write the pair in `backend/migrations/core/`. Follow [apply-migrations.md](apply-migrations.md) for numbers
and lanes. Copy the shape of the newest RBAC backfill in `backend/migrations/core/`
(`ls *_rbac.up.sql | tail -1`):

- One `UPDATE role SET permissions = jsonb_set(permissions, '{objects,<name>}', '<grant>'::jsonb)` per
  grant, for all the roles that share it.
- Guard every statement with `WHERE is_system AND ... AND NOT permissions->'objects' ? '<name>'`. This guard
  acts only when the key is absent. So the migration costs nothing where no one needs it, and it changes
  nothing where an operator has already edited a role.
- The grants must give what step 1 seeds. The replay in step 6 compares the end state after the change
  with the seeded matrix, verb by verb.

**The `down` removes the key.** The grant goes with the object it names. A role document that still names
an object outside the closed set would fail every sign-in (see below). Copy the down half of the same
backfill pair.

### The typo that locks users out of sign-in

`policy.Parse` **refuses** an object key it does not know:

```go
for object := range doc.Objects {
    if !IsCoreObject(object) {
        return Document{}, fmt.Errorf("policy: unknown object %q in permissions document", object)
    }
}
```

`loadGrants` calls `Parse`. It runs on the **sign-in** path, when a session is read, and when
`identity/authority.go` works out an agent's rights again. A role with a bad document is a data error to
show. The whole sign-in fails; it does not drop to no access.

So a typo in the JSON path of the migration does more harm than a missing grant. Say you write
`'{objects,webook_subscription}'`. The migration passes and the object is never granted. Then **every user
who holds that role is locked out**, because the document now names an object outside the closed set. Write
the path from the same string you added to `coreObjects`. Prove it by running the integration lane in step
6. A path with a typo leaves the object with no grant, and the matching tests fail on that.

## 4. Build the published matrix again

[reference/rbac-matrix.md](../reference/rbac-matrix.md) is generated from the seeded documents. From
`backend/`:

```bash
go test ./internal/modules/identity/ -run RBAC -update
```

The same test, **`TestPublishedRBACMatrixMatchesTheSeededRoleDocuments`** in
`backend/internal/modules/identity/rbacmatrix_test.go`, runs on every build *without* `-update`. It fails when
the page and the seeded values disagree. Read the new row. It shows each role's grant in `CRUD` form: the
answer the code works out, not the base grant and changes you wrote. That is where you confirm that a role you wrote no
line for gets the grant you planned.

## 5. Gate the store, then the UI

**Server side.** Every public method on the owning `*Store` or `*Service` that touches the new object calls
`auth.Require(ctx, "<object>", principal.ActionX)`. It also calls `auth.EnsureVisible` or
`auth.ScopeClauseFor` for row scope; see
[reference/platform-toolkit.md](../reference/platform-toolkit.md#platformauth--the-admission-point).
`TestEveryStoreEntryPointIsAuthGated` (`backend/gates/rbacgate_test.go`) reads the list of entry points from
the tree and fails a store method with no gate. Such a method is a way into tenant data. Any transport wired
to it can reach it, and a review cannot see it.

**Client side.** Bind the control to the **grant**, never to a role name. The hooks are in
`frontend/src/app/capability.ts`. They take the generated `RbacObject` union, so a wrong object name fails
the compile:

| Hook | Use it for |
|---|---|
| `useCan(object, action)` | One request. Object RBAC only, no seat limit. |
| `useCanWrite(object, action)` | A control that sends a request **that changes data** (the normal case): grant ∧ seat. |
| `useCanUpsert(object)` | A control whose endpoint adds a record *or* puts a new one in its place, so the client cannot know the verb it needs. |
| `useHoldsWriteGrant(object)` | A place to write *in* (a menu entry, a section heading). It shows when any write verb is granted. |
| `useCanMutate()` | The seat limit from the license alone. |

The answers come from the server (`GET /me` carries the merged grants it worked out). Only the word list
comes from the contract. So the client stays correct on a workspace whose stored grants have moved. The hooks
only shape the UI. The server's `auth.Require` decides every call. A client that gets it wrong shows the
wrong button, and never the wrong data.

## 6. Check

Run both lanes, and know which one proves what.

**`make check`**: the merge gate.

- `TestContractObjectEnumMatchesPolicyVocabulary`: the contract enum and `coreObjects` are the same set. It
  finds step 2 missing, or a wrong name.
- `TestPublishedRBACMatrixMatchesTheSeededRoleDocuments`: the published page matches the seeded documents.
  It finds step 4 not run.
- `TestEveryStoreEntryPointIsAuthGated`: every store entry point has a gate. It finds step 5 missing on the
  server.
- `make check-fe`: the TypeScript build proves the capability hooks can name the new object.

**`make test-integration`**: the lane with a real Postgres, and **the only proof that the backfill reached
an old install**. All three gates live in
`backend/internal/compose/integration/rbacseedparity_integration_test.go`. They run the rule instead of
scanning for it. So no list of objects, and no search for `'{objects,<name>}'`, decides any of them:

- `TestTheRealBootstrapSeedsTheDocumentedMatrix`: the real start of a workspace writes the matrix the docs
  show. It finds step 1 or step 4 missing on a new install.
- `TestEveryRBACBackfillConvergesOnTheSeededMatrix`: each backfill runs against the matrix of today, without
  its own objects, and must reach the matrix again. This test points at one cause: a failure names your
  migration. A typo in the JSON path from step 3, a wrong verb, or a `WHERE` that matches no rows shows up
  here.
- `TestTheBackfillsComposeFromTheOldestUpgradableInstallation`: every backfill runs in version order over the
  documents that an installation started at the migration baseline held. **This test finds step 3 missing
  in full**, for any object added since that baseline. The test above cannot. It takes its first state from
  the migrations, so an object that no migration names is never absent from that state. So the test
  cannot see its missing backfill.

The first state of this last test is a committed fixture,
`backend/migrations/testdata/rbac_baseline_era_defaults.json`. An edit to that fixture could make a backfill that fails
look like a working one, so a second gate guards it.

`TestBaselineEraFixtureIsTheMatrixTheBaselineSeeded` (`backend/gates/rbacbaselineerafixture_test.go`, unit
lane) pins the fixture to the baseline commit's `rbac_seeded_defaults.json`
(`git show <baseline>:backend/migrations/testdata/rbac_seeded_defaults.json`). It compares the JSON as data, not as
text. So line breaks do not count, but any changed key or value does. It also checks that the pinned commit is
the baseline merge, so no one can move the pin to a later commit. Build the fixture again with the command that gate
prints when it fails; never by hand.

It lives in the unit lane because reading history needs a full checkout, and the integration lane takes a
short checkout.

Commit the policy change, the contract, the migration pair, the new matrix and the UI link together. They
are one change, and any one of them alone is a state that does not work.
