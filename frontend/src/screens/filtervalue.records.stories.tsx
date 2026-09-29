// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ChosenRecords, SearchedRecordValue } from "./filtervalue.records";
import type { LeafValue } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

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
    />
  );
}

function searchStory(many: boolean, start: LeafValue) {
  return () => {
    installFetchStub({
      "GET /companies": () =>
        jsonResponse({
          data: [{ id: "c-1", display_name: "Brandt Maschinenbau" }],
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
