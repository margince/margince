// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../../i18n";
import { PromiseRecipients } from "./worklist.recipients";
import "./worklist.row.css";

const meta = {
  title: "Records/Worklist/Promise recipients",
  component: PromiseRecipients,
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof PromiseRecipients>;
export default meta;
type Story = StoryObj<typeof meta>;

export const PartialCompletion: Story = {
  args: {
    recipients: [
      {
        claim_id: "claim-ines",
        contact_id: "ines",
        display_name: "Ines Huber",
        status: "done",
      },
      {
        claim_id: "claim-morgan",
        contact_id: "morgan",
        display_name: "Morgan Recipient",
        status: "open",
      },
      {
        claim_id: "claim-pat",
        contact_id: "pat",
        display_name: "Pat Buyer",
        status: "dismissed",
      },
    ],
  },
};
export const Unavailable: Story = { args: { recipients: undefined } };

export const DarkPartialCompletion: Story = {
  args: PartialCompletion.args,
  globals: { theme: "dark" },
};
