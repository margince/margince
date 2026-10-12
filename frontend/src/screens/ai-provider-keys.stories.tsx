// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { AiProviderKeysCard } from "./ai-provider-keys";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The card draws a credential surface, so what these stories are for is
// checking what is NOT on screen: a configured provider must read as
// configured without the key, or any part of it, being recoverable from the
// pixels. The server never sends one back, and the field a reader could paste
// into is always empty — a story is where that stays visible to a human.
//
// /me decides which of the card's two shapes renders: the grant is
// `ai_routing:update`, the same one the binding carries, because a seat that
// may not re-point a model may not reach the credential that model would call
// with. A reader who can look but not change gets the form DISABLED rather
// than hidden, so they can see which providers are keyed.
const MANAGER: GrantSpec = { ai_routing: ["read", "update"] };
const READER: GrantSpec = { ai_routing: ["read"] };
// Reaches the AI tab on another grant and holds no ai_routing at all.
const NO_GRANT: GrantSpec = { automation: ["read"] };

const ROUTING = {
  profile: "cloud_frontier",
  tiers: {
    cheap_cloud: { provider: "gemini", model: "gemini-3.5-flash" },
    premium: { provider: "gemini", model: "gemini-3.1-pro-preview" },
    frontier: { provider: "anthropic", model: "claude-opus-4-1" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};

function story(
  providers: {
    provider: string;
    configured: boolean;
    env_var: string;
    usable: boolean;
    optional: boolean;
    credential_kind: "api_key" | "service_account";
  }[],
  allow: GrantSpec = MANAGER,
  routing: object | null = null,
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /ai/provider-keys": () => jsonResponse({ providers }),
      ...(routing ? { "GET /ai/routing": () => jsonResponse(routing) } : {}),
      "POST /ai/provider-keys/gemini/test": () =>
        jsonResponse({ provider: "gemini", ok: true, model_count: 42 }),
    });
    return (
      <StoryProviders>
        <AiProviderKeysCard />
      </StoryProviders>
    );
  };
}

const gemini = {
  provider: "gemini",
  configured: true,
  env_var: "GEMINI_API_KEY",
  usable: true,
  optional: false,
  credential_kind: "api_key" as const,
};
const anthropic = {
  provider: "anthropic",
  configured: false,
  env_var: "ANTHROPIC_API_KEY",
  usable: false,
  optional: false,
  credential_kind: "api_key" as const,
};
// Keyed by a file rather than a paste: its row reads "Service account key
// configured" and its editor is the key-file box.
const vertex = {
  provider: "gemini_vertex",
  configured: true,
  env_var: "GEMINI_VERTEX_SA_JSON",
  usable: true,
  optional: false,
  credential_kind: "service_account" as const,
};
// A self-hosted decision server needs no key, so this one is sent when held.
const jevCompatible = {
  provider: "jev_compatible",
  configured: false,
  env_var: "JEV_COMPATIBLE_API_KEY",
  usable: true,
  optional: true,
  credential_kind: "api_key" as const,
};

const meta: Meta<typeof AiProviderKeysCard> = {
  title: "Settings/AI/AI models/Model provider keys",
  component: AiProviderKeysCard,
};
export default meta;
type Story = StoryObj<typeof AiProviderKeysCard>;

// One provider keyed, one not — the ordinary reading, and the one that has to
// distinguish the two states without printing either key.
export const Mixed: Story = { render: story([gemini, anthropic]) };

export const InUse: Story = {
  render: story([gemini, anthropic, jevCompatible, vertex], MANAGER, ROUTING),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table");
    await canvas.findByText("Everyday cloud");
  },
};

export const InUsePhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story([gemini, anthropic, jevCompatible, vertex], MANAGER, ROUTING),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("table");
  },
};

export const InUseDark: Story = {
  globals: { theme: "dark" },
  render: story([gemini, anthropic, jevCompatible, vertex], MANAGER, ROUTING),
};

// Every bound provider still unkeyed: the state a fresh installation is in,
// where the AI lanes are absent until somebody pastes a key. It must read as
// "nothing set yet" and not as an error.
export const NothingConfigured: Story = {
  render: story([anthropic, { ...gemini, configured: false, usable: false }]),
};

// An optional key not held reads as optional, not as a gap: the adapter calls
// without one, so nothing here is wrong and nothing warns.
export const OptionalKey: Story = {
  render: story([gemini, anthropic, jevCompatible]),
};

// A keyed provider on its own. The row says configured and offers removal; the
// input beside it is still empty, because there is nothing to put back in it.
export const Configured: Story = { render: story([gemini]) };

// No cloud provider is bound, so there is no key to ask for. An empty card is
// the honest answer here rather than a fault — a local-only or unbound
// installation has nothing to key.
export const NoCloudProviders: Story = { render: story([]) };

// A seat with the read half of the grant and not the write half. The rows keep
// their place and say which providers are keyed; the field and the buttons are
// disabled rather than absent, so the reader can see the state without being
// invited to change it.
export const ReadOnlySeat: Story = {
  render: story([gemini, anthropic], READER),
};

// No read grant at all. The card keeps its place and says the list is withheld
// — it must not look like an installation with no credentials, and it must not
// draw an error box, which is what asking the server anyway produced.
export const Withheld: Story = {
  render: story([gemini, anthropic], NO_GRANT),
};

// A service-account vendor beside two API-key ones, opened at Replace: the
// key-file box and picker, empty like every other.
export const ServiceAccount: Story = {
  render: story([gemini, vertex, anthropic]),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      within(
        await body.findByTestId("ai-provider-row-gemini_vertex"),
      ).getByRole("button", {
        name: /^Edit/,
      }),
    );
    await userEvent.click(
      await body.findByRole("button", { name: /^replace$/i }),
    );
  },
};

export const ServiceAccountDark: Story = {
  globals: { theme: "dark" },
  render: story([gemini, vertex, anthropic]),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      within(
        await body.findByTestId("ai-provider-row-gemini_vertex"),
      ).getByRole("button", {
        name: /^Edit/,
      }),
    );
    await userEvent.click(
      await body.findByRole("button", { name: /^replace$/i }),
    );
  },
};

// Dark. The configured/not-configured distinction is carried by a Badge tone,
// and a tone that flattens against the dark panel would leave a reader unable
// to tell a keyed provider from an unkeyed one — which on this card is the
// only thing it says.
export const MixedDark: Story = {
  globals: { theme: "dark" },
  render: story([gemini, anthropic]),
};

// A key tested against its vendor: the answer lands on the row it was asked
// for, as a count on a pass and as the named reason on a failure.
export const Tested: Story = {
  render: story([gemini, anthropic]),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      within(await body.findByTestId("ai-provider-row-gemini")).getByRole(
        "button",
        {
          name: /^Edit/,
        },
      ),
    );
    await userEvent.click(await body.findByRole("button", { name: /^test$/i }));
  },
};
