// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ExtensionDescriptor } from "@composition/extensions";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { meFixture } from "../testing/mefixture";
import { ExtensionEntries } from "./extensionentries";

// A unit's entries as the record header and the Log activity form draw them.
// The labels are core keys, because Storybook composes no unit copy.

const VISITS: ExtensionDescriptor = {
  name: "visits",
  title: "",
  verbs: [],
  secretScope: "",
  panels: [],
  entries: [
    {
      slot: "record_header.contact",
      label: "extUnits.user.title",
      icon: "map-pin",
      target: "report",
      object: "ext_visits_report",
    },
    {
      slot: "log_activity",
      label: "extUnits.workspace.title",
      icon: "clipboard-list",
      target: "",
      object: "ext_visits_report",
    },
  ],
};

function Granted({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient();
  client.setQueryData(
    ["me"],
    meFixture({ allow: { ext_visits_report: ["read"] } }),
  );
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider>{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

const meta: Meta<typeof ExtensionEntries> = {
  title: "Records/Extension entries",
  component: ExtensionEntries,
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
type Story = StoryObj<typeof ExtensionEntries>;

export const RecordHeader: Story = {
  args: {
    slot: "record_header.contact",
    record: { type: "contact", id: "c-1" },
    registry: [VISITS],
  },
};

export const LogActivityForm: Story = {
  args: {
    slot: "log_activity",
    inRow: true,
    record: { type: "contact", id: "c-1" },
    registry: [VISITS],
  },
};
