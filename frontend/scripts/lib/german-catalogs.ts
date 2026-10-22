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

// The key families a reader from outside this installation opens. They keep
// Sie, because mail to a contact is written in Sie (`confirmLines` in
// mailcopy/catalog.go) and the server publishes the consent question in it.
// Every other key speaks du.
export const READ_BY_AN_OUTSIDER: Readonly<Record<string, string>> = {
  "buyer.": "die Käuferseite im Deal Room, die hier keinen Sitzplatz hat",
  "confirm.":
    "ein Kontakt, der die serverseitig gestellte Einwilligungsfrage beantwortet",
  "prefs.": "wer entscheidet, was diese Installation noch senden darf",
  "privacynotice.": "wer von außen nachliest, welche Daten gespeichert sind",
};

export function readByAnOutsider(key: string): boolean {
  return Object.keys(READ_BY_AN_OUTSIDER).some((prefix) =>
    key.startsWith(prefix),
  );
}
