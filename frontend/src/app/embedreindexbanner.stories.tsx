// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { EmbedReindexBanner } from "./embedreindexbanner";

function admin(overrides: Record<string, unknown> = {}) {
  return () =>
    jsonResponse({
      ...meFixture({ allow: { embedding_reindex: ["read"] } }),
      ...overrides,
    });
}

function story(reindexNeeded: boolean) {
  return () => {
    installFetchStub({
      "GET /me": admin(),
      "GET /embeddings/reindex/identities": () =>
        jsonResponse({
          configured_identity: "anthropic/voyage-3@1024",
          populated_identity: reindexNeeded
            ? "anthropic/voyage-2@1024"
            : "anthropic/voyage-3@1024",
        }),
    });
    return (
      <StoryProviders>
        <EmbedReindexBanner />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof EmbedReindexBanner> = {
  title: "Shell/Embed reindex banner",
  component: EmbedReindexBanner,
};
export default meta;
type Story = StoryObj<typeof EmbedReindexBanner>;

export const Needed: Story = { render: story(true) };
export const UpToDate: Story = { render: story(false) };

// Gated to ops/admin, same as EconomyBanner: a rep sees nothing, even when
// the identities differ.
export const HiddenForNonOpsRole: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({
          ...meFixture({ roles: ["rep"] }),
        }),
      "GET /embeddings/reindex/identities": () =>
        jsonResponse({
          configured_identity: "anthropic/voyage-3@1024",
          populated_identity: "anthropic/voyage-2@1024",
        }),
    });
    return (
      <StoryProviders>
        <EmbedReindexBanner />
      </StoryProviders>
    );
  },
};
