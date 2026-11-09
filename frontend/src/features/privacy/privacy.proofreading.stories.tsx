// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { StoryProviders } from "../../testing/story-utils";
import { RequestProof } from "./privacy.proofreading";

const meta = {
  title: "Records/Privacy/Request proof",
  component: RequestProof,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof RequestProof>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Erased: Story = {
  args: {
    proof: {
      request_number: "DSR-0042",
      requester_email: "reader@demo.test",
      request_kind: "erasure",
      identity_check: "address_on_file",
      result: "completed",
      received_at: "2026-01-01T00:00:00Z",
      answered_at: "2026-01-04T00:00:00Z",
      keep_until: "2029-01-04T00:00:00Z",
      erased_counts: { contact: 1, activity: 3 },
      erased_uncounted: [],
      erased_counts_known: true,
      completed_by_type: "system",
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("DSR-0042"),
    ).toBeVisible();
  },
};
export const PartialCounts: Story = {
  ...Erased,
  args: {
    proof: {
      ...Erased.args.proof,
      erased_uncounted: ["attachments"],
      erased_counts_known: false,
    },
  },
};
export const ErasedDark: Story = { ...Erased, globals: { theme: "dark" } };
