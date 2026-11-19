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
  // A phrase that marks a question at the start or the end of the text.
  readonly edges: readonly string[];
  // A word that marks a question at the end: "có đạt mục tiêu không".
  readonly closers: readonly string[];
  // A phrase that marks a question wherever it stands: "tôi thắng bao nhiêu deal".
  readonly anywhere: readonly string[];
  // A phrase that asks for a figure from CRM data rather than the handbook.
  readonly data: readonly string[];
};

// "mấy" also means "a few" ("mấy tháng gần đây"), so it asks a count only
// before a counted noun. `backend/gates/vietnamesedatawords_test.go` holds the
// server's won-count check to these phrases.
const VIETNAMESE_COUNTS = [
  "bao nhieu",
  "mấy deal",
  "mấy khach hang",
  "mấy cong ty",
  "mấy lead",
  "mấy du an",
  "mấy hop dong",
];

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
    edges: [],
    closers: [],
    anywhere: [],
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
      "reach the target",
      "reach our target",
      "hit the target",
      "hit our target",
      "meet the target",
      "hit quota",
      "hit our quota",
      "make quota",
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
    edges: [],
    closers: [],
    anywhere: [],
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
      "ziel erreicht",
      "ziel erreichen",
    ],
  },
  // A plain phrase has two words, because the single words (lam, tai, bao,
  // khi, ai) are also given names, surnames or "AI".
  vi: {
    openers: [],
    auxiliaries: [],
    subjects: [],
    edges: [
      "lam sao",
      "tai sao",
      "vi sao",
      "bao gio",
      "khi nao",
      "cach nao",
      "o dau",
      "the nao",
      "la gi",
      "ai đã",
      "ai là",
      "ai đang",
      "của ai",
      "cho ai",
      "với ai",
    ],
    closers: ["khong", "chua", "mấy", "gì", "nào"],
    anywhere: VIETNAMESE_COUNTS,
    data: [
      ...VIETNAMESE_COUNTS,
      "tổng",
      "doanh thu",
      "thắng deal",
      "deal thắng",
      "đạt mục tiêu",
      "đạt chỉ tiêu",
      "vượt mục tiêu",
    ],
  },
} satisfies Record<Locale, QuestionWords>;

const LANGUAGES: readonly QuestionWords[] = Object.values(QUESTION_WORDS);

// The words as typed (lower case), folded, and whether each was capitalized.
type Words = {
  readonly marked: readonly string[];
  readonly plain: readonly string[];
  readonly capital: readonly boolean[];
};

function wordsOf(text: string): Words {
  const typed = text
    .normalize("NFC")
    .split(/[^\p{L}\p{M}\p{N}]+/u)
    .filter((word) => word !== "");
  const marked = typed.map((word) => word.toLowerCase());
  return {
    marked,
    plain: marked.map(foldForSearch),
    capital: typed.map((word) => word !== word.toLowerCase()),
  };
}

// Every place `phrase` stands in the text. A word with marks is matched as
// typed, a plain one on the folded text.
function positionsOf(words: Words, phrase: string): number[] {
  const parts = phrase.split(" ");
  const found: number[] = [];
  for (let at = 0; at + parts.length <= words.marked.length; at++) {
    const same = parts.every((part, offset) =>
      foldForSearch(part) === part
        ? words.plain[at + offset] === part
        : words.marked[at + offset] === part,
    );
    if (same) found.push(at);
  }
  return found;
}

// A name capitalizes every word: "Bao Gio Nguyen", "Trần Thị Nào", "Who Gives
// A Crap". At the start the word after the match must be capitalized too,
// because a sentence capitalizes its first word.
function isName(words: Words, at: number, length: number): boolean {
  const matched = words.capital.slice(at, at + length).every(Boolean);
  return at > 0 ? matched : matched && words.capital[at + length] !== false;
}

function marksAt(
  words: Words,
  phrases: readonly string[],
  where: (at: number, length: number) => boolean,
): boolean {
  return phrases.some((phrase) => {
    const length = phrase.split(" ").length;
    return positionsOf(words, phrase).some(
      (at) => where(at, length) && !isName(words, at, length),
    );
  });
}

/**
 * Whether the text in the search box reads as a question to ask rather than a
 * name to look up. At least three words, and either a closing question mark or
 * a question word where its language puts one. Three words keep a name such as
 * "Wo Holdings" out.
 */
export function looksLikeQuestion(query: string): boolean {
  if (searchWords(query.replaceAll(/[?？]/g, " ")).length < 3) {
    return false;
  }
  if (/[?？]\s*$/.test(query)) {
    return true;
  }
  const words = wordsOf(query);
  const count = words.marked.length;
  const start = (at: number) => at === 0;
  const end = (at: number, length: number) => at + length === count;
  return LANGUAGES.some(
    (language) =>
      marksAt(words, language.openers, start) ||
      (language.auxiliaries.includes(words.plain[0] ?? "") &&
        language.subjects.includes(words.plain[1] ?? "")) ||
      marksAt(words, language.edges, (at, n) => start(at) || end(at, n)) ||
      marksAt(words, language.closers, end) ||
      marksAt(words, language.anywhere, () => true),
  );
}

// This selects the reader, never a query or a value; the server plans the question.
export function looksLikeDataQuestion(question: string): boolean {
  const words = wordsOf(question);
  return LANGUAGES.some((language) =>
    language.data.some((phrase) => positionsOf(words, phrase).length > 0),
  );
}
