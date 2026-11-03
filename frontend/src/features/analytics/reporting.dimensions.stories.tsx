// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { reportingStoryEvaluation } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DimensionChips, DimensionFilters } from "./reporting.dimensions";
import type { ReportingSelection } from "./reporting.model";

// The dials that narrow a report to a product, an initiative or a market, and
// the chips that say which are set: none, every one, and a product the catalog
// cannot name.

const meta: Meta = {
  title: "Analytics/Reporting/Dimensions",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const AT = "2026-10-01T09:00:00Z";
const base: ReportingSelection = reportingStoryEvaluation.selection;
const everyDimension: ReportingSelection = {
  ...base,
  product_id: "p-suite",
  initiative_id: "i-dach",
  country: "DE",
  industry: "logistics_transport",
  value_basis: "software",
};

function catalog(products: readonly { id: string; name: string }[]) {
  installFetchStub({
    "GET /products": () =>
      jsonResponse({
        data: products,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /analytics/initiatives": () =>
      jsonResponse({
        data: [
          {
            id: "i-dach",
            name: "DACH expansion",
            owner_id: "maya",
            objective_metric: "bookings_won",
            deadline: "2026-12-31",
            status: "active",
            include_future: false,
            version: 1,
            created_at: AT,
            updated_at: AT,
          },
        ],
      }),
  });
}

const PRODUCTS = [{ id: "p-suite", name: "Suite" }];

function Filters({ initial }: Readonly<{ initial: ReportingSelection }>) {
  const [selection, setSelection] = useState(initial);
  return <DimensionFilters selection={selection} onChange={setSelection} />;
}

function Chips({ initial }: Readonly<{ initial: ReportingSelection }>) {
  const [selection, setSelection] = useState(initial);
  return <DimensionChips selection={selection} onChange={setSelection} />;
}

/** No dimension set: every dial reads "all". */
export const FiltersWithNoDimension: Story = {
  render: () => {
    catalog(PRODUCTS);
    return (
      <StoryProviders>
        <Filters initial={base} />
      </StoryProviders>
    );
  },
};

/** Every dimension set. */
export const FiltersWithEveryDimension: Story = {
  render: () => {
    catalog(PRODUCTS);
    return (
      <StoryProviders>
        <Filters initial={everyDimension} />
      </StoryProviders>
    );
  },
};

/** A chip for each dimension set, each removable on its own. */
export const ChipsForEveryDimension: Story = {
  render: () => {
    catalog(PRODUCTS);
    return (
      <StoryProviders>
        <Chips initial={everyDimension} />
      </StoryProviders>
    );
  },
};

/** A product the catalog no longer lists reads as unnamed rather than as its id. */
export const ChipForAProductTheCatalogCannotName: Story = {
  render: () => {
    catalog([]);
    return (
      <StoryProviders>
        <Chips initial={{ ...base, product_id: "p-retired" }} />
      </StoryProviders>
    );
  },
};
