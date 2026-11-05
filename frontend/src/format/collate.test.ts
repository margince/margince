// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  foldForMatch,
  foldForSearch,
  forReader,
  holdsEveryWord,
  searchWords,
  stable,
} from "./collate";

// The two orderings exist because neither one is right everywhere, so what is
// asserted here is the DIFFERENCE between them. A test that only checked each
// sorted something would pass with one function.

describe("forReader orders the way a reader of that locale expects", () => {
  it("puts an umlaut beside its base letter for a German reader", () => {
    // Code-unit order puts every accented vowel after Z, so `Äpfel` lands
    // after `Zebra` — the single most visible collation defect in this
    // product's German UI, and the reason this function exists.
    const names = ["Zebra", "Äpfel", "Apfel"];
    expect([...names].sort((a, b) => forReader(a, b, "de"))).toEqual([
      "Apfel",
      "Äpfel",
      "Zebra",
    ]);
    expect([...names].sort(stable)).toEqual(["Apfel", "Zebra", "Äpfel"]);
  });

  it("reads case as a difference in weight, not in identity", () => {
    // A reader scanning names does not expect every capital before every
    // lower-case letter, which is what code-unit order gives.
    const names = ["bravo", "Alpha", "alpha"];
    expect([...names].sort((a, b) => forReader(a, b, "en"))).toEqual([
      "alpha",
      "Alpha",
      "bravo",
    ]);
    expect([...names].sort(stable)).toEqual(["Alpha", "alpha", "bravo"]);
  });

  it("answers per locale, which is the whole point and also the cost", () => {
    // Swedish-style ordering is not what German gives; the same pair can order
    // differently for two readers. That is correct for a name and wrong for a
    // key, which is why `stable` is a separate function rather than a flag.
    expect(forReader("Apfel", "Äpfel", "de")).toBeLessThan(0);
    expect(forReader("a", "A", "en")).toBeLessThan(0);
  });
});

describe("stable orders identically for everyone", () => {
  it("is a total order with no locale in it", () => {
    expect(stable("a", "b")).toBeLessThan(0);
    expect(stable("b", "a")).toBeGreaterThan(0);
    expect(stable("a", "a")).toBe(0);
  });

  it("sorts an ISO timestamp chronologically, which is why a key may use it", () => {
    // ISO-8601 is designed so that lexicographic order IS chronological order.
    const stamps = [
      "2026-08-25T09:00:00Z",
      "2026-01-02T23:59:59Z",
      "2026-08-25T08:59:59Z",
    ];
    expect([...stamps].sort(stable)).toEqual([
      "2026-01-02T23:59:59Z",
      "2026-08-25T08:59:59Z",
      "2026-08-25T09:00:00Z",
    ]);
  });
});

describe("foldForMatch folds the same way on every machine", () => {
  it("folds a capital I to a dotted i, whatever the host locale is", () => {
    // `toLocaleLowerCase` under a Turkish locale yields the dotless `ı`, so a
    // needle folded there stops matching a haystack folded anywhere else.
    // Matching needs both sides to agree more than it needs either to be
    // linguistically right.
    expect(foldForMatch("ISTANBUL")).toBe("istanbul");
    expect(foldForMatch("Istanbul".toUpperCase())).toBe(
      foldForMatch("istanbul"),
    );
  });

  it("still folds the accented letters a search has to see through", () => {
    expect(foldForMatch("MÜLLER")).toBe("müller");
  });
});

describe("foldForSearch reads through what a keyboard leaves out", () => {
  it("drops diacritics and case", () => {
    expect(foldForSearch("Zurücksetzen")).toBe("zurucksetzen");
    expect(foldForSearch("Múi giờ báo cáo")).toBe("mui gio bao cao");
  });

  it("reads the Vietnamese đ, which has no decomposition, as d", () => {
    expect(foldForSearch("Khoảng đồng bộ")).toBe("khoang dong bo");
    expect(foldForSearch("ĐỒNG BỘ")).toBe("dong bo");
  });

  it("reads a full-width letter as the letter a reader types", () => {
    expect(foldForSearch("ＡＮＮＡ Müller")).toBe("anna muller");
  });

  it("matches when every word lands in some field", () => {
    const words = searchWords("dong  bo  khoang");
    expect(words).toEqual(["dong", "bo", "khoang"]);
    expect(holdsEveryWord(words, ["Khoảng", "đồng bộ (giây)"])).toBe(true);
    expect(holdsEveryWord(words, ["Khoảng"])).toBe(false);
  });
});
