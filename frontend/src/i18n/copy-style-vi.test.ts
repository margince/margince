// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  catalogEntries,
  type LocaleEntry,
  localeCatalogs,
} from "../../scripts/lib/locale-catalogs";
import { sourceFileAt } from "../../scripts/lib/source-tree";
import { vi } from "./vi";

// The mechanical half of docs/reference/ui-copy-style-vi.md. Each rule lists
// every offender, so one run names all the strings a rewrite has to touch.

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..", "..", "..");

const VIETNAMESE = localeCatalogs("vi", vi);
const entries = () => catalogEntries(VIETNAMESE);

// Read from source, not imported: importing a test file runs its tests here.
function declaredStrings(path: string, name: string): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === name &&
      node.initializer
    ) {
      found.push(...literalsOf(node.initializer));
    }
    ts.forEachChild(node, visit);
  };
  visit(sourceFileAt(path));
  if (found.length === 0) {
    throw new Error(`${path} no longer declares ${name}; this gate reads it`);
  }
  return found;
}

function literalsOf(initializer: ts.Expression): string[] {
  if (ts.isObjectLiteralExpression(initializer)) {
    return initializer.properties.flatMap((property) =>
      property.name && ts.isStringLiteral(property.name)
        ? [property.name.text]
        : [],
    );
  }
  const found: string[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isStringLiteral(node)) found.push(node.text);
    ts.forEachChild(node, visit);
  };
  visit(initializer);
  return found;
}

function declaredString(path: string, name: string): string {
  const [only, ...rest] = declaredStrings(path, name);
  if (only === undefined || rest.length > 0) {
    throw new Error(`${path}: ${name} no longer holds one string`);
  }
  return only;
}

const ENGLISH_GATE = resolve(here, "copy-style.test.ts");
const AGENT_SPEAKS_PREFIX = declaredString(ENGLISH_GATE, "AGENT_SPEAKS_PREFIX");
const CONTROLLER_SPEAKS_PREFIX = declaredString(
  ENGLISH_GATE,
  "CONTROLLER_SPEAKS_PREFIX",
);
const SPOKEN_BY_THE_READER = new Set(
  declaredStrings(ENGLISH_GATE, "SPOKEN_BY_THE_READER"),
);
const SPOKEN_BY_THE_READER_PREFIX = declaredString(
  ENGLISH_GATE,
  "SPOKEN_BY_THE_READER_PREFIX",
);

function spokenByTheReader(key: string): boolean {
  return (
    SPOKEN_BY_THE_READER.has(key) || key.startsWith(SPOKEN_BY_THE_READER_PREFIX)
  );
}

// German's Sie families, plus the public booking form a guest fills in, which
// Vietnamese alone addresses as an outsider.
const READ_BY_AN_OUTSIDER = [
  ...declaredStrings(
    resolve(here, "address-register.test.ts"),
    "READ_BY_AN_OUTSIDER",
  ),
  "book.",
];

function readByAnOutsider(key: string): boolean {
  return READ_BY_AN_OUTSIDER.some((prefix) => key.startsWith(prefix));
}

// The server publishes these consent answers and a proof records their exact
// wording, so a fix starts in the server copy, not here.
function serverWordedKeys(): Set<string> {
  const gate = readFileSync(
    resolve(repoRoot, "backend", "gates", "marketingquestion_test.go"),
    "utf8",
  );
  const keys = [...gate.matchAll(/\{"([\w.]+)", "Confirm\w+"\}/g)].map(
    ([, key]) => key,
  );
  if (keys.length === 0) {
    throw new Error("marketingquestion_test.go no longer pairs screen keys");
  }
  return new Set(keys);
}
const SERVER_WORDED = serverWordedKeys();

function judged(): LocaleEntry[] {
  return entries().filter(([, key]) => !SERVER_WORDED.has(key));
}

function offenders(breaks: (entry: LocaleEntry) => boolean): string[] {
  return judged()
    .filter(breaks)
    .map(
      ([source, key, value]) => `${source} ${key}: ${JSON.stringify(value)}`,
    );
}

function matching(
  pattern: RegExp,
  options: {
    text?: (value: string) => string;
    exempt?: (entry: LocaleEntry) => boolean;
  } = {},
): string[] {
  const { text = (value: string) => value, exempt = () => false } = options;
  return offenders((entry) => !exempt(entry) && pattern.test(text(entry[2])));
}

// A placeholder names a parameter, not copy. A space keeps the words on either
// side from reading as one.
function prose(value: string): string {
  return value.replace(/\{[^}]*\}/g, " ");
}

// A vendor's label quoted in “…” stays exactly as the English quotes it.
function unquoted(value: string): string {
  return prose(value).replace(/“[^“”]*”/g, " ");
}

function word(alternation: string, flags = "iu"): RegExp {
  return new RegExp(`(?<![\\p{L}])(?:${alternation})(?![\\p{L}])`, flags);
}

function escaped(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Owned by the Vocabulary table's Never column on the Vietnamese style page,
// which sets each of these in code format; the last test holds the two equal.
const RETIRED_WORDS = [
  "thương vụ",
  "đầu mối",
  "phễu",
  "worklist",
  "hàng đợi công việc",
  "bản tóm tắt buổi sáng",
  "briefing",
  "chấp thuận",
  "hộ chiếu",
  "bộ kết nối",
  "connector",
  "phòng deal",
  "spine",
  "backread",
  "đọc sâu",
  "deep read",
  "phán quyết",
  "verdict",
  "admission check",
  "phần của bản ghi",
  "lời hứa",
  "dự phóng",
  "forecast",
  "trường hợp tốt nhất",
  "khả quan nhất",
  "TTNT",
  "login",
  "log in",
  "đăng nhập vào trong",
  "thử lần nữa",
  "upload",
  "download",
  "thư điện tử",
  "e-mail",
  "mailbox",
  "buổi họp",
  "meeting",
  "task",
  "tag",
  "report",
  "định mức",
  "chỗ ngồi",
  "ghế",
  "legal hold",
  "buying center",
  "nhóm mua hàng",
  "thực thể pháp lý",
  "nhật ký kiểm toán",
  "audit log",
  "audit trail",
  "follow-up",
  "chào giá",
  "ngày đóng",
  "file",
  "website",
  "server",
  "lí do",
  "chứng cứ",
  "tác tử",
  "bản ghi lời",
  "làm giàu dữ liệu",
  "điểm cuối",
  "nhật ký kiểm tra",
  "dừng giữa chừng",
  "quản trị viên hệ thống",
  "nhờ quản trị viên",
  "lưu giữ pháp lý",
  "suất",
];

// The compounds where a retired word keeps an ordinary sense: hiệu suất is
// performance, not a seat.
const ORDINARY_COMPOUNDS = word(
  "(?:hiệu|xác|tần|thuế|lãi|năng|công|áp) suất",
  "giu",
);

// A file name or a URL is typed exactly as the reader must type it.
function withoutPaths(value: string): string {
  return unquoted(value).replace(
    /[^\s“”(),]*(?:\/|\.[a-z])[^\s“”(),]*/giu,
    " ",
  );
}

// An English loanword is retired in its plural too: "tasks" is "task".
const RETIRED = word(
  RETIRED_WORDS.map(
    (retired) =>
      `${escaped(retired)}${/^[\x20-\x7e]+$/.test(retired) ? "s?" : ""}`,
  ).join("|"),
);

function gatedNeverWords(): string[] {
  return [...vocabularyNeverColumn().matchAll(/`([^`]+)`/g)].map(
    ([, gated]) => gated,
  );
}

function vocabularyNeverColumn(): string {
  const page = readFileSync(
    resolve(repoRoot, "docs", "reference", "ui-copy-style-vi.md"),
    "utf8",
  );
  const section = page.split("\n## Vocabulary\n")[1]?.split("\n## ")[0] ?? "";
  return section
    .split("\n")
    .filter((line) => line.startsWith("|") && !line.startsWith("|---"))
    .slice(1)
    .map((row) => row.split("|")[3] ?? "")
    .join("\n");
}

// Reflexive "mình" refers back to a third party, not to the product.
const REFLEXIVE = word("(?:của|chính|riêng|tự|một|chỉ) mình", "giu");

// What can stand before a capital that opens a sentence. `\}\s` because a
// placeholder's value may itself end a sentence.
const SENTENCE_OPENS = /(?:[.?:]\s|\n|[“(]|\}\s)$/;

function placeholderSet(value: string): string {
  return (value.match(/\{\w+\}/g) ?? []).sort().join();
}

function catalogOf(source: string): Readonly<Record<string, string>> {
  return VIETNAMESE.find((copy) => copy.source === source)?.catalog ?? {};
}

function englishOf(source: string): Readonly<Record<string, string>> {
  return VIETNAMESE.find((copy) => copy.source === source)?.english ?? {};
}

describe("vi copy style", () => {
  it("is NFC", () => {
    expect(
      offenders(([, , value]) => value !== value.normalize("NFC")),
    ).toEqual([]);
  });

  it("marks the tone of an open oa, oe and uy on the first vowel", () => {
    const newStyle =
      /(?<!q)o[àáảãạ](?![\p{L}])|o[èéẻẽẹ](?![\p{L}])|(?<!q)u[ỳýỷỹỵ](?![\p{L}])/iu;
    expect(matching(newStyle, { text: prose })).toEqual([]);
  });

  it("marks the tone on the second vowel before a final consonant", () => {
    const misplaced = /[òóỏõọ][ae]\p{L}|(?<!q)[ùúủũụ]y\p{L}/iu;
    expect(matching(misplaced, { text: prose })).toEqual([]);
  });

  it("uses no en dash, em dash, double hyphen or spaced hyphen", () => {
    expect(
      matching(/[–—]|--|\s-\s/, { text: (value) => ` ${prose(value)} ` }),
    ).toEqual([]);
  });

  it("spells an ellipsis as the one character …", () => {
    expect(matching(/\.\.\./)).toEqual([]);
  });

  it("uses no exclamation mark", () => {
    expect(matching(/!/)).toEqual([]);
  });

  it("quotes with “…” and ’ only", () => {
    expect(matching(/["„»«]|\p{L}'\p{L}/u)).toEqual([]);
  });

  // Edge whitespace is not checked: some values are fragments the code joins
  // around markup, and their leading or trailing space is load-bearing.
  it("has no double space", () => {
    expect(matching(/ {2}/)).toEqual([]);
  });

  it("uses no abbreviation or ampersand", () => {
    expect(
      matching(new RegExp(`${word("v\\.v\\.|vd\\.|v/v").source}|&`, "iu"), {
        text: unquoted,
      }),
    ).toEqual([]);
  });

  // Where the English arms carry different placeholders, i18n.test.ts makes
  // the Vietnamese arms differ too, and the page owns that shape.
  it("writes the _one arm as the _other arm", () => {
    expect(
      offenders(([source, key, value]) => {
        if (!key.endsWith("_one")) return false;
        const otherKey = key.replace(/_one$/, "_other");
        const other = catalogOf(source)[otherKey];
        const english = englishOf(source);
        return (
          other !== undefined &&
          placeholderSet(english[key] ?? "") ===
            placeholderSet(english[otherKey] ?? "") &&
          value !== other
        );
      }),
    ).toEqual([]);
  });

  // "Giao cho tôi" and "Deal của tôi" name the reader in a control, as the
  // English "Assign to me" and "My deals" do.
  it("says tôi only for the onboarding agent, a consent statement or an English me", () => {
    expect(
      matching(word("tôi"), {
        text: (value) => prose(value).replace(word("chúng tôi", "giu"), " "),
        exempt: ([, key, , english]) =>
          key.startsWith(AGENT_SPEAKS_PREFIX) ||
          spokenByTheReader(key) ||
          /\b(?:me|my|mine)\b/i.test(english),
      }),
    ).toEqual([]);
  });

  it("says chúng tôi only as the data controller or in a consent statement", () => {
    expect(
      matching(word("chúng tôi"), {
        text: prose,
        exempt: ([, key]) =>
          key.startsWith(CONTROLLER_SPEAKS_PREFIX) || spokenByTheReader(key),
      }),
    ).toEqual([]);
  });

  it("never speaks as chúng ta or mình", () => {
    expect(
      matching(word("chúng ta|mình"), {
        text: (value) => prose(value).replace(REFLEXIVE, " "),
        exempt: ([, key]) => spokenByTheReader(key),
      }),
    ).toEqual([]);
  });

  it("never addresses anyone as quý vị, anh/chị or kính thưa", () => {
    expect(
      matching(word("quý vị|anh/chị|anh chị|kính thưa"), { text: prose }),
    ).toEqual([]);
  });

  it("says quý khách and vui lòng only where an outsider reads", () => {
    expect(
      matching(word("quý khách|vui lòng"), {
        text: prose,
        exempt: ([, key]) => readByAnOutsider(key),
      }),
    ).toEqual([]);
  });

  it("never says bạn where an outsider reads", () => {
    expect(
      matching(word("bạn"), {
        text: prose,
        exempt: ([, key]) => !readByAnOutsider(key),
      }),
    ).toEqual([]);
  });

  it("capitalises bạn and quý khách only where a sentence opens", () => {
    expect(
      offenders(([, , value]) =>
        [...value.matchAll(/(?<![\p{L}])(?:Bạn|Quý khách)(?![\p{L}])/gu)].some(
          (hit) =>
            hit.index > 0 && !SENTENCE_OPENS.test(value.slice(0, hit.index)),
        ),
      ),
    ).toEqual([]);
  });

  it("every outsider family names a key the catalogs carry", () => {
    expect(
      READ_BY_AN_OUTSIDER.filter(
        (prefix) => !entries().some(([, key]) => key.startsWith(prefix)),
      ),
    ).toEqual([]);
  });

  it("uses no retired word", () => {
    expect(
      matching(RETIRED, {
        text: (value) => withoutPaths(value).replace(ORDINARY_COMPOUNDS, " "),
      }),
    ).toEqual([]);
  });

  // A short value is a title, label, status or counter, where a failure is
  // "bị lỗi" or "không thành công".
  it("says thất bại only in a sentence, never in a short value", () => {
    const wordsIn = (value: string) =>
      prose(value)
        .split(/\s+/)
        .filter((token) => /\p{L}/u.test(token)).length;
    expect(
      offenders(
        ([, , value]) => word("thất bại").test(value) && wordsIn(value) <= 8,
      ),
    ).toEqual([]);
  });

  it("retires exactly the words the Vocabulary table sets in code format", () => {
    const gated = gatedNeverWords();
    expect(gated.length).toBeGreaterThan(0);
    expect(RETIRED_WORDS.filter((retired) => !gated.includes(retired))).toEqual(
      [],
    );
    expect(gated.filter((never) => !RETIRED_WORDS.includes(never))).toEqual([]);
  });
});
