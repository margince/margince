// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Button, Card, SectionHeader } from "./atoms";

const meta: Meta<typeof SectionHeader> = {
  title: "Components/Layout and structure/Section header",
  component: SectionHeader,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof SectionHeader>;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

export const Levels: Story = {
  render: () => (
    <div style={stack}>
      <SectionHeader title="Pipeline" />
      <SectionHeader
        title="Reporting currency"
        actions={<Button>Change</Button>}
      />
      <Card>
        <SectionHeader title="Contacts" />
        <p className="t-caption">Carol Wagner · Bob Schmidt · Alice Müller</p>
      </Card>
      <Card>
        <SectionHeader title="Delivery" />
        <SectionHeader title="Endpoints" level={3} />
        <p className="t-caption">Two subscriptions, both healthy.</p>
        <SectionHeader title="Dead-lettered" level={3} />
        <p className="t-caption">Nothing waiting.</p>
      </Card>
    </div>
  ),
};
