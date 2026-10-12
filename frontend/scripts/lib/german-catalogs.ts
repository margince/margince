// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { de } from "../../src/i18n/de";
import {
  catalogEntries,
  type LocaleEntry,
  localeCatalogs,
} from "./locale-catalogs";

// The letters a German word is made of. `\b` answers for none of the umlauts,
// so a boundary written with it would match inside a word that carries one.
export const LETTER = "A-Za-zÄÖÜäöüß";

export const GERMAN = localeCatalogs("de", de);

export function entries(): LocaleEntry[] {
  return catalogEntries(GERMAN);
}
