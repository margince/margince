import { extensionCopy } from "@composition/copy";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { de } from "./de";
import { en, type MessageKey } from "./en";
import { vi } from "./vi";

// Locale is a presentation concern only (architecture/10 §3): it resolves at
// the render edge and never participates in storage or math. The resolution
// order is the signed-in person's own `user.locale` → their stored pick on this
// machine → the browser's Accept-Language → en-GB (A100), which stays the floor
// when the browser asks for a language we don't ship.
//
// The first of those ARRIVES: `/v1/me` resolves below this provider, so whoever
// holds it calls `adoptLocale`. Until it answers — and for a reader who has
// never chosen — the stored pick and then the browser decide, which is what a
// signed-out page renders from.
//
// The installation's `base_language` is a different setting and does not appear
// here: it governs what AI writes for the whole team, not what any one person
// reads the interface in.

// The catalog registry is what we ship. `Locale` derives from it, so the type
// needs no edit when a locale arrives. `LOCALES` below does NOT derive: it is
// hand-ordered because it also fixes the order the switcher shows, and both
// the switcher and browser detection read that written list. `satisfies
// readonly Locale[]` proves each entry is a real locale — it does not prove
// the list is COMPLETE. Completeness is enforced by i18n.test.ts.
export const catalogs = { en, de, vi } satisfies Record<
  string,
  Record<MessageKey, string>
>;

export type Locale = keyof typeof catalogs;

/**
 * A key an extension unit's own copy supplies, namespaced to the unit the way
 * its tables and RBAC objects are (`extNotes.title`).
 *
 * A template-literal type rather than a union, for the reason
 * ExtensionRbacObject is one: this file cannot enumerate what an installation
 * enabled, and a union would have to be generated and would then make the
 * vanilla and composed lanes different programs. The GENERATOR checks the real
 * rule — that every key a unit ships begins with that unit's own prefix.
 */
export type ExtensionMessageKey = `ext${string}`;

/**
 * The copy an enabled unit contributed, per locale, merged by gen-composition.
 * Empty on a vanilla tree.
 */
const unitCopy: Partial<Record<Locale, Record<string, string>>> = extensionCopy;

// Display order for the switcher. `satisfies` proves each entry is a real
// locale; i18n.test.ts proves the list is exhaustive.
export const LOCALES = ["en", "de", "vi"] as const satisfies readonly Locale[];

export const DEFAULT_LOCALE: Locale = "en";

function isLocale(value: string): value is Locale {
  return LOCALES.some((locale) => locale === value);
}

// The endonym key for a locale. The template literal is checked against
// MessageKey, so adding a locale without adding its `locale.name.<code>` key
// fails the build rather than rendering a raw key at runtime.
export function localeNameKey(locale: Locale): MessageKey {
  return `locale.name.${locale}`;
}

// detectLocale reads the visitor's own language preference and maps it to a
// locale we ship, falling back to the A100 default when none of the shipped
// locales is asked for. It never throws off-browser (SSR, tests): an absent
// navigator yields the default.
export function detectLocale(
  languages: readonly string[] = globalThis.navigator?.languages ??
    (globalThis.navigator?.language ? [globalThis.navigator.language] : []),
): Locale {
  for (const tag of languages) {
    const base = tag.toLowerCase().split("-")[0];
    if (isLocale(base)) {
      return base;
    }
  }
  return DEFAULT_LOCALE;
}

// Where an EXPLICIT locale choice is kept so it survives a reload. The reader
// picks a language on the sign-in screen, and until /v1/me carries a locale
// this is the only place the choice can live — without it the next load falls
// back to the browser's preference and silently undoes the pick, which reads as
// the switcher not working.
const LOCALE_STORAGE_KEY = "margince.locale";

/**
 * The locale a reader has chosen, if they have chosen one.
 *
 * Only an explicit pick is stored, never the detected default. Persisting what
 * the browser asked for would freeze it: a reader who later changes their
 * browser's language would keep getting the old one from a value they never set.
 *
 * The stored string is validated rather than trusted. It outlives the release
 * that wrote it, so a locale we have since stopped shipping — or a hand-edited
 * value — must fall back to detection instead of reaching the catalogs as a key
 * they have no entry for.
 *
 * Storage is unavailable in some browser modes and throws on ACCESS rather than
 * returning null. That is not an error to report: this value is a preference
 * whose absence already has a defined meaning, and detection is exactly what
 * absence means. A reader in private mode gets the browser's language, which is
 * the same answer they got before this existed.
 */
export function storedLocale(): Locale | null {
  try {
    const stored = globalThis.localStorage?.getItem(LOCALE_STORAGE_KEY);
    return stored !== null && stored !== undefined && isLocale(stored)
      ? stored
      : null;
  } catch {
    return null;
  }
}

function rememberLocale(locale: Locale): void {
  try {
    globalThis.localStorage?.setItem(LOCALE_STORAGE_KEY, locale);
  } catch {
    // Same reasoning as the read: a preference that cannot be kept is a
    // preference that holds for this session only, not a failure to surface.
  }
}

export function translate(
  locale: Locale,
  key: MessageKey | ExtensionMessageKey,
  params?: Record<string, string | number>,
): string {
  // CORE FIRST, and a unit second. The generator already refuses a key outside
  // the unit's own namespace, so the two sets cannot overlap today — this
  // ordering is what keeps that true if the namespace rule were ever loosened:
  // a unit must not be able to change what a core string says.
  //
  // A key neither side carries falls back to the key itself, which is what an
  // untranslated string has always done here and reads as an obvious defect on
  // the page rather than as an empty element.
  const message =
    (catalogs[locale] as Record<string, string>)[key] ??
    unitCopy[locale]?.[key] ??
    key;
  if (!params) {
    return message;
  }
  return message.replace(/\{(\w+)\}/g, (whole, name: string) =>
    name in params ? String(params[name]) : whole,
  );
}

type LocaleContextValue = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  /**
   * Take the language the signed-in person chose, as reported by the server.
   *
   * Separate from `setLocale` because it is not a choice being MADE: it is one
   * already on record arriving late, so it must not be written back as a fresh
   * pick. The caller is whoever holds `/me` — the provider sits above it and
   * cannot ask.
   */
  adoptLocale: (locale: Locale) => void;
};

const LocaleContext = createContext<LocaleContextValue>({
  locale: DEFAULT_LOCALE,
  setLocale: () => {},
  adoptLocale: () => {},
});

export function LocaleProvider({
  initial,
  children,
}: Readonly<{
  initial?: Locale;
  children: ReactNode;
}>) {
  // Three sources, in falling order of authority: the server's answer for this
  // person (`/v1/me` carries their chosen locale), then the reader's own stored
  // pick, then the browser's preference. The stored pick outranks detection
  // because it is the more specific statement of the same intent — this reader,
  // on this machine, asked for this language.
  const [locale, setLocaleState] = useState<Locale>(
    () => initial ?? storedLocale() ?? detectLocale(),
  );
  // The signed-in person's own choice, which ARRIVES rather than being present
  // at mount: `/me` resolves after this provider renders, so seeding state once
  // would mean a stored choice never reached the browser they signed in from —
  // the whole reason for storing it on the server.
  //
  // Adopted only when it CHANGES, so a reader who switches language mid-session
  // is not dragged back to their saved one before the write lands.
  // What the signed-in person chose, once it is known. It ARRIVES rather than
  // being present at mount — `/me` resolves below this provider — so the
  // provider takes it through `adoptLocale` instead of seeding state once,
  // which is what lets a stored choice reach the browser they just signed in
  // from.
  //
  // Adopted only when it DIFFERS from the last adopted value, so a reader who
  // switches language mid-session is not dragged back to their saved one while
  // the write is still in flight.
  const [adopted, setAdopted] = useState<Locale | undefined>(initial);
  const adoptLocale = useCallback(
    (next: Locale) => {
      // Validated for the same reason the stored pick is: this value comes off
      // the wire, and a locale we do not ship — an older release's, a regional
      // tag, a server that widened the field — would otherwise reach `catalogs`
      // as a key it has no entry for. Every lookup then throws, including the
      // error boundary's own, so a single unknown string costs the reader the
      // whole application rather than just their language.
      if (!isLocale(next)) {
        return;
      }
      if (next === adopted) {
        return;
      }
      setAdopted(next);
      setLocaleState(next);
    },
    [adopted],
  );
  // A pick is remembered where a detected default is not, so the two cannot be
  // confused later. Wrapped rather than persisted in an effect: an effect would
  // also fire for the detected value on first render and store a choice nobody
  // made.
  const setLocale = useCallback((next: Locale) => {
    rememberLocale(next);
    setLocaleState(next);
  }, []);
  /*
   * The document's own language follows the catalog. index.html can only ship a
   * static `lang`, so without this every German reader gets German text inside a
   * document declared English — a screen reader then applies English phonemes to
   * German words, which is the difference between a page that can be listened to
   * and one that cannot. WCAG 3.1.1.
   *
   * It fails on FIRST LOAD, not only after the switcher is used, because
   * `detectLocale` reads the browser's own preference — so the reader most likely
   * to need it is the one who never touches the switch.
   */
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  const value = useMemo(
    () => ({ locale, setLocale, adoptLocale }),
    [locale, setLocale, adoptLocale],
  );
  return (
    <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
  );
}

export function useLocale(): LocaleContextValue {
  return useContext(LocaleContext);
}

export function useT() {
  const { locale } = useContext(LocaleContext);
  // NARROW on purpose: a core key, and a typo in one is a compile error. The
  // union a unit needs is added at the published surface (src/surface/index.ts)
  // rather than here, because `ReturnType<typeof useT>` is the parameter type
  // some two dozen core helpers take a translator as — and widening the RETURN
  // makes every core-only test fake stop being assignable to it, for a
  // capability no core helper has any use for.
  return (key: MessageKey, params?: Record<string, string | number>) =>
    translate(locale, key, params);
}
