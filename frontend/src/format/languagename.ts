// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./format";

/** The language an ISO 639-1 code names, in the reader's language. */
export function languageName(code: string, locale: Locale): string {
  try {
    return (
      new Intl.DisplayNames(INTL_LOCALE[locale], { type: "language" }).of(
        code,
      ) ?? code
    );
  } catch {
    // A code the platform table does not know is still one the reader can quote.
    return code;
  }
}
