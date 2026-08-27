# custom/ — the fork-owned screen directory

Upstream ships this directory holding only this file. It is the frontend
counterpart of `backend/migrations/custom/` and `modules/<name>/custom/`
(ADR-0054 §7): a place a fork writes that upstream never does, so an upgrade
is a fast-forward instead of a conflict in `App.tsx`, `nav.ts` and
`router.tsx`.

## Adding a screen

One directory per screen, holding a `screen.tsx` that exports `screen`:

```
src/screens/custom/warranty/screen.tsx
```

```tsx
import { ShieldCheck } from "lucide-react";
import type { CustomScreen } from "../../../app/custom";

function WarrantyScreen() {
  return <div className="wrap">…</div>;
}

export const screen: CustomScreen = {
  key: "warranty",
  component: WarrantyScreen,
  // Optional. Without it the screen is reachable by address and by anything
  // that links to it, which is what a surface opened FROM somewhere wants.
  nav: {
    group: "records",
    label: { en: "Warranty", de: "Garantie" },
    icon: ShieldCheck,
  },
};
```

## Words

A label is **your own words**, shipped beside your screen:

```ts
label: { en: "Warranty", de: "Garantie" }
```

English is required, every other locale is optional, and a locale you do not
carry falls back to it. That is the honest shape: this product ships three
languages and your fork may not, so there is always something to render and
never a rail row showing a key.

It is a fork-local catalogue on purpose. `MessageKey` only accepts keys from
`src/i18n/en.ts`, so minting one for your own noun means editing `en.ts`,
`de.ts` and `vi.ts` — three upstream files, for the one string that names a
row, which is exactly the conflict this directory exists to avoid.

Where your screen IS one of this product's nouns seen differently, name the key
instead and get every language it already has:

```ts
label: "nav.contacts"
```

The address is **`#/x/warranty`**. The `x` segment is the hash-route spelling
of the `x_` column prefix custom migrations already use, and it is there for
the same reason: a fork screen at `#/warranty` is one upstream release away
from colliding with a destination of that name, and the collision would be
silent — the upstream screen would win and the fork's would become
unreachable.

`app/custom.ts` discovers this directory with `import.meta.glob`. There is no
registry file to append to, and that is the point: a list a fork edited would
be a shared file again, and would conflict on exactly the releases this seam
exists to keep clean.

## What a fork still cannot do

Add a rail **group**. The three headings are the product's own answer to
"what kind of thing is this", and a fork wanting a fourth is describing a
different product rather than an addition to this one — `nav.group` names one
of the existing three.

Reach `#/x/<key>` for a key nothing declares. That address renders the same
honest not-found a mistyped one does, rather than an empty page.
