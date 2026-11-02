// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../../i18n";
import { attentionReport } from "./systemhealth.fixtures";
import { HealthReadings } from "./systemhealthreadings";

const meta = {
  title: "Settings/Governance/System health/Check results",
  component: HealthReadings,
  args: {
    sources: attentionReport.sources,
    findings: attentionReport.findings,
  },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof HealthReadings>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Results: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
