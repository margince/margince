// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";

export function sendingCompanyScope(
  id: string | null | undefined,
  name: string | null | undefined,
  t: Translator,
): string {
  if (name) return t("consent.exceptionCompany", { name });
  return t(
    id ? "consent.exceptionUnnamedCompany" : "consent.exceptionUnknownCompany",
  );
}
