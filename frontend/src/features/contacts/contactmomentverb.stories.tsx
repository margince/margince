// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ActionVerb } from "./contactmomentverb";

type Contact = components["schemas"]["Contact"];
type ContactMomentAction = components["schemas"]["ContactMomentAction"];

// The moment card's verbs under each answer the activity gate gives.
const meta: Meta = {
  title: "Records/Contact 360/Moment verbs",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const ANNA: Contact = {
  id: "p-1",
  full_name: "Anna Weber",
  source: "manual",
  captured_by: "human:u1",
  attachable: true,
  created_at: "2026-05-01T08:00:00Z",
  updated_at: "2026-07-01T08:00:00Z",
};

const LOG: ContactMomentAction = {
  kind: "log_activity",
  label: "Log an interaction",
  state: "available",
  destination: { surface: "activity_log" },
};

const RESEARCH: ContactMomentAction = {
  kind: "open_research",
  label: "Open research",
  state: "available",
  destination: { surface: "research" },
};

const WRITES: Parameters<typeof meRoute>[0] = {
  contact: ["read", "update"],
  activity: ["read", "create"],
};

function verbs(contact: Contact, grants: Parameters<typeof meRoute>[0]) {
  installFetchStub({ "GET /me": meRoute(grants) });
  return (
    <StoryProviders>
      <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
        <ActionVerb
          action={LOG}
          contact={contact}
          primary
          onAction={() => undefined}
        />
        <ActionVerb
          action={RESEARCH}
          contact={contact}
          onAction={() => undefined}
        />
      </div>
    </StoryProviders>
  );
}

/** A writing role on its own contact: both verbs press. */
export const Writable: Story = { render: () => verbs(ANNA, WRITES) };

/** A writing role on a colleague's contact: Log stays, refused by the row. */
export const NotYours: Story = {
  render: () => verbs({ ...ANNA, attachable: false }, WRITES),
  play: async ({ canvasElement }) => {
    const log = await within(canvasElement).findByRole("button", {
      name: /Log an interaction/,
    });
    await waitFor(() =>
      expect(log).toHaveAccessibleDescription(
        "You can only read this contact. Ask its owner for write access.",
      ),
    );
  },
};

/** A role that only reads: the writing verb is not drawn at all. */
export const ReadOnlyRole: Story = {
  render: () => verbs(ANNA, { contact: ["read"], activity: ["read"] }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: /Open research/ });
    await waitFor(() =>
      expect(
        canvas.queryByRole("button", { name: /Log an interaction/ }),
      ).toBeNull(),
    );
  },
};

export const ReadOnlyRolePhone: Story = {
  ...ReadOnlyRole,
  tags: ["uat-phone"],
};
