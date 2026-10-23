// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { foldForSearch, searchWords } from "../format/collate";

// The words a question opens with in English, German and Vietnamese, folded the
// way `searchWords` folds a query. All three lists hold whatever the screen's
// language, because a reader asks in the language they think in.
const LEADING = new Set([
  // en
  "how",
  "what",
  "where",
  "why",
  "when",
  "which",
  "who",
  "can",
  "could",
  "do",
  "does",
  "is",
  "are",
  "should",
  // de
  "wie",
  "was",
  "wo",
  "warum",
  "wann",
  "welche",
  "welcher",
  "welches",
  "wer",
  "kann",
  "konnen",
  "muss",
  "darf",
  "gibt",
  // vi
  "lam",
  "tai",
  "vi",
  "cach",
  "khi",
  "bao",
]);

// Vietnamese most often puts the question at the end: "đổi múi giờ thế nào".
const TRAILING = ["the nao", "o dau", "khong", "gi", "chua"];

/**
 * Whether the text in the search box reads as a question to ask the handbook
 * rather than a name to look up. At least three words, and either a closing
 * question mark or a question word where that language puts one. Three words
 * keep a name such as "Wo Holdings" out.
 */
export function looksLikeQuestion(query: string): boolean {
  const words = searchWords(query.replaceAll(/[?？]/g, " "));
  if (words.length < 3) {
    return false;
  }
  if (/[?？]\s*$/.test(query)) {
    return true;
  }
  const folded = foldForSearch(query.trim());
  return (
    LEADING.has(words[0] ?? "") ||
    TRAILING.some((phrase) => folded.endsWith(` ${phrase}`))
  );
}
