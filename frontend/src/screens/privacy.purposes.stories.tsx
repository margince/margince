// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { GrantSpec } from "../app/mefixture";
import { ConsentPurposesCard } from "./privacy.purposes";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// The consent registry: a seat that may append to it, and one that may only
// read it, where the create verb is absent and the card says why.

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "marketing",
      label: "Marketing",
      requires_double_opt_in: true,
    },
    {
      id: "p2",
      key: "transactional",
      label: "Transactional",
      requires_double_opt_in: false,
    },
    {
      id: "p3",
      key: "product_updates",
      label: "Product updates",
      requires_double_opt_in: false,
    },
  ],
};

// Appending a purpose asks `consent_config:create` (consent/store.go's
// CreatePurpose) and the registry is readable by every seat, so the GRANT is
// what decides whether the header carries a verb. A role name does not carry
// it and does not fail loudly either: `meFixture` seats an admin by default, so
// a session naming only a role is a real principal holding no object grants —
// the card draws, the capture is green, and the verb is missing from the story
// whose whole subject is the verb.
const MAY_APPEND_PURPOSE: GrantSpec = { consent_config: ["create"] };

function purposes(
  allow: GrantSpec,
  purposeList: unknown = PURPOSES,
  routes: RouteMap = {},
) {
  return () => {
    stubWithSession(
      { "GET /consent-purposes": () => jsonResponse(purposeList), ...routes },
      allow,
    );
    return (
      <StoryProviders>
        <ConsentPurposesCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConsentPurposesCard> = {
  title: "Settings/Governance/Privacy and retention/Consent purposes",
  component: ConsentPurposesCard,
};
export default meta;

type Story = StoryObj<typeof ConsentPurposesCard>;

// A seat that may append: the registry, and `Add purpose` in the card header
// above it.
export const Registry: Story = { render: purposes(MAY_APPEND_PURPOSE) };

// A seat without the grant: the same registry, no verb, and the read-only
// posture under the card's description.
export const ReadOnly: Story = { render: purposes({}) };

// Dark, because a tinted double opt-in badge against `--bgElevated` is the pair
// most likely to collapse when the ground goes dark.
export const RegistryDark: Story = {
  globals: { theme: "dark" },
  render: purposes(MAY_APPEND_PURPOSE),
};

// Nothing registered yet.
export const Empty: Story = {
  render: purposes(MAY_APPEND_PURPOSE, { data: [] }),
};

// The append-only warning is the dialog's first line, said exactly once and
// beside the key it is about.
export const AddPurpose: Story = {
  render: purposes(MAY_APPEND_PURPOSE),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /add purpose/i }),
    );
  },
};

export const RegistryPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: purposes(MAY_APPEND_PURPOSE),
};

// The longest a purpose runs: a label that wraps and a key with no break in it.
export const LongPurpose: Story = {
  render: purposes(MAY_APPEND_PURPOSE, {
    data: [
      {
        id: "p9",
        key: "partner_programme_quarterly_newsletter_and_event_invitations",
        label: "Quarterly partner programme newsletter and event invitations",
        requires_double_opt_in: true,
      },
      ...PURPOSES.data,
    ],
  }),
};

export const LongPurposePhone: Story = {
  ...LongPurpose,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const Loading: Story = {
  render: purposes(MAY_APPEND_PURPOSE, PURPOSES, {
    "GET /consent-purposes": () => new Promise(() => {}),
  }),
};

export const ReadFailed: Story = {
  render: purposes(MAY_APPEND_PURPOSE, PURPOSES, {
    "GET /consent-purposes": () =>
      jsonResponse(
        { title: "Internal Server Error", status: 500, code: "internal" },
        500,
      ),
  }),
};
