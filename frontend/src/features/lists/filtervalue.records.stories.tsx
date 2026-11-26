// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { translate } from "../../i18n";
import type { LeafValue } from "../../lib/segmentpredicate";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ChosenRecords, SearchedRecordValue } from "./filtervalue.records";

// The record half of a filter value: a company found by search, and the
// records a list clause already holds.
const meta: Meta = { title: "Patterns/Filter value/Records" };
export default meta;

type Story = StoryObj;

function Searching({
  many,
  start,
}: Readonly<{ many: boolean; start: LeafValue }>) {
  const [value, setValue] = useState<LeafValue>(start);
  return (
    <SearchedRecordValue
      many={many}
      value={value}
      onChange={setValue}
      label="Value, filter 1"
      placeholder={translate("en", "filters.pickCompany")}
    />
  );
}

function searchStory(many: boolean, start: LeafValue) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        contact: ["read"],
        company: ["read"],
        deal: ["read"],
      }),
      "GET /companies": () =>
        jsonResponse({
          data: [
            { id: "c-1", display_name: "Brandt Maschinenbau" },
            { id: "c-2", display_name: "Brandt Logistik" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <Searching many={many} start={start} />
      </StoryProviders>
    );
  };
}

// Nothing typed yet: the box says what it is for.
export const CompanySearch: Story = { render: searchStory(false, "") };

// The answers in the suggestion list under the box; the arrows walk it.
export const CompanySuggestions: Story = {
  render: searchStory(false, ""),
  play: async ({ canvasElement }) => {
    const box = within(canvasElement).getByRole("combobox");
    await userEvent.type(box, "Brandt");
    const list = await within(document.body).findByRole("listbox");
    await expect(within(list).getAllByRole("option")).toHaveLength(2);
  },
};

// A list clause keeps its box open under what it already holds.
export const ManyCompanies: Story = {
  render: searchStory(true, ["c-7", "c-9"]),
};

export const Chosen: Story = {
  render: () => (
    <StoryProviders>
      <ChosenRecords
        ids={["c-1", "c-2"]}
        labels={new Map([["c-1", "Brandt Maschinenbau"]])}
        onRemove={() => {}}
      />
    </StoryProviders>
  ),
};

export const CompanySearchPhone: Story = {
  ...CompanySearch,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
