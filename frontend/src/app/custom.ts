import type { LucideIcon } from "lucide-react";
import type { ComponentType } from "react";
import type { Locale } from "../i18n";
import type { MessageKey } from "../i18n/en";

// The fork seam for UI, mirroring the one the backend already has.
//
// ADR-0054 §7 gives a fork `modules/<name>/custom/` and `migrations/custom/`:
// directories upstream ships and never writes to, so a fork extends the product
// without patching a file upstream also edits, and an upgrade is a fast-forward
// rather than a merge. The frontend had no counterpart. A fork that added one
// screen had to edit `App.tsx` (the dispatch), `nav.ts` (the rail) and
// `router.tsx` (the address union) — the three files upstream touches most —
// so every release was a conflict in all three.
//
// This is the same seam with the same property, and the property is the design:
// NOTHING A FORK OWNS IS A FILE UPSTREAM EDITS. A fork adds a directory under
// `src/screens/custom/` and nothing else. The registry below discovers it with
// `import.meta.glob`, the way `migrations/custom/` is discovered by an embed
// rather than by a list somebody maintains — a registry file a fork appended to
// would be a shared file again, and would conflict on exactly the releases this
// exists to make clean.
//
// The address is `#/x/<key>`, and the `x` is the hash-route spelling of the
// prefix the schema already uses for the same reason (`x_` columns "can never
// collide with an upstream column on upgrade" — migrations/custom/README.md).
// A fork screen at `#/warranty` would be one release away from colliding with
// an upstream destination of that name, and the collision would be silent: the
// upstream screen would win and the fork's would become unreachable.
//
// This is why `Screen` gains ONE member rather than widening to a template
// literal. `x` opens a whole namespace under it, and the dispatch in App.tsx
// stays an exhaustive Record over a finite union — a `Screen` that admitted
// `x-${string}` would make that Record impossible to write, and the
// exhaustiveness is what stops a destination existing in the router and being
// missing from the dispatch.

/**
 * What a fork screen is called, in the reader's language.
 *
 * Two arms, and the second is what makes this seam whole. A `MessageKey` reuses
 * a word the product already has — right for a screen that IS one of the
 * product's nouns seen differently. But a fork's own noun has no key, and
 * minting one means editing `i18n/en.ts`, `de.ts` and `vi.ts`: three upstream
 * files, for the one string that names a row. That is the conflict this whole
 * directory exists to avoid, so a fork ships its words WITH its screen instead.
 *
 * English is required and the rest are optional, which is the honest shape: the
 * product ships three languages and a fork may not, so there is always
 * something to render and never a locale that renders a key. A fork adds
 * locales as it has them, in its own file, at no cost to anyone.
 */
export type CustomLabel =
  | MessageKey
  | Readonly<Partial<Record<Locale, string>> & { en: string }>;

/**
 * The words a fork label resolves to for one reader.
 *
 * A key goes through `t` like any other; a fork's own record is read directly,
 * falling back to English for a locale it does not carry. Discriminated on
 * `typeof`, which is exact here: a `MessageKey` is a string literal and the
 * record is never one.
 */
export function resolveCustomLabel(
  label: CustomLabel,
  locale: Locale,
  t: (key: MessageKey) => string,
): string {
  return typeof label === "string" ? t(label) : (label[locale] ?? label.en);
}

/**
 * One screen a fork adds, as its directory declares it.
 *
 * `nav` and `palette` are optional and mean different things by their absence:
 * a screen with no `nav` is reachable by address and by anything that links to
 * it, which is the right answer for a surface opened FROM somewhere (a detail
 * page, a wizard step) rather than navigated to.
 */
export type CustomScreen = {
  /**
   * The `<key>` in `#/x/<key>`. Lowercase, digits and hyphens: it is a URL
   * segment, and a key that needed encoding would appear in one spelling in the
   * address bar and another in the registry.
   */
  key: string;
  /**
   * A component, not a rendered node — React needs a stable type to mount, and
   * a registry of elements would build every fork screen on every route change
   * including the ones nobody navigated to. The same reason the extension
   * registry holds components (app/extensions.ts).
   */
  component: ComponentType;
  /** A rail entry, if this screen is a destination a person navigates TO. */
  nav?: {
    /**
     * Which of the rail's existing groups it joins. Not a new group: the three
     * headings are the product's own answer to "what kind of thing is this",
     * and a fork that wanted a fourth is describing a different product rather
     * than an addition to this one.
     */
    group: "records" | "work" | "intelligence";
    label: CustomLabel;
    icon: LucideIcon;
  };
  /**
   * A command-palette entry. Separate from `nav` because they answer different
   * questions — the rail is where things live, the palette is what you can do —
   * and a screen can honestly want one without the other.
   */
  palette?: { label: CustomLabel };
};

/**
 * The route segment every fork surface lives under.
 *
 * Exported and spelled once, because the router's union, App.tsx's dispatch and
 * any link a fork writes all have to agree: a registry keyed under a segment
 * the router never dispatches on resolves nothing, however correct its lookup.
 */
export const CUSTOM_SCREEN = "x";

/**
 * Every screen the fork's own directory declares.
 *
 * Eager, because this is what the router dispatches on: a lazily-globbed
 * registry would answer "no such screen" for a key whose module had not been
 * fetched yet, and the answer would depend on what the reader had already
 * visited. The cost is that a fork's screens are in the entry chunk; upstream
 * ships none, so upstream pays nothing, and a fork that wants its screen split
 * out uses React.lazy INSIDE its own component, where it can say so.
 *
 * The glob is a literal on purpose — Vite resolves it at build time and cannot
 * see through a variable — so this string is the whole contract with a fork's
 * directory layout, and it is stated in the README beside it.
 */
const declared = import.meta.glob<{ readonly screen?: CustomScreen }>(
  "../screens/custom/*/screen.tsx",
  { eager: true },
);

/** A registry of fork screens, keyed by the address segment that reaches each. */
export type CustomScreenRegistry = ReadonlyMap<string, CustomScreen>;

/**
 * The registry, keyed by the address segment that reaches it.
 *
 * Built from `key` rather than from the directory name, so a fork renaming a
 * folder does not silently change a URL people have bookmarked. A module that
 * exports no `screen` is skipped rather than failing the build: the directory
 * is a fork's, and a work-in-progress file there must not stop the product
 * compiling.
 */
export const customScreens: CustomScreenRegistry = buildRegistry(declared);

/**
 * The registry, refusing a key claimed twice.
 *
 * A Map would silently keep the last of them, and the loser would be a screen
 * with a directory, a component and a rail row that no address reaches — the
 * failure this seam is worst at, because there is no error to read and nothing
 * looks wrong. It THROWS, at module load, so a fork learns on its first build
 * rather than from a page that never opens.
 *
 * A throw and not a console warning: the registry is what the router dispatches
 * on, and a build where two screens claim one address has no correct behaviour
 * to fall back to.
 */
export function buildRegistry(
  modules: Record<string, { readonly screen?: CustomScreen }>,
): CustomScreenRegistry {
  const registry = new Map<string, CustomScreen>();
  for (const [path, module] of Object.entries(modules)) {
    const screen = module.screen;
    if (screen === undefined) {
      continue;
    }
    const claimed = registry.get(screen.key);
    if (claimed !== undefined) {
      throw new Error(
        `two custom screens claim the key "${screen.key}" (${path} is the second). ` +
          "The key is the address segment that reaches a screen, so the first would " +
          "become unreachable — give one of them a different key.",
      );
    }
    registry.set(screen.key, screen);
  }
  return registry;
}

// The three readers below are pure functions OF a registry, defaulting to the
// discovered one. Not test scaffolding, and the same reason app/extensions.ts
// gives for the same shape: upstream's registry is empty by construction, so a
// reader that closed over the module binding could only ever be exercised on
// its miss path — every answer it gave a fork would be one nothing upstream had
// ever run.

/** The screen a `#/x/<key>` address names, or undefined for one nothing declares. */
export function findCustomScreen(
  key: string | undefined,
  registry: CustomScreenRegistry = customScreens,
): CustomScreen | undefined {
  return key === undefined ? undefined : registry.get(key);
}

// The two readers below return a NARROWED screen rather than a CustomScreen, so
// a caller reading the field the filter selected on needs no non-null
// assertion — which this codebase forbids, for the reason one is worth
// forbidding: an assertion is a claim the compiler stops checking, and the
// claim here would be "the filter above me still filters on this". A type
// predicate keeps the two together instead.

/** A fork screen that declared a rail entry. */
export type CustomNavScreen = CustomScreen & {
  nav: NonNullable<CustomScreen["nav"]>;
};

/** A fork screen that declared a palette command. */
export type CustomPaletteScreen = CustomScreen & {
  palette: NonNullable<CustomScreen["palette"]>;
};

/** Every fork screen that asked for a rail entry, in the group it named. */
export function customNavItems(
  group: CustomNavScreen["nav"]["group"],
  registry: CustomScreenRegistry = customScreens,
): readonly CustomNavScreen[] {
  return [...registry.values()].filter(
    (screen): screen is CustomNavScreen => screen.nav?.group === group,
  );
}

/** Every fork screen that asked to be findable by name in the palette. */
export function customPaletteScreens(
  registry: CustomScreenRegistry = customScreens,
): readonly CustomPaletteScreen[] {
  return [...registry.values()].filter(
    (screen): screen is CustomPaletteScreen => screen.palette !== undefined,
  );
}
