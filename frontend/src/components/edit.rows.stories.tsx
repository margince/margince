// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import type { CreateField } from "./create.fields";
import { FlowFieldRow, ReadOnlyFieldRow } from "./edit.rows";

const meta: Meta = {
  title: "Patterns/Create record/Read-only and flow rows",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

const arrField: CreateField = {
  key: "expected_arr",
  labelText: "Expected ARR",
};
const ownerField: CreateField = { key: "owner", labelText: "Owner" };

type RowArgs = Readonly<{ onFollow: (open: () => void) => void }>;

// A locked field reads with its reason; a field another flow changes offers
// that flow's verb.
export const LockedAndFlowRows: StoryObj<RowArgs> = {
  args: { onFollow: fn() },
  render: ({ onFollow }) => (
    <div className="form-stack" style={{ maxWidth: 340 }}>
      <ReadOnlyFieldRow
        field={arrField}
        readOnly={{ reading: "€48,000.00", reason: "Set by the signed offer" }}
      />
      <FlowFieldRow
        field={ownerField}
        flow={{ open: () => {}, reading: "Ana Brandt" }}
        disabled={false}
        onFollow={onFollow}
      />
    </div>
  ),
  play: async ({ args, canvasElement }) => {
    const canvas = within(canvasElement);
    const arr = await canvas.findByRole("group", {
      name: englishCopy["deal.expectedArr"],
    });
    await expect(arr).toHaveTextContent("€48,000.00");
    await expect(arr).toHaveAccessibleDescription("Set by the signed offer");
    await userEvent.click(canvas.getByRole("button", { name: "Change Owner" }));
    await expect(args.onFollow).toHaveBeenCalledOnce();
  },
};

export const LockedAndFlowRowsPhone: StoryObj<RowArgs> = {
  ...LockedAndFlowRows,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
