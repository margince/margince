// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LOCALES, type Locale } from "../../i18n/locale";
import { useInstallationSettings } from "../../lib/uploadlimit";

/**
 * The UI languages an admin-named label needs a name in: those some active
 * member chose, less the installation's base language, which the stored name
 * is written in. Empty until the settings answer, so nothing is marked early.
 */
export function teamLanguages(
  memberLanguages: readonly string[] | undefined,
  baseLanguage: Locale | undefined,
): Locale[] {
  if (!memberLanguages || !baseLanguage) return [];
  return LOCALES.filter(
    (locale) => locale !== baseLanguage && memberLanguages.includes(locale),
  );
}

export function useTeamLanguages(): Locale[] {
  const settings = useInstallationSettings().data;
  return teamLanguages(settings?.member_languages, settings?.base_language);
}
