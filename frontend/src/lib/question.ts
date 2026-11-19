// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { foldForSearch, searchWords } from "../format/collate";
import type { Locale } from "../i18n";

// Words are written folded, as `foldForSearch` folds a query, so "bao nhieu"
// also finds "bao nhiêu". A word keeps its marks when its folded spelling is
// also a name or another word: "mấy" (how many) folds to "may" like "máy".
type QuestionWords = {
  // A word that opens a question on its own.
  readonly openers: readonly string[];
  // A verb that also opens company names ("Can Do Logistics"), so it counts
  // only with a subject after it: "can I", "kann man".
  readonly auxiliaries: readonly string[];
  readonly subjects: readonly string[];
  // A phrase that marks a question wherever it stands. Vietnamese most often
  // puts it in the middle or at the end: "tôi thắng bao nhiêu deal".
  readonly phrases: readonly string[];
  // A word that closes a question: "có đạt mục tiêu không".
  readonly closers: readonly string[];
  // A phrase that asks for a figure from CRM data rather than the handbook.
  readonly data: readonly string[];
};

// Every list is read whatever the screen's language, because a reader asks in
// the language they think in.
const QUESTION_WORDS = {
  en: {
    openers: ["how", "what", "where", "why", "when", "which", "who"],
    auxiliaries: ["can", "could", "do", "does", "is", "are", "should", "will"],
    subjects: [
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
    ],
    phrases: [],
    closers: [],
    data: [
      "how many",
      "how much",
      "which deals",
      "which contacts",
      "which companies",
      "which leads",
      "which projects",
      "which tasks",
      "what deals",
      "what contacts",
      "what companies",
      "what leads",
      "what projects",
      "what tasks",
      "open pipeline",
      "sales won",
      "total",
      "sum",
      "count",
      "won",
      "lost",
      "target",
      "quota",
    ],
  },
  de: {
    openers: [
      "wie",
      "was",
      "wo",
      "warum",
      "wann",
      "welche",
      "welcher",
      "welches",
      "wer",
    ],
    auxiliaries: ["kann", "konnen", "muss", "darf", "gibt", "werden"],
    subjects: [
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
    ],
    phrases: [],
    closers: [],
    data: [
      "wie viele",
      "wie viel",
      "welche deals",
      "welche kontakte",
      "welche unternehmen",
      "welche leads",
      "welche projekte",
      "gewonnen",
      "verloren",
      "summe",
      "umsatz",
      "ziel",
    ],
  },
  // A plain phrase has two words, because the single words (lam, tai, bao,
  // khi, ai) are also given names, surnames or "AI".
  vi: {
    openers: [],
    auxiliaries: [],
    subjects: [],
    phrases: [
      "lam sao",
      "tai sao",
      "vi sao",
      "bao gio",
      "bao nhieu",
      "khi nao",
      "cach nao",
      "o dau",
      "the nao",
      "la gi",
      "mấy",
      "gì",
      "nào",
      "ai đã",
      "ai là",
      "ai đang",
      "của ai",
      "cho ai",
      "với ai",
    ],
    closers: ["khong", "chua"],
    data: [
      "bao nhieu",
      "mấy",
      "tổng",
      "doanh thu",
      "thắng",
      "mục tiêu",
      "chỉ tiêu",
    ],
  },
} satisfies Record<Locale, QuestionWords>;

const LANGUAGES: readonly QuestionWords[] = Object.values(QUESTION_WORDS);

// The text as space-padded word lines, so a phrase matches only whole words.
// A marked phrase is matched on the text as typed, a plain one on the folded.
type Line = { readonly marked: string; readonly plain: string };

function lineOf(text: string): Line {
  const marked = text
    .normalize("NFC")
    .toLowerCase()
    .split(/[^\p{L}\p{M}\p{N}]+/u)
    .filter((word) => word !== "");
  return {
    marked: ` ${marked.join(" ")} `,
    plain: ` ${marked.map(foldForSearch).join(" ")} `,
  };
}

function holds(line: Line, phrase: string): boolean {
  const plain = foldForSearch(phrase) === phrase;
  return (plain ? line.plain : line.marked).includes(` ${phrase} `);
}

/**
 * Whether the text in the search box reads as a question to ask rather than a
 * name to look up. At least three words, and either a closing question mark or
 * a question word where its language puts one. Three words keep a name such as
 * "Wo Holdings" out.
 */
export function looksLikeQuestion(query: string): boolean {
  const words = searchWords(query.replaceAll(/[?？]/g, " "));
  if (words.length < 3) {
    return false;
  }
  if (/[?？]\s*$/.test(query)) {
    return true;
  }
  const line = lineOf(query);
  const folded = line.plain.trim().split(" ");
  const [first = "", second = ""] = folded;
  const last = folded.at(-1) ?? "";
  return LANGUAGES.some(
    (language) =>
      language.openers.includes(first) ||
      (language.auxiliaries.includes(first) &&
        language.subjects.includes(second)) ||
      language.closers.includes(last) ||
      language.phrases.some((phrase) => holds(line, phrase)),
  );
}

// This selects the reader, never a query or a value; the server plans the question.
export function looksLikeDataQuestion(question: string): boolean {
  const line = lineOf(question);
  return LANGUAGES.some((language) =>
    language.data.some((phrase) => holds(line, phrase)),
  );
}
