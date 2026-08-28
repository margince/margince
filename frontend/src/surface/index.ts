// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Copy, locale and capability — the app-level surface a unit screen needs to
// render honestly rather than merely to render.
//
// The capability hooks are surface rather than an internal detail on purpose:
// a screen that shows a control the caller may not use has told them something
// false, and the alternative to exporting these is every unit inventing its
// own read of /me. They are UX honesty and never enforcement — the server
// refuses independently (`extensionTool.Handle`), which is what makes it safe
// to hand a unit the same hooks the core screens use.
//
// `useT` reads the merged catalogue, so a unit's own copy resolves through the
// same lookup as core's rather than through a second mechanism.
import { useCallback } from "react";
import {
  type ExtensionMessageKey,
  translate,
  useLocale as useCoreLocale,
} from "../i18n";
import type { MessageKey } from "../i18n/en";

export { useCan, useCanWrite } from "../app/capability";
// `formatNumber` alongside `formatDateTime` for the same reason: a unit's screen
// renders figures a person reads, `useT` below refuses a raw number, and the
// only alternative left to a unit would be `String(n)` — which groups for
// nobody. A surface that narrows the parameter without exporting the formatter
// has told the unit to write the defect a different way.
// `formatBytes` for the same reason, one step further: a unit screen that
// reports a stored payload's size in bytes has no locale-aware unit scale of
// its own to reach for, and inventing one per unit is the same defect
// `formatNumber`'s note describes, one abstraction level up.
export { formatBytes, formatDateTime, formatNumber } from "../format/format";
// LocaleProvider is here for the TESTS: a unit's screen calls useT, so a test
// that renders it without the provider the app mounts around it renders
// nothing useful. Exporting the provider is what lets a unit test its own
// screen the way the app actually runs it.
export { LocaleProvider, useLocale } from "../i18n";

/**
 * The catalogue lookup, widened to a unit's own keys.
 *
 * Core's `useT` is narrow — its key is a closed union, so a typo in a core key
 * is a compile error, and `ReturnType<typeof useT>` is the parameter type some
 * two dozen core helpers take a translator as. Widening it THERE would make
 * every core-only test fake stop being assignable, for a capability no core
 * helper has any use for. So the widening lives here, on the surface a unit
 * imports, which is the only place that needs it.
 *
 * A unit's half cannot be a closed union: this file cannot enumerate what an
 * installation enabled. The real rule is checked by the generator, which
 * refuses any key a unit ships that is not namespaced to that unit.
 *
 * The KEY is what widens here and nothing else. `params` stays strings, exactly
 * as core's does: a raw number reaching a catalog sentence is coerced without
 * grouping, which is wrong in a unit's screen for the same reason it is wrong
 * in a core one — and a cast that widened it here would put the hole back on
 * the one surface nothing in this repo typechecks against.
 */
export function useT(): (
  key: MessageKey | ExtensionMessageKey,
  params?: Record<string, string>,
) => string {
  // Built from `translate`, which already takes the wider key, rather than cast
  // from core's `useT`. A translator is CONTRAVARIANT in its key, so widening
  // core's narrow one is not an assignment TypeScript can check — the cast that
  // used to stand here suppressed the check on the one surface nothing in this
  // repo typechecks against, which is the worst place in the tree to hold a
  // claim by assertion. Same lookup, same merged catalogue, no `as`.
  const { locale } = useCoreLocale();
  // Stable per locale, so a unit can put `t` in a dependency array without the
  // memo it guards being recomputed on every render of the screen around it.
  return useCallback((key, params) => translate(locale, key, params), [locale]);
}
