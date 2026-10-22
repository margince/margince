// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  SETTINGS_PAGES,
  type SettingsPage,
  type SettingsPageId,
} from "../../lib/settingscatalog";
import type { SettingName } from "./configurationentries";
import { settingsSearch } from "./settingssearch";

const t = (key: MessageKey) => translate("en", key);
const de = (key: MessageKey) => translate("de", key);
const vi = (key: MessageKey) => translate("vi", key);

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
    ["install", "account"],
    ["calendar", "meetings"],
    ["working hours", "meetings"],
    ["gdpr", "privacy"],
    ["webhook", "integrations"],
    ["csv", "import"],
    ["licence", "seats"],
    ["stage", "pipelines"],
    ["token", "authentication"],
    ["limit", "capture"],
  ] as const)("finds the page that owns %s", (query, page) => {
    expect(ids(query)).toContain(page);
  });

  // A reader types a phrase as one thought, and its words live in different
  // fields — "email" in one alias, "signature" in another beside it. Demanding
  // the whole phrase be one substring answered nothing for exactly the queries
  // a contact composes naturally.
  it.each([
    ["email signature", "account"],
    ["install app", "account"],
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

describe("settingsSearch over the settings a page draws", () => {
  it.each([
    ["data retention", "system-health", "operations.retention.label"],
    ["date format", "company", "installationSettings.dateFormat"],
    ["sync interval", "capture", "captureMailSync.interval.label"],
    ["two-step", "account", "mfa.title"],
  ] as const)("sends %p to %s and names the setting", (query, page, key) => {
    const hit = settingsSearch(query, ALL, t).find(
      (candidate) => candidate.page.id === page,
    );
    expect(hit?.section).toBe(t(key));
  });

  it.each([
    ["de", de, "system-health", "operations.retention.label"],
    ["vi", vi, "company", "installationSettings.timezone"],
  ] as const)("finds a %s setting by its own label", (_, words, page, key) => {
    const hit = settingsSearch(words(key), ALL, words).find(
      (candidate) => candidate.page.id === page,
    );
    expect(hit?.section).toBe(words(key));
  });

  it.each([
    ["Mui gio bao cao", "company"],
    ["khoang dong bo", "capture"],
  ] as const)(
    "matches the Vietnamese %p typed without its marks",
    (q, page) => {
      expect(ids(q, ALL, vi)).toContain(page);
    },
  );

  it("offers the company-context settings only where the card draws", () => {
    const find = (seesCompanyContext: boolean) =>
      settingsSearch("ideal customer", ALL, t, { seesCompanyContext }).map(
        (hit) => hit.page.id,
      );
    expect(find(true)).toContain("company");
    expect(find(false)).not.toContain("company");
  });

  // Automations is a page and a phrase on two others ("Automations that ran").
  it("puts a page-name hit above a section hit", () => {
    const hits = settingsSearch("automations", ALL, t);
    expect(hits[0]?.page.id).toBe("automations");
    expect(hits[0]?.section).toBeUndefined();
    const sectionHits = hits.slice(1).map((hit) => hit.page.id);
    expect(sectionHits).toEqual(
      expect.arrayContaining(["notifications", "system-health"]),
    );
  });

  it("names no setting when an alias already matched", () => {
    const hit = settingsSearch("working hours", ALL, t).find(
      (candidate) => candidate.page.id === "meetings",
    );
    expect(hit).toBeDefined();
    expect(hit?.section).toBeUndefined();
  });
});

// The names an operator holds from the docs, which only the server lists.
const SETTING_NAMES: readonly SettingName[] = [
  {
    key: "capture.auto_enrich_daily_cap",
    env: "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
  },
  { key: "http.port", env: "MARGINCE_HTTP_PORT" },
];

describe("settingsSearch by a setting's name", () => {
  function configurationHit(query: string, pages = ALL) {
    return settingsSearch(query, pages, t, undefined, SETTING_NAMES).find(
      (hit) => hit.page.id === "configuration",
    );
  }

  it.each([
    [
      "capture.auto_enrich_daily_cap",
      "capture.auto_enrich_daily_cap",
      "capture.auto_enrich_daily_cap",
    ],
    [
      "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
      "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP",
      "capture.auto_enrich_daily_cap",
    ],
    [" margince_http_port ", "margince_http_port", "http.port"],
  ])("opens Configuration on %j filtered to it", (query, filter, named) => {
    const hit = configurationHit(query);
    expect(hit?.filter).toBe(filter);
    expect(hit?.section).toBe(named);
  });

  // The prefix every variable shares names no one setting.
  it("opens Configuration on the variable prefix and names no setting", () => {
    const hit = configurationHit("MARGINCE_");
    expect(hit?.filter).toBe("MARGINCE_");
    expect(hit?.section).toBeUndefined();
  });

  it("ranks a setting's name below every page its words name", () => {
    const hits = settingsSearch("capture", ALL, t, undefined, SETTING_NAMES);
    expect(hits.at(-1)?.page.id).toBe("configuration");
    expect(hits.at(-1)?.filter).toBe("capture");
  });

  it("opens Configuration unfiltered when its own words match", () => {
    const hit = configurationHit("env");
    expect(hit).toBeDefined();
    expect(hit?.filter).toBeUndefined();
  });

  it("offers nothing for a name no setting holds", () => {
    expect(configurationHit("capture.no_such_setting")).toBeUndefined();
  });

  it("never offers Configuration to a reader who may not open it", () => {
    const pages = ALL.filter((page) => page.id !== "configuration");
    expect(configurationHit("MARGINCE_HTTP_PORT", pages)).toBeUndefined();
  });

  it.each(["env file", "environment variable", "variable"])(
    "finds Configuration by %j",
    (query) => {
      expect(ids(query)).toContain("configuration");
    },
  );
});
