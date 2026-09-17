// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Fact, RecordFacts } from "./recordfacts";

const meta: Meta<typeof RecordFacts> = {
  title: "Design System/Record facts",
  component: RecordFacts,
};
export default meta;
type Story = StoryObj<typeof RecordFacts>;

export const Strip: Story = {
  render: () => (
    <RecordFacts>
      <Fact label="Email">mareike.vollmer@nordwind-logistik.de</Fact>
      <Fact label="Phone">+49 40 3311 8842</Fact>
      <Fact label="Owner">Tim Rasche</Fact>
      <Fact label="Created">1 June 2026</Fact>
    </RecordFacts>
  ),
};
