// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./labeltranslations.css";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { LOCALES, type Locale } from "../i18n/locale";
import { suggestLabelTranslation } from "../lib/labelsuggestion";
import type { LabelTranslations } from "../lib/labeltranslation";
import type { CreateField } from "./create";

// The settings side of admin-named labels: one text field per UI language
// beside the stored name, and the line that shows what was given.

const NAME_IN: Record<Locale, MessageKey> = {
  en: "translation.nameIn.en",
  de: "translation.nameIn.de",
  vi: "translation.nameIn.vi",
};

/** The form key of one language's name, under a prefix the form owns. */
function translationKey(prefix: string, locale: Locale): string {
  return `${prefix}_${locale}`;
}

/**
 * One optional text field per UI language. With `source`, which reads the
 * stored name from the form, each field offers Suggest translation from it.
 */
export function translationFields(
  prefix: string,
  source?: (values: Record<string, string>) => string | undefined,
): CreateField[] {
  const named = (values: Record<string, string>) =>
    source?.(values)?.trim() ?? "";
  return LOCALES.map((locale) => ({
    key: translationKey(prefix, locale),
    label: NAME_IN[locale],
    maxLength: 200,
    fill: source
      ? {
          label: "translation.suggest",
          ready: (values) => named(values) !== "",
          from: (values) => suggestLabelTranslation(named(values), locale),
        }
      : undefined,
  }));
}

/** A stored map as the form's initial values. */
export function translationValues(
  prefix: string,
  translations: LabelTranslations | undefined,
): Record<string, string> {
  return Object.fromEntries(
    LOCALES.map((locale) => [
      translationKey(prefix, locale),
      translations?.[locale] ?? "",
    ]),
  );
}

/**
 * The form's answers as the map the server stores. Every language is sent,
 * an empty one included, because the server replaces the whole map and drops
 * an empty text.
 */
export function translationsFromValues(
  prefix: string,
  values: Record<string, unknown>,
): LabelTranslations {
  return Object.fromEntries(
    LOCALES.map((locale) => {
      const text = values[translationKey(prefix, locale)];
      return [locale, typeof text === "string" ? text.trim() : ""];
    }),
  );
}

/** The names given in further languages, as one quiet line under a name. */
export function TranslationsLine({
  translations,
}: Readonly<{ translations: LabelTranslations | undefined }>) {
  const t = useT();
  const given = LOCALES.filter((locale) => translations?.[locale]?.trim());
  if (given.length === 0) {
    return null;
  }
  return (
    <span className="label-translations" data-testid="label-translations">
      {given
        .map((locale) =>
          t("translation.given", {
            language: locale.toUpperCase(),
            name: translations?.[locale] ?? "",
          }),
        )
        .join(" · ")}
    </span>
  );
}

const MISSING_IN: Record<Locale, MessageKey> = {
  en: "translation.missing.en",
  de: "translation.missing.de",
  vi: "translation.missing.vi",
};

/**
 * The team's languages a label has no name in, as one line under the name.
 * `languages` are those some active member reads the app in, less the
 * installation's base language, which the stored name is written in.
 */
export function MissingTranslationsLine({
  translations,
  languages,
}: Readonly<{
  translations: LabelTranslations | undefined;
  languages: readonly Locale[];
}>) {
  const t = useT();
  const missing = languages.filter((locale) => !translations?.[locale]?.trim());
  if (missing.length === 0) {
    return null;
  }
  return (
    <span
      className="label-translations label-translations-missing"
      data-testid="label-translations-missing"
    >
      {missing.map((locale) => t(MISSING_IN[locale])).join(" · ")}
    </span>
  );
}
