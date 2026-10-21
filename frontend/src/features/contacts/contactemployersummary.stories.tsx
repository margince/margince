// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CompanyMark } from "../../components/contactemployersummary";
import { StoryProviders } from "../../testing/story-utils";
import "./contact360.css";

// A company's mark in its two readings: the company's own logo, and the
// monogram keyed on the company's id when no logo resolved.
const meta: Meta = {
  title: "Records/Contact 360/Company mark",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const LOGO =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect x='8' y='8' width='48' height='48' rx='10' fill='%230b7a53'/%3E%3C/svg%3E";

function CompanyMarkReadings() {
  return (
    <div style={{ display: "flex", gap: "var(--space-4)", height: 32 }}>
      <div style={{ width: 32, height: 32 }}>
        <CompanyMark
          companyId="o-1"
          name="Brandt Automotive GmbH"
          logoUrl={LOGO}
        />
      </div>
      <div style={{ width: 32, height: 32 }}>
        <CompanyMark
          companyId="o-1"
          name="Brandt Automotive GmbH"
          logoUrl={null}
        />
      </div>
    </div>
  );
}

export const Readings: Story = {
  render: () => <CompanyMarkReadings />,
};
