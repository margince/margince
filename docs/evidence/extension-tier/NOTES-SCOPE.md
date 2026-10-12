# `extensions/notes`: scope

> **Historical record, 2026-08-28.** `extensions/notes` was removed when
> `openchannel` replaced it as the tier's one reference unit, so the unit below
> is no longer in the tree; see the sibling README for why the
> evidence is kept. The RLS and `workspace_id` statements here describe the tier
> as it was in August 2026.

The demo unit for the tier's first delivery. It makes every capability the tier gains visible and
clickable, so acceptance is a human driving the SPA rather than a green test suite.

**`fixtures/extensions/crm-hello` is not touched.** It stays the minimal CI fixture: the smallest unit
that exercises scan → compose → boot, copied under `extensions/` by the CI lane
(`fixtures/extensions/crm-hello/crmhello.go:5`). Growing it would cost the tier its "smallest path"
probe. `notes` is a *first-party enabled unit* alongside `de` and `yogi`.

---

## 1. The one-screen premise

Everything lands on one screen at `#/ext/notes`: **Demo Notepad**. The domain is mundane because the
demo exists to show the tier, and a domain-shaped demo would invite arguing about the domain.

```
┌─ Demo Notepad ──────────────────────────────────────────────┐
│                                                             │
│  Connection            ● connected                          │
│  Signing key      (stored — never displayed)  [ Replace ][×]│  ← secrets
│  [ sign this payload…            ] [ Sign ]                 │
│  → hmac-sha256  4f1c9a…e207                                 │  ← proves USE, not export
│                                                             │
│  ── Notes ───────────────────────────────────────────────   │
│  [ type a note…                              ] [ Add ]      │  ← api + migrations
│                                                             │
│  • 09:14  hello from the demo extension          [ × ]      │
│  • 09:10  ⟳ heartbeat — tick #7                             │  ← jobs
│  • 09:05  ⟳ heartbeat — tick #6                             │
│                                                             │
│  Last tick 4m ago · next in ~1m                             │
└─────────────────────────────────────────────────────────────┘
```

## 2. Capability → what the user actually does

| Surface | Demo behavior | How a human verifies it |
|---|---|---|
| **`migrations/`** | owns `ext_notes_note` | add a note, restart the stack, it is still there |
| **`api/`** | six POSTs (`/v1/ext/notes/list`, `/notes/add`, `/notes/remove`, `/signing-key`, `/signing-key/status`, `/signature`) and its own RBAC object `ext_notes_note` | a read-only seat sees the list; `Add` is not rendered |
| **`frontend/`** | the screen itself, mounted from the composed set | `#/ext/notes` resolves; on a vanilla tree it 404s |
| **`secrets`** | store a signing key; HMAC-sign a payload with it | paste a key → "connected"; sign a string → signature returned. The key is never emitted, even masked |
| **`Jobs`** | tick appends `heartbeat — tick #N` | leave the screen open; a row appears with no user action |
| **`Tools`** | `list_notes`, served, auto-execute + read | ask the agent "what's in my demo notepad" |

Six surfaces, one screen, nothing that needs explaining to whoever is watching.

Every operation is a POST, and there is no `/notes` base path. A served extension operation is a
governed tool invocation whose arguments are the request body, so the method validator admits only
POST/PUT/PATCH (see `extensions/notes/api/crm.yaml`). "list", "add" and "remove" are three verbs
on three paths. An operator who tries `GET /v1/ext/notes` gets a 404.

## 3. What each surface must prove, precisely

**`migrations/`**: the table is `ext_notes_note` rather than `note`. Namespacing is the claim, so the
demo should include a negative test. A migration attempting `notes_note` (unprefixed), or a table name
long enough to exceed the 63-byte derived-identifier budget, must fail generation with its position,
per the obligation deferred from `backend/pkg/extension/extension.go:40`. Rows are workspace-scoped
under RLS like core tables.

**`api/`**: the RBAC object proves the contract chain ran. `useCan` types on `RbacObject`
(`frontend/src/app/capability.ts:23`). That type accepts any `ext_` string, so a missing overlay merge,
a misspelled `x-rbac-object` or a fragment that never loaded is not a type error: the screen builds,
and the gate denies as if the user held no grant. The reason is ownership. `RbacObject` is a core node,
and a unit may extend only nodes it created, which keeps an installation's contract reproducible. A
misspelled core object is still a type error, because a string without the `ext_` prefix must be a
member of the enum.

Check the ordering at run time instead, with both of these:

1. **The merged contract.** Run `make gen`, then grep `build/composition/api/crm.yaml` for the unit's
   path and `build/composition/frontend/extensions.gen.ts` for `rbacObject: "ext_notes_note"`. If the
   overlay did not merge, both are absent. This names the missing artifact instead of failing at a
   call site.
2. **The `/me` snapshot.** The object must appear in `authorization.objects` for the demo principal.
   A gate that denies while the object is absent from `/me` means the overlay did not merge; a gate
   that denies while the object is present and false is the ordinary "no grant" state.

The composed lane still checks the route half at compile time: `src/api/client.ts` is parameterised by
the merged contract's `paths` under `tsconfig.composed.json`, so a call to a route the overlay did not
merge is a type error.

It must also prove the *server* half, which is a separate seam: the object has to reach `coreObjects`
through the published vocabulary seam and appear in the `/me` grants snapshot. A screen that typechecks
but gates on an object the client never learns the user holds renders nothing, and would look like a
frontend bug.

**`secrets`**: the demo proves the capability by use. The key is sealed via the port, and no endpoint
returns it or any part of it, masked or otherwise. Signing a payload shows the unit can use a credential
it never emits. A real connector needs the same shape (an HMAC webhook signature, a request signature),
so the demo exercises the production pattern. Store and use both land in `system_log`, and that audit
trail is part of the deliverable. Demo the namespace wall too: a second unit must not read `notes`'s key.

**`Jobs`**: the tick is the only thing on the screen that happens without a user. It writes one row and
returns. The demo should also carry a failing tick (behind a toggle) so the operator can see what a
panicking or slow extension job does to the worker. It must stay bounded and logged.

**`frontend/`**: `App.tsx:103` is a hardcoded `switch (screen)` over statically imported screens. The
slice adds a fall-through: unmatched screen → look up the composed extension registry from
`extensions.gen.ts`. Two things to pin while doing it:
- Vite must resolve `extensions/<name>/frontend/**` from `build/composition/`, with an alias plus
  `server.fs.allow`. Easy to miss and fails only at dev-server time.
- The vanilla tree must still build with the registry empty, and `#/ext/notes` must 404 cleanly.

## 4. Acceptance: the click-through

The acceptance run:

1. `make composition && make run` with `notes` present → boot inventory lists it
2. Navigate `#/ext/notes` → screen renders, "not connected"
3. Paste a signing key → "connected"; sign a payload → signature returned, key never displayed
4. Add a note → appears; reload → still there
5. Wait one tick interval → heartbeat row appears unprompted
6. Ask the agent to list notes → returns them
7. Switch to a read-only seat → list visible, `Add` gone
8. `rm -rf extensions/notes && make composition` → byte-identical to the committed vanilla stub,
   `#/ext/notes` 404s, `make check-composition` green

**Step 8 matters most.** It is the tier's core guarantee (an empty tree reproduces vanilla
byte-for-byte), and it is the one most likely to be broken by making `api/crm.yaml` and
`extensions.gen.ts` real instead of placeholders.

## 5. Explicit non-goals

- No core-object writes. The demo touches only `ext_notes_*`.
- No outbound network. `ScopeSend` is refused for served tools anyway
  (`backend/internal/compose/extensiontools.go`), and the demo must not argue with that.
- No second demo unit, except one throwaway fixture proving the secrets namespace wall (section 3).
- Nothing Zalo. A later delivery owns that.

## 6. Open, pending review

1. **Tick interval**: short enough to demo (60s?), long enough not to be noise in logs.
2. **Default-role seeding**: does the demo's RBAC object need seeding into a default role, or does an admin grant it by hand?
   Affects step 7 of the click-through.
3. **The heartbeat tick is a fan-out** (DESIGN.md section 4.4): the dispatcher kind enqueues one
   workspace child per live workspace. A single-workspace dev install has one child, so the demo should
   make the fan-out visible, with the heartbeat row naming its workspace. Otherwise it shows only the
   single-tenant case and the multi-tenant guarantee goes untested.
4. ~~Is `notes` shipped enabled in the vanilla tree?~~ **Decided 2026-08-08: shipped enabled**,
   alongside `de` and `yogi`, so every build exercises the demo.

   Consequence: `notes` is part of the vanilla composed set. `make check-composition`'s byte-identity
   gate proves an empty `extensions/` tree reproduces the committed stub, so it must keep running
   against an empty tree rather than "the tree minus notes". Step 8 of section 4 stays as written.
