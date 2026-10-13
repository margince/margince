<!-- prose:plain -->
# custom/: the screen folder a fork owns

Margince ships this folder with only this file in it. It is the frontend partner of
`backend/migrations/custom/` and `modules/<name>/custom/`: a place a fork writes and Margince never
does. So an upgrade is a fast-forward instead of a conflict in `App.tsx`, `nav.ts` and `router.tsx`.

## Adding a screen

One folder per screen, holding a `screen.tsx` that exports `screen`:

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

A label is your own words, shipped next to your screen:

```ts
label: { en: "Warranty", de: "Garantie" }
```

English must be there, and every other locale may be left out. A locale you do not have falls
back to English, so a row in the side menu never shows a raw key.

The label is a catalog local to the fork, because `MessageKey` only accepts keys from
`src/i18n/en.ts`. To add a key for your own noun means editing `en.ts`, `de.ts` and `vi.ts`. That is
three Margince files for the one string that names a row, which is the conflict this folder exists to
avoid.

Where your screen is one of this product's nouns seen another way, name the key instead, and get
every language it already has:

```ts
label: "nav.contacts"
```

The address is **`#/x/warranty`**. The `x` part is how a hash route spells the `x_` column prefix
that custom migrations already use, for the same reason. A fork screen at `#/warranty` is one
Margince release away from a conflict with a page of that name. The conflict raises no error: the Margince
screen wins, and no one can reach the fork's screen any more.

`app/custom.ts` finds this folder with `import.meta.glob`, so there is no list file to add to. A
list a fork edited would be a shared file again, and would conflict on the releases this seam exists
to keep clean.

## What a fork still cannot do

Add a group to the side menu. The three headings are the product's own answer to "what kind of thing
is this". A fork that wants a fourth is describing some other product. `nav.group` names one of
the three that exist.

Reach `#/x/<key>` for a key nothing declares. That address renders the same not-found page as an
address typed wrong, not an empty page.
