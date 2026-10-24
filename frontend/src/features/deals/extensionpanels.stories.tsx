// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ExtensionDescriptor } from "@composition/extensions";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Card } from "../../design-system/atoms";
import { Stack } from "../../design-system/stack";
import { LocaleProvider } from "../../i18n";
import type { ExtensionPanelProps } from "../../lib/extensions";
import { meFixture } from "../../testing/mefixture";
import { ExtensionPanels } from "./extensionpanels";

// Two units' panels as a deal page's side column stacks them, and a third that
// throws, which its own boundary keeps to its card.

function unit(name: string): ExtensionDescriptor {
  return {
    name,
    title: "",
    verbs: [],
    entries: [],
    secretScope: "",
    panels: [
      {
        slot: "record_panel.deal",
        export: "DealPanel",
        object: `ext_${name}_x`,
      },
    ],
  };
}

function ChannelPanel({ record }: ExtensionPanelProps) {
  return <Card title="Dispact channel">Linked to deal {record.id}</Card>;
}

function VisitsPanel({ record }: ExtensionPanelProps) {
  return <Card title="Field visits">Two visits logged on {record.id}</Card>;
}

function BrokenPanel(): ReactNode {
  throw new Error("a unit's render bug");
}

const COMPONENTS = {
  channel: { "record_panel.deal": ChannelPanel },
  visits: { "record_panel.deal": VisitsPanel },
  broken: { "record_panel.deal": BrokenPanel },
};

function Granted({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient();
  client.setQueryData(
    ["me"],
    meFixture({
      allow: {
        ext_channel_x: ["read"],
        ext_visits_x: ["read"],
        ext_broken_x: ["read"],
      },
    }),
  );
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <Stack gap="4">{children}</Stack>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

const meta: Meta<typeof ExtensionPanels> = {
  title: "Records/Extension panels",
  component: ExtensionPanels,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <Granted>
        <Story />
      </Granted>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ExtensionPanels>;

export const TwoUnits: Story = {
  args: {
    slot: "record_panel.deal",
    record: { type: "deal", id: "d-1" },
    registry: [unit("channel"), unit("visits")],
    components: COMPONENTS,
  },
};

export const OneUnitThrows: Story = {
  args: {
    slot: "record_panel.deal",
    record: { type: "deal", id: "d-1" },
    registry: [unit("channel"), unit("broken"), unit("visits")],
    components: COMPONENTS,
  },
};

export const NoUnit: Story = {
  args: {
    slot: "record_panel.deal",
    record: { type: "deal", id: "d-1" },
    registry: [],
    components: {},
  },
};
