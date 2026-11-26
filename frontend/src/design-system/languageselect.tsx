// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { localeNameKey, useT } from "../i18n";
import type { Locale } from "../i18n/locale";
import { Select, type SelectProps } from "./select";

type LanguageSelectProps<L extends Locale> = Omit<
  SelectProps,
  "options" | "value" | "onChange"
> &
  Readonly<{
    languages: readonly L[];
    value: L;
    onChange: (next: L) => void;
  }>;

/** A dropdown of languages, each named in its own language. */
export function LanguageSelect<L extends Locale>({
  languages,
  value,
  onChange,
  ...select
}: LanguageSelectProps<L>) {
  const t = useT();
  return (
    <Select
      {...select}
      value={value}
      // Each name is in its own language, so each option carries `lang` (WCAG
      // 2.2 AA 3.1.2); a locale code is already a BCP 47 subtag.
      options={languages.map((language) => ({
        value: language,
        label: t(localeNameKey(language)),
        lang: language,
      }))}
      // Narrowed through the list the options came from, so the answer is a
      // language with no assertion, and one never offered is dropped.
      onChange={(next) => {
        const picked = languages.find((language) => language === next);
        if (picked) onChange(picked);
      }}
    />
  );
}
