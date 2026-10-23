/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { RecordCustomFields } from "../components/recordcustomfields";
import { customFieldFixture } from "../components/recordcustomfields.stories";
import { CustomFieldsPanel } from "../features/companies/customfields.card";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

afterEach(cleanup);

const budget = {
  ...customFieldFixture,
  id: "f3",
  label: "Annual budget",
  slug: "budget",
  type: "currency" as const,
  currency: "EUR",
  column_name: "cf_budget",
};
const wiki = {
  ...customFieldFixture,
  id: "f4",
  label: "Account wiki",
  slug: "wiki",
  column_name: "cf_wiki",
};
const WIKI = "https://wiki.example.com/globex";

function catalog(fields: unknown[]) {
  installFetchStub({
    "GET /me": meRoute({ deal: ["read", "update"] as const }, { seat: "full" }),
    "GET /custom-fields": () =>
      jsonResponse({ data: fields, page: { has_more: false } }),
  });
}

// The record's own details and the contracts panel are two writers of one
// reading. Each type of wire value must read the same in both.
function typed(
  type: (typeof customFieldFixture)["type"],
  label: string,
  options?: string[],
) {
  return {
    ...customFieldFixture,
    id: `f-${type}`,
    label,
    type,
    options,
    column_name: `cf_${type}`,
  };
}
const EVERY_TYPE = [
  customFieldFixture,
  wiki,
  budget,
  typed("number", "Fleet"),
  typed("date", "Renewal"),
  typed("picklist", "Region", ["North", "West"]),
  typed("multiselect", "Depots", ["North", "West"]),
  typed("boolean", "Framework"),
].map((field) => ({ ...field, object: "deal" as const }));
// One value per type in the shape the record API sends it.
const WIRE = {
  cf_tier: "Strategic",
  cf_wiki: WIKI,
  cf_budget: 4_800_000,
  cf_number: 220,
  cf_date: "2026-03-01",
  cf_picklist: "North",
  cf_multiselect: ["North", "West"],
  cf_boolean: true,
};

it.each([false, true])(
  "reads every type's wire value as the projects panel does (writable: %s)",
  async (writable) => {
    catalog(EVERY_TYPE);
    render(
      <StoryProviders>
        <div data-testid="details">
          <RecordCustomFields
            kind="deal"
            record={{ id: "record1", version: 1, writable, ...WIRE }}
          />
        </div>
        <div data-testid="panel">
          <CustomFieldsPanel object="deal" record={WIRE} />
        </div>
      </StoryProviders>,
    );
    const details = within(screen.getByTestId("details"));
    const panel = within(screen.getByTestId("panel"));
    await details.findByText("Customer tier");
    await panel.findByText("Customer tier");
    const readings = EVERY_TYPE.map(({ label }) => {
      const cell = details.getByText(label).nextElementSibling;
      return writable
        ? cell?.firstElementChild?.textContent
        : cell?.textContent;
    });
    expect(readings).toEqual(
      EVERY_TYPE.map(
        ({ label }) => panel.getByText(label).nextElementSibling?.textContent,
      ),
    );
    expect(readings.slice(1, 3)).toEqual([WIKI, "€48,000.00"]);
  },
);
