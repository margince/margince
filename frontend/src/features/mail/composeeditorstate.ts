// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How the composer's fields are saved with a draft and read back from one.

import type { components } from "../../api/schema";
import { isLocale, type Locale } from "../../i18n/locale";
import type { FieldsSnapshot } from "./composefields";

// A saved code this build does not ship is no choice it can name again.
export function chosenLanguage(saved: string | undefined): Locale | null {
  return saved !== undefined && isLocale(saved) ? saved : null;
}

export function editorStateOf(
  fields: Pick<
    FieldsSnapshot,
    "intent" | "provenance" | "servedBody" | "language"
  >,
): components["schemas"]["MailDraftEditorState"] {
  return {
    intent: fields.intent,
    ai_generated: fields.provenance?.ai_generated ?? false,
    ai_disclosure: fields.provenance?.ai_disclosure ?? undefined,
    voice_profile_version:
      fields.provenance?.voice_profile_version ?? undefined,
    voice_degraded: fields.provenance?.voice_degraded ?? false,
    served_body: fields.servedBody,
    ...(fields.language ? { language: fields.language } : {}),
  };
}
