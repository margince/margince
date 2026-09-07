// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { GrantSpec } from "../app/mefixture";
import { ConsentPurposesCard } from "./privacy";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

// The consent registry (the Privacy & retention tab's ConsentPurposesCard). Its own
// file rather than a second component in privacy.stories.tsx, so each surface
// keeps one story title: `fe-uat` keys on the co-located name, and a card with
// no story of its own is a card nobody looks at in either theme.
//
// The two states worth reading are the two the card actually has: a seat that
// may append to the registry, and one that may only read it — where the create
// verb is ABSENT from the header and the registry row's own description says so.

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

function purposes(allow: GrantSpec, purposeList: unknown = PURPOSES) {
  return () => {
    stubWithSession(
      { "GET /consent-purposes": () => jsonResponse(purposeList) },
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
  title: "Settings/Governance/Privacy & retention/Consent purposes",
  component: ConsentPurposesCard,
};
export default meta;

type Story = StoryObj<typeof ConsentPurposesCard>;

// A seat that may append: the registry, and `Add purpose` in the card header
// above it.
export const Registry: Story = { render: purposes(MAY_APPEND_PURPOSE) };

// A seat without the grant: the same registry, no verb, and the read-only
// posture as the registry row's own description — the sentence sits at the
// label's x rather than as a loose paragraph between the card's line and the
// badges.
export const ReadOnly: Story = { render: purposes({}) };

// Dark, because the registry is a run of badges and one of them carries `warn`
// for a double-opt-in purpose: a tinted badge against `--bgElevated` is the pair
// most likely to collapse when the ground goes dark.
export const RegistryDark: Story = {
  globals: { theme: "dark" },
  render: purposes(MAY_APPEND_PURPOSE),
};

// Nothing registered yet — the honest empty answer to the row's question, which
// takes a row's interval rather than a page-sized plate.
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
