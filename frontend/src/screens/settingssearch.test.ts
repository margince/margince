// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  SETTINGS_PAGES,
  type SettingsPage,
  type SettingsPageId,
} from "./settingscatalog";
import { settingsSearch } from "./settingssearch";

const t = (key: MessageKey) => translate("en", key);
const de = (key: MessageKey) => translate("de", key);

// Every page, as an admin holding everything sees them.
//
// Widened to the readonly array `settingsSearch` takes: `SETTINGS_PAGES` is a
// 28-element TUPLE, so a filtered subset is not assignable to it and every case
// that narrows the reader's pages would fail to compile.
const ALL: readonly SettingsPage[] = SETTINGS_PAGES;

function ids(
  query: string,
  pages: readonly SettingsPage[] = ALL,
  translator = t,
): SettingsPageId[] {
  return settingsSearch(query, pages, translator).map((hit) => hit.page.id);
}

describe("settingsSearch", () => {
  it("finds a page by its own name", () => {
    expect(ids("audit")).toContain("audit");
  });

  // The whole point of aliases: a reader searches for the THING they want to
  // change, not for the page that happens to hold it. Nobody looking for their
  // password types "account".
  it.each([
    ["password", "account"],
    ["gdpr", "privacy"],
    ["webhook", "integrations"],
    ["csv", "import"],
    ["licence", "seats"],
    ["stage", "pipelines"],
  ] as const)("finds the page that owns %s", (query, page) => {
    expect(ids(query)).toContain(page);
  });

  // A reader types a phrase as one thought, and its words live in different
  // fields — "email" in one alias, "signature" in another beside it. Demanding
  // the whole phrase be one substring answered nothing for exactly the queries
  // a contact composes naturally.
  it.each([
    ["email signature", "account"],
    ["lead source", "leads"],
    ["sign in", "authentication"],
  ] as const)("answers the phrase %p", (query, page) => {
    expect(ids(query)).toContain(page);
  });

  // Every word must land, or a reader who typed two words gets pages that
  // answer only one of them — which is a longer list, not a better one.
  it("answers nothing when only half the phrase matches", () => {
    expect(ids("audit kubernetes")).toEqual([]);
  });

  // "billing" used to reach Seats & license, which reads the entitlement the
  // deployment file sets and carries no control at all — a reader looking to
  // change what they pay would have been sent to a page that cannot.
  it("does not promise a page that cannot answer the word", () => {
    expect(ids("billing")).toEqual([]);
  });

  // The old vocabulary still works. A reader who learnt this product before the
  // redesign, or who has a bookmark, types the word they know.
  it("still answers the names the redesign replaced", () => {
    expect(ids("general")).toContain("company");
    expect(ids("users")).toContain("members");
    expect(ids("data model")).toContain("fields");
  });

  // Two pages can each honestly answer one word, and picking one would make the
  // search wrong for everybody who meant the other.
  it("offers both pages a shared word reaches", () => {
    const hits = ids("email");
    expect(hits).toContain("connections");
    expect(hits).toContain("capture");
  });

  // The page's own name outranks a word it merely answers to.
  it("puts a name match above an alias match", () => {
    const hits = ids("tags");
    expect(hits[0]).toBe("tags");
  });

  // A prefix is a stronger signal than a substring: somebody typing "au" is
  // starting a word, not naming a fragment.
  it("puts a prefix above a contained match", () => {
    const hits = settingsSearch("audit", ALL, t);
    expect(hits[0]?.page.id).toBe("audit");
  });

  // PERMISSION IS THE CALLER'S LIST. The search cannot widen it, because it
  // never asks who is reading — it is handed the pages the sidebar drew.
  it("finds nothing a reader's own page list does not contain", () => {
    const personal = ALL.filter((page) => page.group === "me");
    expect(ids("audit", personal)).toEqual([]);
    expect(ids("gdpr", personal)).toEqual([]);
    // …and still finds what IS theirs, so the empty results above are about the
    // missing pages rather than about a search that answers nothing.
    expect(ids("password", personal)).toContain("account");
  });

  // An empty query is not a request for everything. The sidebar is already the
  // list of everything, and repeating it under a search box would tell the
  // reader they had searched.
  it.each(["", "   "])("answers nothing for the empty query %p", (query) => {
    expect(ids(query)).toEqual([]);
  });

  it("answers nothing for a word no page holds", () => {
    expect(ids("kubernetes")).toEqual([]);
  });

  // A reader types what their keyboard gives them. Folding one direction only
  // would make the unaccented spelling miss the accented label.
  it("matches a German label with and without its diacritics", () => {
    const accented = de("settings.tab.reset");
    const plain = accented.normalize("NFD").replace(/\p{Diacritic}/gu, "");
    expect(ids(accented, ALL, de)).toContain("reset");
    expect(ids(plain, ALL, de)).toContain("reset");
  });

  // Case is not a signal either.
  it("ignores case", () => {
    expect(ids("AUDIT")).toContain("audit");
    expect(ids("AuDiT")).toContain("audit");
  });

  // Every hit carries where it lives, because "Tags" alone does not say whether
  // the reader has found the vocabulary or something else.
  it("says which group each hit sits under", () => {
    const [hit] = settingsSearch("audit", ALL, t);
    expect(hit?.group).toBe(translate("en", "settings.group.governance"));
    expect(hit?.label).toBe(translate("en", "settings.tab.audit"));
  });

  // A gate reading no pages would pass every claim above.
  it("has pages to search at all", () => {
    expect(ALL.length).toBeGreaterThan(20);
  });
});
