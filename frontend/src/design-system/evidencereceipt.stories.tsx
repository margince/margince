// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { EvidenceReceipt } from "./evidencereceipt";

// What a number was drawn from, beside the number.
const meta: Meta = {
  title: "Design System/EvidenceReceipt",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 360 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const counts = [
  { key: "eligible", term: "Eligible deals", value: "52" },
  { key: "priced", term: "Priced", value: "40" },
  { key: "confirmed", term: "Close date confirmed", value: "31" },
  { key: "fx", term: "FX rate missing", value: "2" },
];

export const Ready: Story = {
  render: () => (
    <EvidenceReceipt
      title="Data and evidence checked"
      state={{ label: "Ready", tone: "success" }}
      counts={counts}
      calculationSummary="How €1.2M was calculated"
      calculation={
        <p>
          Each deal's amount times its stage win probability, rounded per deal,
          then summed. Unpriced deals count as eligible and contribute nothing.
        </p>
      }
    />
  ),
};

// The gap is the point: 40 priced of 52 eligible says what the total does not
// cover, in the same glance as the total.
export const WithExceptions: Story = {
  render: () => (
    <EvidenceReceipt
      title="Data and evidence checked"
      state={{ label: "Needs review", tone: "warn" }}
      counts={counts}
    />
  ),
};

// Still loading its verdict. No badge, because a defaulted "ok" would state a
// conclusion about inputs nobody has checked yet.
export const NoVerdictYet: Story = {
  render: () => (
    <EvidenceReceipt title="Data and evidence checked" counts={counts} />
  ),
};

export const NothingToReport: Story = {
  render: () => (
    <EvidenceReceipt title="Data and evidence checked" counts={[]} />
  ),
};
