// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../api/client";
import type { Locale } from "../i18n/locale";
import { unwrap } from "./problem";

/**
 * An AI suggestion of one admin-named label in another UI language. Nothing
 * is stored: the editor fills the field and the admin saves it, or not.
 */
export async function suggestLabelTranslation(
  text: string,
  language: Locale,
): Promise<string> {
  const answer = unwrap(
    await api.POST("/label-translations", { body: { text, language } }),
  );
  return answer.text;
}
