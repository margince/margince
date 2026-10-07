// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { ProviderHealthCard } from "./providerhealth";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Which AI providers are not answering. The healthy state is a finding of its
// own, and each of the four failures says who can fix it.

const meta: Meta<typeof ProviderHealthCard> = {
  title: "Settings/Governance/System health/AI provider status",
  component: ProviderHealthCard,
};
export default meta;
type Story = StoryObj<typeof ProviderHealthCard>;

function story(
  providers: unknown[],
  allow: GrantSpec = { ai_diagnostics: ["read"] },
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ roles: ["admin"], allow })),
      "GET /ai/provider-health": () => jsonResponse({ providers }),
    });
    return (
      <StoryProviders>
        <ProviderHealthCard />
      </StoryProviders>
    );
  };
}

const ago = (minutes: number) =>
  new Date(Date.now() - minutes * 60_000).toISOString();
const ahead = (minutes: number) =>
  new Date(Date.now() + minutes * 60_000).toISOString();

export const AllAnswering: Story = { render: story([]) };

export const EveryFailure: Story = {
  render: story([
    { provider: "anthropic", health: "degraded", since: ago(20) },
    {
      provider: "gemini",
      health: "down",
      since: ago(95),
      retry_after: ahead(4),
    },
    {
      provider: "openai",
      health: "out_of_credit",
      since: ago(600),
      retry_after: ahead(12),
    },
    {
      provider: "jev",
      health: "unauthorized",
      since: ago(2000),
      retry_after: ahead(9),
    },
  ]),
};

// Dark. Each failure is told apart by a badge tone and a caption colour, which
// must both stay legible against the dark panel.
export const EveryFailureDark: Story = {
  ...EveryFailure,
  globals: { theme: "dark" },
};

export const Withheld: Story = {
  render: story([], { ai_diagnostics: [] }),
};
