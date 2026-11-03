// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { MailCoverageNotice } from "./mailcoveragenotice";

const meta = {
  title: "Records/Mail coverage",
  component: MailCoverageNotice,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof MailCoverageNotice>;
export default meta;
type Story = StoryObj<typeof meta>;
type Coverage = components["schemas"]["MailCaptureCoverage"];

function coverage(state: Coverage["state"]): Coverage {
  return {
    channel: "email",
    state,
    interval_from: "2026-02-01T12:00:00Z",
    interval_to: "2026-08-24T12:00:00Z",
    evaluated_at: "2026-08-24T12:00:00Z",
  };
}

export const Partial: Story = { args: { coverage: coverage("partial") } };
export const Unavailable: Story = {
  args: { coverage: coverage("unavailable") },
};
export const Withheld: Story = { args: { coverage: coverage("withheld") } };
export const OlderServer: Story = {};
