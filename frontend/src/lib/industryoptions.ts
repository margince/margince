// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { INDUSTRY_CODES, industryOptions } from "../format/industry";
import { useLocale, useT } from "../i18n";

// The fixed list in the reader's order. A value that is no code, from a row not
// yet repaired, is offered as it is stored so a save keeps it.
export function useIndustryOptions(stored: string | null | undefined) {
  const t = useT();
  const { locale } = useLocale();
  const options = industryOptions(t, locale);
  return stored && !INDUSTRY_CODES.some((code) => code === stored)
    ? [{ value: stored, label: stored }, ...options]
    : options;
}
