import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { standardViews } from "../../components/recordlist";
import { translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { dealSurfaceChips } from "./deal360/dealfilterchips";
import { dealFilterChips } from "./deals.listtools";
import { dealRangeChips } from "./deals.rangefilters";

// The handbook's table names the Deals screen's tabs and filters. Each name it
// types must be one the screen offers. Each one the screen offers must be there.

const t = (key: MessageKey) => translate("en", key);

// The owner, team and tag chips come from hooks, so they are named by label key.
// deals.list.tsx puts owner and team first, and tags after stage and company.
const offeredFilters = [
  t("list.owner"),
  t("list.team"),
  ...dealFilterChips([], t, undefined).map((chip) => chip.label),
  t("tags.columnHeader"),
  ...dealRangeChips({
    words: { t, locale: "en", zone: "UTC" },
    filters: {},
    baseCurrency: "EUR",
    withheld: new Set(),
    leading: ["EUR"],
  }).map((chip) => chip.label),
  // Every conditional chip is offered: each has the options it waits for.
  ...dealSurfaceChips({
    partnerOptions: [{ value: "partner-1", label: "Northwind" }],
    acquisitionSources: [
      {
        id: "src-1",
        key: "outbound",
        label: "Outbound",
        sort_order: 1,
        active: true,
        system: true,
        version: 1,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
    ],
    linkTypes: [{ name: "renewal_of", label: "Renewal of", from: [], to: [] }],
    retiredSuffix: "",
  }).map((chip) => t(chip.label)),
];

const offeredTabs = standardViews("viewer-1", { sort: "" }).map((view) =>
  t(view.label),
);

function handbookDealsRow(): { filters: string[]; tabs: string[] } {
  const here = dirname(fileURLToPath(import.meta.url));
  const page = readFileSync(
    resolve(
      here,
      ...Array(4).fill(".."),
      "docs",
      "handbook",
      "lists-filters-and-views.md",
    ),
    "utf8",
  );
  const header = page.split("\n").find((line) => line.startsWith("| List |"));
  const row = page.split("\n").find((line) => line.startsWith("| **Deals** |"));
  if (!header || !row) {
    throw new Error("The handbook page has no list table with a Deals row.");
  }
  const cells = (line: string) =>
    line
      .split("|")
      .slice(1, -1)
      .map((cell) => cell.trim());
  const column = (name: string) =>
    (cells(row)[cells(header).indexOf(name)] ?? "")
      .split(", ")
      .map((item) => item.replaceAll("**", ""));
  return { filters: column("Filters"), tabs: column("Tabs") };
}

describe("the handbook's Deals row", () => {
  it("names every filter the Deals screen offers, in its order", () => {
    expect(handbookDealsRow().filters).toEqual(offeredFilters);
  });

  it("names the Deals screen's tabs", () => {
    expect(handbookDealsRow().tabs).toEqual(offeredTabs);
  });
});
