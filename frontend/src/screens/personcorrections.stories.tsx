// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { EnrichedFields } from "./personcorrections";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The card where the page stops asserting and starts asking: what a machine
// read, the snippet it read it from, and the two verdicts a reader may record.
// Both halves of the grant boundary are here, because who may SAY something is
// a different question from who may see the evidence — and so is the editor,
// which is the only state the correction field is drawn in.

type Person360 = components["schemas"]["Person360"];
type ProfileField = components["schemas"]["PersonProfileField"];

const person: components["schemas"]["Person"] = {
  id: "p-1",
  full_name: "Dana Buyer",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

const titleRead: ProfileField = {
  field: "title",
  value: "Head of Procurement",
  evidence_snippet: "Head of Procurement, Brandt Automotive GmbH",
  source: "capture_enrich",
  captured_by: "agent:enrich",
  captured_at: "2026-08-01T08:00:00Z",
};

const view: Person360 = {
  as_of: "2026-08-18T09:00:00Z",
  person,
  sections_omitted: [],
  profile_fields: [titleRead],
};

const meta: Meta<typeof EnrichedFields> = {
  title: "Records/Person corrections",
  component: EnrichedFields,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof EnrichedFields>;

function fields(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({ "GET /me": meRoute(allow) });
    return (
      <StoryProviders>
        <EnrichedFields personId="p-1" view={view} />
      </StoryProviders>
    );
  };
}

const MAY_CORRECT = { person: ["update"] as const };

// The resting card for a reader the server will admit: the reading, its
// evidence, and the two verdicts.
export const Correctable: Story = { render: fields(MAY_CORRECT) };

// The editor, which is the only state the correction field is drawn in — it
// sits beside the field's name on one line rather than filling a form column.
export const Editing: Story = {
  render: fields(MAY_CORRECT),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(await canvas.findByRole("button", { name: "Correct" }));
  },
};

// Dark, on the editor, because the field's ground and the card's are two
// elevations a darker palette compresses toward each other.
export const EditingDark: Story = {
  globals: { theme: "dark" },
  render: fields(MAY_CORRECT),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(await canvas.findByRole("button", { name: "Correct" }));
  },
};

// The evidence is a read and renders for anyone who can open the contact; the
// verdicts are a write, so a seat without `person:update` gets neither button.
export const ReadOnly: Story = {
  render: fields({}),
};
