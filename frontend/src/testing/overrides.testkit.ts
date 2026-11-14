// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { isLocale, type Locale } from "../i18n/locale";
import { vi } from "../i18n/vi";

const core: Record<Locale, Readonly<Record<string, string>>> = { en, de, vi };

const PLURAL_FORM = /^(.+)_(zero|one|two|few|many|other)$/;
const placeholders = (text: string): string[] =>
  [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1] ?? "");

/**
 * What an override breaks against core. It may drop a placeholder core has.
 * It may omit a plural form core has for the same base.
 */
export function overrideProblems(
  locale: Locale,
  overrides: Readonly<Record<string, string>>,
): string[] {
  const catalog = core[locale];
  const problems: string[] = [];
  for (const [key, value] of Object.entries(overrides)) {
    const original = catalog[key];
    if (original === undefined) continue;
    for (const name of placeholders(original)) {
      if (!placeholders(value).includes(name)) {
        problems.push(`${locale} ${key}: drops {${name}}`);
      }
    }
    for (const sibling of missingPluralForms(catalog, key, overrides)) {
      problems.push(`${locale} ${key}: omits plural form ${sibling}`);
    }
  }
  return problems;
}

/** The plural forms core has for a key's base that the override lacks. */
function missingPluralForms(
  catalog: Readonly<Record<string, string>>,
  key: string,
  overrides: Readonly<Record<string, string>>,
): string[] {
  const base = PLURAL_FORM.exec(key)?.[1];
  if (base === undefined) return [];
  return Object.keys(catalog).filter(
    (sibling) =>
      PLURAL_FORM.exec(sibling)?.[1] === base &&
      !Object.hasOwn(overrides, sibling),
  );
}

/**
 * The problems of every `extensions/<unit>/frontend/i18n-overrides/<locale>.json`
 * under a root. Read from the files themselves, so the answer does not depend
 * on which extensions a lane composed.
 */
export function shippedOverrideProblems(extensionsRoot: string): string[] {
  const problems: string[] = [];
  if (!existsSync(extensionsRoot)) return problems;
  for (const unit of readdirSync(extensionsRoot)) {
    const layer = join(extensionsRoot, unit, "frontend", "i18n-overrides");
    if (!existsSync(layer)) continue;
    for (const file of readdirSync(layer)) {
      const locale = file.replace(/\.json$/, "");
      if (!isLocale(locale)) {
        problems.push(`${unit}/${file}: not a shipped locale`);
        continue;
      }
      const overrides: Record<string, string> = JSON.parse(
        readFileSync(join(layer, file), "utf8"),
      );
      problems.push(...overrideProblems(locale, overrides));
    }
  }
  return problems;
}
