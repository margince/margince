// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { VocabularyField } from "./filterdata";
import type { Reference } from "./filterreference";
import { ValueControl } from "./filtervalue";
import type { FilterOp, LeafValue } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// One operand control per kind of value a clause can compare against. The
// Filters builder and the Analytics question builder both draw this, so each
// kind is shown here on its own rather than inside either host.
const meta: Meta<typeof ValueControl> = {
  title: "Patterns/Filter value",
  component: ValueControl,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ValueControl>;

function Editing({
  type,
  references,
  op,
  start,
}: Readonly<{
  type: VocabularyField["type"];
  references?: Reference;
  op: FilterOp;
  start: LeafValue;
}>) {
  const [value, setValue] = useState<LeafValue>(start);
  return (
    <ValueControl
      type={type}
      references={references}
      options={undefined}
      op={op}
      value={value}
      onChange={setValue}
    />
  );
}

function story(props: Parameters<typeof Editing>[0]) {
  return () => {
    installFetchStub({
      "GET /users": () =>
        jsonResponse({
          data: [
            { id: "u-1", display_name: "Ann Lee" },
            { id: "u-2", display_name: "Bruno Sá" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /companies": () =>
        jsonResponse({
          data: [{ id: "c-1", display_name: "Brandt Maschinenbau" }],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <Editing {...props} />
      </StoryProviders>
    );
  };
}

// An id names a seat, picked by name from the shared roster.
export const ReferencePicker: Story = {
  render: story({ type: "id", references: "app_user", op: "eq", start: "u-2" }),
};

// Companies are as many as the customers, so they are searched, not listed.
export const CompanySearch: Story = {
  render: story({ type: "id", references: "company", op: "eq", start: "" }),
};

export const YesOrNo: Story = {
  render: story({ type: "boolean", op: "eq", start: true }),
};

export const Quantity: Story = {
  render: story({ type: "number", op: "gte", start: 50 }),
};

export const Text: Story = {
  render: story({ type: "text", op: "eq", start: "Referral" }),
};

// The operator answers the question itself, so the two readings are the value.
export const OperatorOnly: Story = {
  render: story({
    type: "id",
    references: "app_user",
    op: "exists",
    start: true,
  }),
};

export const Phone: Story = {
  ...CompanySearch,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
