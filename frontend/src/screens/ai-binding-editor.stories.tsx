// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { BindingEditor } from "./ai-binding-editor";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const ROUTING = {
  profile: "eu_hosted" as const,
  tiers: {
    cheap_cloud: { provider: "gemini", model: "gemini-3.1-flash-lite" },
  },
  embeddings: { provider: "gemini", model: "gemini-embedding-001" },
};
const KEYS = [
  {
    provider: "gemini",
    configured: true,
    env_var: "GEMINI_API_KEY",
    optional: false,
  },
  {
    provider: "anthropic",
    configured: false,
    env_var: "ANTHROPIC_API_KEY",
    optional: false,
  },
];

function story(routing = ROUTING) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { ai_routing: ["read", "update"] } })),
      "GET /ai/available-models/gemini": () =>
        jsonResponse({ provider: "gemini", models: [] }),
      "GET /ai/available-models/ollama": () =>
        jsonResponse({ provider: "ollama", models: [{ id: "gemma3:latest" }] }),
      "GET /ai/available-models/vllm": () =>
        jsonResponse({
          provider: "vllm",
          models: [],
          unavailable: "unreachable",
        }),
    });
    return (
      <StoryProviders>
        <BindingEditor
          opened={{ routing, version: '"routing-v1"' }}
          initial={{
            kind: "tier",
            tier: "cheap_cloud",
            binding: routing.tiers.cheap_cloud,
          }}
          label="Cheap cloud"
          keys={KEYS}
          catalogue={[]}
          canManage
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof BindingEditor> = {
  title: "Settings/AI/Models and routing/Binding editor",
  component: BindingEditor,
};
export default meta;
type Story = StoryObj<typeof BindingEditor>;

export const Editing: Story = { render: story() };
export const EditingDark: Story = {
  globals: { theme: "dark" },
  render: story(),
};

// The provider list: the keyed vendor with a key and the keyless one that
// answers are offered; the vendor without a key, the adapter nothing listens
// for and fake are not.
export const ProvidersOffered: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    const dialog = within(within(canvasElement).getByRole("dialog"));
    await userEvent.click(
      await dialog.findByRole("combobox", { name: "Provider" }),
    );
    const listbox = within(await within(document.body).findByRole("listbox"));
    await expect(await listbox.findByText("ollama")).toBeVisible();
    await expect(listbox.queryByText("vllm")).toBeNull();
    await expect(listbox.queryByText("anthropic")).toBeNull();
    await expect(listbox.queryByText("fake")).toBeNull();
  },
};
