// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { en } from "../../i18n/en";
import type { FilterPreview, VocabularyField } from "../../lib/filterdata";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { FilterResults } from "./filterresults";

// FilterResults takes the preview response and the vocabulary as props. Only a
// column that points at another record fetches, to name it; StoryProviders
// gives those reads their query client and every string its locale.
//
// The three stories are the three things a reader can be looking at: rows behind
// a count, a filter that selected nothing, and the recount that has not answered
// yet. The middle one matters most, because "nothing matched" and "something
// failed" look identical in a table that only knows how to be empty.
const meta: Meta<typeof FilterResults> = {
  title: "Patterns/Filter results",
  component: FilterResults,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

const FIELDS: readonly VocabularyField[] = [
  {
    name: "full_name",
    type: "text",
    operators: ["eq", "neq", "in", "contains", "exists"],
    custom: false,
  },
  {
    name: "cf_loyalty_tier",
    type: "picklist",
    operators: ["eq", "neq", "in", "exists"],
    custom: true,
  },
];

const COLUMNS = ["id", "full_name", "city", "cf_loyalty_tier", "created_at"];

/** A page of rows, one of them missing the value the filter asked about. */
const ROWS = [
  {
    id: "p-1",
    full_name: "Ann Lee",
    city: "Berlin",
    cf_loyalty_tier: "gold",
    created_at: "2026-08-01",
  },
  {
    id: "p-2",
    full_name: "Bruno Sá",
    city: "Lisbon",
    cf_loyalty_tier: "silver",
    created_at: "2026-07-19",
  },
  {
    // The dash case, which is the reason a null does not render as blank: a
    // reader checking an `exists` clause has to be able to see the absence.
    id: "p-3",
    full_name: "Chen Wei",
    city: "Singapore",
    cf_loyalty_tier: null,
    created_at: "2026-06-30",
  },
] satisfies FilterPreview["rows"];

function preview(rows: FilterPreview["rows"]): FilterPreview {
  return {
    resource: "contact",
    match_count: rows.length,
    columns: COLUMNS,
    rows,
    truncated: false,
  };
}

type Story = StoryObj<typeof FilterResults>;

const shared = {
  fields: FIELDS,
  // The filter asked about tier, so tier is a column — the derivation this
  // component exists to make, rather than a fixed product column list.
  named: ["cf_loyalty_tier"],
  unit: "contacts",
  widthsKey: "story-filter-preview",
  pending: false,
};

export const Rows: Story = {
  args: { ...shared, preview: preview(ROWS) },
};

export const NothingMatched: Story = {
  args: {
    ...shared,
    preview: preview([]),
    emptyNote: en["filters.noMatches"].replace("{records}", "contacts"),
  },
};

export const Recounting: Story = {
  // The state between an edit and its answer. The rows on screen are the PREVIOUS
  // filter's, which is why the screen marks the count stale rather than blanking
  // it — a table that empties on every keystroke is unreadable.
  args: { ...shared, preview: preview(ROWS), pending: true },
};

const PAGE = Array.from({ length: 25 }, (_, index) => ({
  id: `p-${index}`,
  full_name: `Contact ${index + 1}`,
  city: "Berlin",
  cf_loyalty_tier: "gold",
  created_at: "2026-08-01",
})) satisfies FilterPreview["rows"];

export const FirstPageOfMany: Story = {
  // 214 match and 25 came: the count line says 214, the page holds the 25,
  // and the size dial asks the server again rather than paging past them.
  args: {
    ...shared,
    preview: { ...preview(PAGE), match_count: 214, truncated: true },
    total: 214,
    perPage: 25,
    onPerPage: () => {},
  },
};

const DEAL_FIELDS: readonly VocabularyField[] = [
  { name: "owner_id", type: "id", operators: ["eq"], references: "app_user" },
  { name: "stage_id", type: "id", operators: ["eq"], references: "stage" },
  { name: "company_id", type: "id", operators: ["eq"], references: "company" },
];

const DEAL_ROWS = [
  {
    id: "d-1",
    name: "Brandt renewal",
    owner_id: "u-1",
    stage_id: "s-1",
    company_id: "c-1",
  },
  {
    // Ids the workspace no longer holds: a removed seat, a deleted stage and
    // company. They read as gone, never as a raw id.
    id: "d-2",
    name: "Orphaned deal",
    owner_id: "u-removed",
    stage_id: "s-removed",
    company_id: "c-removed",
  },
] satisfies FilterPreview["rows"];

export const ReferencesByName: Story = {
  render: () => {
    installFetchStub({
      "GET /users/names": () =>
        jsonResponse({ data: [{ id: "u-1", display_name: "Priya Shah" }] }),
      "GET /stages": () =>
        jsonResponse({ data: [{ id: "s-1", name: "Qualified" }] }),
      "GET /companies": () =>
        jsonResponse({
          data: [{ id: "c-1", display_name: "Brandt GmbH" }],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <FilterResults
        preview={{
          resource: "deal",
          match_count: DEAL_ROWS.length,
          columns: ["id", "name", "owner_id", "stage_id", "company_id"],
          rows: DEAL_ROWS,
          truncated: false,
        }}
        fields={DEAL_FIELDS}
        named={["owner_id", "stage_id", "company_id"]}
        unit="deals"
        widthsKey="story-filter-preview-deals"
        pending={false}
      />
    );
  },
};
