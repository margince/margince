// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders, stubWithSession } from "./story-utils";
import { ReceiptReview } from "./worklist.receiptreview";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "./worklist.receiptreview.fixtures";

const meta: Meta<typeof ReceiptReview> = {
  title: "Records/Worklist/Review applied changes",
  component: ReceiptReview,
  decorators: [
    (Story) => {
      stubWithSession({}, { deal: ["read", "update"] });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof ReceiptReview>;
export const AutomaticStage: Story = {
  args: { receipt: automaticStageReceipt },
};
export const CorrectedDate: Story = { args: { receipt: automaticDateReceipt } };
export const Accepted: Story = {
  args: {
    receipt: {
      ...automaticStageReceipt,
      review: {
        kind: "stage",
        accepted: true,
        reversed: false,
        version: 7,
        can_undo: true,
        can_accept: true,
        writable: true,
      },
    },
  },
};
export const Reversed: Story = {
  args: {
    receipt: {
      ...automaticStageReceipt,
      review: {
        kind: "stage",
        accepted: false,
        reversed: true,
        version: 8,
        can_undo: false,
        can_accept: false,
        writable: true,
      },
    },
  },
};
