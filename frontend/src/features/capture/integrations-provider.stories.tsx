// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ProviderCard } from "./integrations-provider";

// ProviderCard stories for the fe-uat render gate: the same three postures
// integrations-provider.test.tsx asserts (every write, connect-only, read-only)
// plus the two calm no-provider states, all off the GET /provider-connections
// shape.
//
// Every story routes GET /me, and the grants are the story's whole subject:
// this card's affordances are scoped by three separate ones, and a story that
// left the probe unrouted would silently capture the denied branch under a name
// claiming otherwise.

type ProviderConnection = components["schemas"]["ProviderConnection"];

const OPERATOR: GrantSpec = {
  integrations: ["create", "read", "update", "delete"],
};
// A seat that may bind a key but may not destroy what it bought — nothing seeds
// this, and an operator editing a role can produce it.
const CONNECT_ONLY: GrantSpec = { integrations: ["create", "read"] };
const READER: GrantSpec = { integrations: ["read"] };

const connected: ProviderConnection = {
  provider: "surfe",
  status: "connected",
  credential_present: true,
  configuration: {
    mode: "automatic_on_create",
    preset: "full",
    automatic_individual_create: true,
    automatic_import: false,
    categories: { professional: true },
  },
  credits: { pools: { email: 1840, mobile: 210 } },
  effective_constraints: ["EU residency", "no consumer mobile"],
  spend: {
    months: [
      {
        month: "2026-08-01",
        pool: "email",
        charged_credits: 312,
        held_credits: 8,
        runs: 74,
      },
      {
        month: "2026-07-01",
        pool: "email",
        charged_credits: 1204,
        held_credits: 0,
        runs: 291,
      },
      {
        month: "2026-07-01",
        pool: "mobile",
        charged_credits: 96,
        held_credits: 12,
        runs: 31,
      },
    ],
  },
  version: 4,
  created_at: "2026-01-05T09:00:00Z",
  updated_at: "2026-08-05T09:04:00Z",
};

// No key yet: the provider is registered, so the row exists and the key field
// with it, but there is no balance to read and nothing to destroy.
const unconnected: ProviderConnection = {
  provider: "surfe",
  status: "disconnected",
  credential_present: false,
  configuration: {
    mode: "on_demand",
    preset: "full",
    automatic_individual_create: false,
    automatic_import: false,
    categories: { professional: true },
  },
  credits: { pools: {} },
  version: 1,
  created_at: "2026-01-05T09:00:00Z",
  updated_at: "2026-01-05T09:00:00Z",
};

function cardStory(
  allow: GrantSpec,
  connections: ProviderConnection[],
  automaticLookup = true,
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /provider-connections": () => jsonResponse({ data: connections }),
      // Routed explicitly, and defaulting to the INSTALLATION's own default of
      // on. The stub's fallback answers an empty page, which reads as
      // `automatic_lookup: undefined` and draws every switch off — so a story
      // that skipped this route would show the posture off in its screenshot
      // while its name claimed something else.
      "GET /integrations/settings": () =>
        jsonResponse({ automatic_lookup: automaticLookup }),
    });
    return (
      <StoryProviders>
        <ProviderCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ProviderCard> = {
  title: "Settings/Data/Integrations/Contact data provider",
  component: ProviderCard,
};
export default meta;
type Story = StoryObj<typeof ProviderCard>;

export const OperatorConnected: Story = {
  render: cardStory(OPERATOR, [connected]),
};

// Before a key: the provider is one row with Connect beside it, and nothing it
// would read through a key is drawn yet.
export const OperatorNotYetConnected: Story = {
  render: cardStory(OPERATOR, [unconnected]),
};

export const OperatorNotYetConnectedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: cardStory(OPERATOR, [unconnected]),
};

/**
 * The key field itself, which now lives behind the row's verb.
 *
 * Opened by a `play` rather than left to a reader's click, because the dialog is
 * where the one input on this card lives: without it the render gate captures a
 * row and proves nothing about the field, the warning that a connect costs
 * money, or the submit's pending state.
 */
export const ConnectDialog: Story = {
  render: cardStory(OPERATOR, [unconnected]),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", { name: /connect/i }),
    );
    // Asserted, not merely awaited: a `play` that clicks and moves on records a
    // screenshot of whatever happened, and a dialog that failed to open looks
    // like a story nobody wrote a state for.
    await screen.findByRole("dialog");
  },
};

// An installation still working through the contacts it had before the provider
// was connected. The count and the sentence under it are the picture worth
// having: a figure alone stops moving for reasons this card would not explain,
// so the row says whether the sweep is running or paused.
export const OperatorCatchingUp: Story = {
  render: cardStory(OPERATOR, [
    { ...connected, lookup_backlog: { remaining: 1240, paused: false } },
  ]),
};

// The same backlog, going nowhere. Every cause — the posture off, the day's
// ceiling spent, a provider that stopped answering — reads identically as a
// number that will not fall, which is why the card says so in words.
export const OperatorCatchUpPaused: Story = {
  render: cardStory(
    OPERATOR,
    [{ ...connected, lookup_backlog: { remaining: 1240, paused: true } }],
    // Paused BECAUSE the posture is off, which is one of the three causes the
    // row's sentence names and the only one a screenshot can show.
    false,
  ),
};

// The posture off with nothing pending: what an installation in a jurisdiction
// that forbids trading personal data looks like after somebody switched it off.
export const OperatorLookupsOff: Story = {
  render: cardStory(OPERATOR, [connected], false),
};

// May bind a key, may not destroy what it bought — so the overflow that holds
// disconnect and delete-data is not offered at all.
export const ConnectOnly: Story = {
  render: cardStory(CONNECT_ONLY, [connected]),
};

// The reading stays — a rep's explanation for a dated value on a contact record
// — and the card says once why nothing here is writable.
export const ReadOnlySeat: Story = {
  render: cardStory(READER, [connected]),
};

// No adapter is compiled in. An empty list and a 501 both land here, because
// the card collapses them into one honest no-provider state rather than a
// failure — which is why there is ONE story and not two: the second was the
// same picture under a second name. The 501 path is drawn where it differs, in
// the contact record's own provider section.
export const NoProvider: Story = {
  render: cardStory(OPERATOR, []),
};

// The connected card in dark: the ProviderRow badge, the credit Meter's two
// greens a step apart, and the DataTable spend hairlines all re-resolve here.
export const OperatorConnectedDark: Story = {
  globals: { theme: "dark" },
  render: cardStory(OPERATOR, [connected]),
};

// The connected card at 390px. The ProviderRow head wraps its name and badge.
// The five-column spend DataTable scrolls inside its own box. The connect verb
// and its OverflowMenu wrap as a pair instead of squeezing.
//
// Storybook applies the viewport from the manager, by resizing the preview
// iframe. The fe-uat capture loads a bare iframe.html, so it renders this at
// the harness's own width. Its PNG is not a picture of a phone. Review it in
// Storybook, or narrow the browser.
export const OperatorConnectedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: cardStory(OPERATOR, [connected]),
};

/**
 * The paid half of the card, which no other story reaches: none of them carries
 * a catalog, so the switches that decide what a rep may buy render nowhere.
 *
 * The mobile row is the case worth looking at. The provider only issues a
 * number alongside the work email, so with that switch off the mobile cannot be
 * bought at all — the switch is refused and says what it needs, rather than
 * sitting on while the buy button it promises never appears on any contact.
 */
export const OperatorPricedCategories: Story = {
  render: cardStory(OPERATOR, [
    {
      ...connected,
      configuration: {
        ...connected.configuration,
        categories: { linkedin_profile: true, professional_email: false },
      },
      catalog: [
        { category: "linkedin_profile", free: true, cost: {} },
        { category: "professional_email", free: false, cost: { email: 1 } },
        {
          category: "mobile",
          free: false,
          cost: { email: 1, mobile: 1 },
          requires: "professional_email",
        },
      ],
    },
  ]),
};

// Two registered providers under ONE lookup switch: the switch answers for the
// installation, so it is drawn once above the connections rather than once per
// provider row.
export const OperatorTwoProviders: Story = {
  render: cardStory(OPERATOR, [
    connected,
    { ...unconnected, provider: "second_provider" },
  ]),
};

// A key still being checked: the badge stays neutral and carries the live dot,
// the way every running state is drawn.
export const OperatorValidating: Story = {
  render: cardStory(OPERATOR, [{ ...connected, status: "validating" }]),
};

/**
 * The typed confirm behind "Delete purchased data": the danger plate says what
 * goes and what stays, and the field asks for the provider's name.
 */
export const DeleteDataDialog: Story = {
  render: cardStory(OPERATOR, [connected]),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: "More actions",
      }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Delete purchased data" }),
    );
    await screen.findByRole("dialog");
  },
};
