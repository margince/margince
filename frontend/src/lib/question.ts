// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { foldForSearch, searchWords } from "../format/collate";

// The words an English or German question opens with, folded the way
// `searchWords` folds a query. Every list below is read whatever the screen's
// language, because a reader asks in the language they think in.
const QUESTION_WORDS = new Set([
  // en
  "how",
  "what",
  "where",
  "why",
  "when",
  "which",
  // de
  "wie",
  "was",
  "wo",
  "warum",
  "wann",
  "welche",
  "welcher",
  "welches",
]);

// A verb that also opens company names ("Can Do Logistics"), so it counts only
// with a pronoun or article after it: "can I", "kann man".
const AUXILIARIES = new Set([
  "can",
  "could",
  "do",
  "does",
  "is",
  "are",
  "should",
  "kann",
  "konnen",
  "muss",
  "darf",
  "gibt",
]);
const SUBJECTS = new Set([
  "i",
  "we",
  "you",
  "my",
  "our",
  "the",
  "a",
  "an",
  "it",
  "this",
  "there",
  "margince",
  "ich",
  "man",
  "wir",
  "du",
  "es",
  "mein",
  "meine",
  "der",
  "die",
  "das",
  "ein",
  "eine",
]);

// Vietnamese question phrases. Each is two words, because the single words
// (lam, tai, bao, khi) are also given names and surnames.
const LEADING_PHRASES = [
  "lam sao",
  "lam the nao",
  "tai sao",
  "vi sao",
  "bao gio",
  "khi nao",
  "cach nao",
  "o dau",
];
// Vietnamese most often puts the question at the end: "đổi múi giờ thế nào".
const TRAILING_PHRASES = ["the nao", "o dau", "khong", "chua"];

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
  const [first = "", second = ""] = words;
  const folded = foldForSearch(query.trim());
  return (
    QUESTION_WORDS.has(first) ||
    (AUXILIARIES.has(first) && SUBJECTS.has(second)) ||
    LEADING_PHRASES.some((phrase) => folded.startsWith(`${phrase} `)) ||
    TRAILING_PHRASES.some((phrase) => folded.endsWith(` ${phrase}`))
  );
}
