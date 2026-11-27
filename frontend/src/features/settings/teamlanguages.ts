// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LOCALES, type Locale } from "../../i18n/locale";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { type User, useMembers } from "./users-members";

/**
 * The UI languages an admin-named label needs a name in: those some active
 * member chose, less the installation's base language, which the stored name
 * is written in. Empty until both reads answer, so nothing is marked early.
 */
export function teamLanguages(
  members: readonly Pick<User, "status" | "locale">[] | undefined,
  baseLanguage: Locale | undefined,
): Locale[] {
  if (!members || !baseLanguage) return [];
  const chosen = new Set(
    members
      .filter((member) => member.status === "active" && member.locale)
      .map((member) => member.locale),
  );
  return LOCALES.filter(
    (locale) => locale !== baseLanguage && chosen.has(locale),
  );
}

export function useTeamLanguages(): Locale[] {
  const members = useMembers();
  const settings = useInstallationSettings();
  return teamLanguages(members.data, settings.data?.base_language);
}
