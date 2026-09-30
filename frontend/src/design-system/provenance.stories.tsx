// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { LocaleProvider } from "../i18n";
import { ProvenanceTag } from "./provenance";
import { PassportChip } from "./trust";

const meta: Meta<typeof ProvenanceTag> = {
  title: "Components/AI and provenance/Provenance tag",
  component: ProvenanceTag,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ProvenanceTag>;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};
const row: CSSProperties = {
  display: "flex",
  gap: "1rem",
  alignItems: "center",
  flexWrap: "wrap",
};

const DEMO_USERS: Record<string, string> = { usr_7f2: "Carol Wagner" };

// Rows pair the arms a reader most needs told apart: an agent and a system
// job, the reader and a colleague, a buyer and an unrecorded source.
export const EveryArm: Story = {
  render: () => (
    <div style={stack}>
      <div style={row}>
        <ProvenanceTag provenance={{ kind: "agent", agent: "capture" }} />
        <PassportChip id="psp_7Q3fa91" />
        <ProvenanceTag provenance={{ kind: "agent" }} />
      </div>
      <div style={row}>
        <ProvenanceTag provenance={{ kind: "connector", connector: "gmail" }} />
      </div>
      <div style={row}>
        <ProvenanceTag
          provenance={{ kind: "system", job: "contact_auto_enrich" }}
        />
        <ProvenanceTag provenance={{ kind: "system" }} />
      </div>
      <div style={row}>
        <ProvenanceTag provenance={{ kind: "human", self: true }} />
        <ProvenanceTag
          provenance={{ kind: "human", self: false, userId: "usr_7f2" }}
          renderUser={(userId) => (
            <strong>{DEMO_USERS[userId] ?? userId}</strong>
          )}
        />
        {/* No renderUser: the tag says a contact entered it without claiming
            which one. */}
        <ProvenanceTag
          provenance={{ kind: "human", self: false, userId: "usr_7f2" }}
        />
      </div>
      <div style={row}>
        <ProvenanceTag provenance={{ kind: "buyer" }} />
        <ProvenanceTag provenance={{ kind: "unknown" }} />
      </div>
    </div>
  ),
};
