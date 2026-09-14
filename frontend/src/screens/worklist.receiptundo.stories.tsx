// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders, stubWithSession } from "./story-utils";
import { automaticDateReceipt } from "./worklist.receiptreview.fixtures";
import { ReceiptUndo } from "./worklist.receiptundo";

const meta: Meta<typeof ReceiptUndo> = {
  title: "Records/Worklist/Undo correction",
  component: ReceiptUndo,
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
type Story = StoryObj<typeof ReceiptUndo>;
export const AppliedDate: Story = { args: { receipt: automaticDateReceipt } };
