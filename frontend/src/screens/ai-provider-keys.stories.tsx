// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
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

function story(
  providers: { provider: string; configured: boolean; env_var: string }[],
  allow: GrantSpec = MANAGER,
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /ai/provider-keys": () => jsonResponse({ providers }),
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
};
const anthropic = {
  provider: "anthropic",
  configured: false,
  env_var: "ANTHROPIC_API_KEY",
};

const meta: Meta<typeof AiProviderKeysCard> = {
  title: "Settings/Admin settings/AI/Model provider keys",
  component: AiProviderKeysCard,
};
export default meta;
type Story = StoryObj<typeof AiProviderKeysCard>;

// One provider keyed, one not — the ordinary reading, and the one that has to
// distinguish the two states without printing either key.
export const Mixed: Story = { render: story([gemini, anthropic]) };

// Every bound provider still unkeyed: the state a fresh installation is in,
// where the AI lanes are absent until somebody pastes a key. It must read as
// "nothing set yet" and not as an error.
export const NothingConfigured: Story = {
  render: story([anthropic, { ...gemini, configured: false }]),
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

// Dark. The configured/not-configured distinction is carried by a Badge tone,
// and a tone that flattens against the dark panel would leave a reader unable
// to tell a keyed provider from an unkeyed one — which on this card is the
// only thing it says.
export const MixedDark: Story = {
  globals: { theme: "dark" },
  render: story([gemini, anthropic]),
};
