// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { formatPercent } from "../../format/format";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyTable } from "./analytics.company";

afterEach(cleanup);

const rows = [
  { company_id: "o-1", raw_minor: 3_000_000, deal_count: 4, currency: "EUR" },
  { company_id: "o-2", raw_minor: 1_000_000, deal_count: 2, currency: "EUR" },
  {
    company_id: "o-2",
    raw_minor: 4_500_000_000,
    deal_count: 1,
    currency: "VND",
  },
];

function draw(complete: boolean, withheld = 0) {
  installFetchStub({
    "GET /companies/o-1": () =>
      jsonResponse({ id: "o-1", display_name: "Bär" }),
    "GET /companies/o-2": () =>
      jsonResponse({ id: "o-2", display_name: "Brandt" }),
  });
  render(
    <StoryProviders>
      <CompanyTable
        rows={rows}
        complete={complete}
        withheld={withheld}
        locale="en"
      />
    </StoryProviders>,
  );
}

function shareColumn(): string[] {
  const table = screen.getByRole("table");
  const header = within(table).getByRole("columnheader", {
    name: "Share of open pipeline",
  });
  const index = within(table).getAllByRole("columnheader").indexOf(header);
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("cell")[index]?.textContent ?? "");
}

it("states each company's share of the open pipeline in its own currency", () => {
  draw(true);
  expect(shareColumn()).toEqual([
    formatPercent(0.75, "en"),
    formatPercent(0.25, "en"),
    formatPercent(1, "en"),
  ]);
});

it("states no share for a capped answer, which is missing part of the whole", () => {
  draw(false);
  expect(shareColumn()).toEqual(["—", "—", "—"]);
});

it("states no share when the reader may not see some deals, whose value is part of the whole", () => {
  draw(true, 2);
  expect(shareColumn()).toEqual(["—", "—", "—"]);
});
