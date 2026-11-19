// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { FilterOp, LeafValue } from "../../lib/segmentpredicate";
import { StoryProviders } from "../../testing/story-utils";
import { MoneyControl } from "./filtervalue.money";

const meta: Meta<typeof MoneyControl> = {
  title: "Patterns/Filter value/Money",
  component: MoneyControl,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof MoneyControl>;

function Editing({
  op,
  start,
  currency,
}: Readonly<{ op: FilterOp; start: LeafValue; currency: string }>) {
  const [value, setValue] = useState<LeafValue>(start);
  return (
    <StoryProviders>
      <MoneyControl
        op={op}
        value={value}
        onChange={setValue}
        label="Deal value"
        currency={currency}
      />
    </StoryProviders>
  );
}

export const SingleAmount: Story = {
  render: () => <Editing op="gt" start={1250000} currency="EUR" />,
};

export const ListOfAmounts: Story = {
  render: () => <Editing op="in" start={[1000000, 2500000]} currency="EUR" />,
};
