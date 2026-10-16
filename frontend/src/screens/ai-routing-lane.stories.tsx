// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { type Lane, TiersTable } from "./ai-routing-lane";

const LANES: readonly Lane[] = [
  {
    name: "cheap_cloud",
    lane: "chat",
    binding: { provider: "gemini", model: "gemini-2.5-flash" },
  },
  {
    name: "premium",
    lane: "chat",
    binding: { provider: "anthropic", model: "claude-opus-4-8" },
  },
  {
    name: "embeddings",
    lane: "embeddings",
    binding: { provider: "gemini", model: "gemini-embedding-001" },
  },
];

function Tiers() {
  // The tier lines read the session to learn whether this reader may see the
  // week's figures, so the table no longer renders without one.
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture()),
    "GET /ai/call-stats": () =>
      jsonResponse({ window: "7d", group: "tier", rows: [] }),
  });
  return (
    <StoryProviders>
      <Panel title="Model tiers">
        <TiersTable
          lanes={LANES}
          health={undefined}
          features={undefined}
          catalogue={undefined}
          canManage={false}
        />
      </Panel>
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/AI/AI models/Model tiers table",
  component: Tiers,
} satisfies Meta<typeof Tiers>;
export default meta;
type Story = StoryObj<typeof meta>;

// A tier is named in words alone; its routing key stays off the row.
const tiersInWords = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  const canvas = within(canvasElement);
  for (const name of ["Everyday cloud", "Premium", "Embedding model"]) {
    await expect(await canvas.findByText(name)).toBeInTheDocument();
  }
  for (const key of ["cheap_cloud", "premium", "embeddings"]) {
    await expect(canvas.queryByText(key)).toBeNull();
  }
};

export const TiersNamedInWords: Story = { play: tiersInWords };

export const TiersNamedInWordsAtPhoneWidth: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: tiersInWords,
};
