// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { RecordGone } from "./recordgone";

// What a record page draws when its read answers 404 or 422: the record is not
// there, or not shared with the reader. An offer leads back to Deals, since
// offers have no list of their own.
const meta: Meta<typeof RecordGone> = {
  title: "Records/Record 360/Record not there",
  component: RecordGone,
  parameters: { layout: "padded" },
  render: (args) => (
    <StoryProviders>
      <div className="wrap">
        <RecordGone {...args} />
      </div>
    </StoryProviders>
  ),
};
export default meta;

type Story = StoryObj<typeof RecordGone>;

export const Deal: Story = { args: { kind: "deal" } };

export const Contact: Story = { args: { kind: "contact" } };

export const Offer: Story = { args: { kind: "offer" } };
