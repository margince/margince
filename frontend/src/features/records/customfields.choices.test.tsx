// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { jsonResponse } from "../../testing/story-utils";
import {
  CustomFieldChoices,
  cleanChoices,
  recordsLosingAChoice,
} from "./customfields.choices";

type CustomField = components["schemas"]["CustomField"];

const field: CustomField = {
  id: "01a0f000-0000-7000-8000-0000000000f2",
  object: "deal",
  label: "Sales region",
  slug: "sales_region",
  type: "picklist",
  options: ["North", "West"],
  status: "active",
  column_name: "cf_sales_region",
  show_in_create: true,
  reportable: false,
  created_by: "00000000-0000-4000-8000-000000000001",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the choices editor", () => {
  it("counts only the records holding a choice the edit drops", () => {
    expect(recordsLosingAChoice({ North: 3, West: 2 }, ["West"])).toBe(3);
    expect(recordsLosingAChoice({ North: 3, West: 2 }, ["North", "West"])).toBe(
      0,
    );
    expect(cleanChoices([" North ", "", "North", "East"])).toEqual([
      "North",
      "East",
    ]);
  });

  it("says before Save how many records lose a removed choice", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          choices: [
            { choice: "North", records: 12 },
            { choice: "West", records: 0 },
          ],
        }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <CustomFieldChoices field={field} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    fireEvent.click(
      screen.getByRole("button", { name: englishCopy["cf.choices.edit"] }),
    );
    expect(await screen.findByText("12 records")).toBeInTheDocument();
    expect(screen.queryByText(/Saving clears/)).not.toBeInTheDocument();
    fireEvent.click(
      screen.getAllByRole("button", {
        name: englishCopy["cf.removeOption"],
      })[0],
    );
    expect(
      await screen.findByText(
        "Saving clears the removed choices from 12 records.",
      ),
    ).toBeInTheDocument();
  });
});
