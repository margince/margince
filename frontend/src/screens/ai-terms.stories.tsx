// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../design-system/panel";
import {
  ModelRef,
  PanelTitle,
  RowOpen,
  TermChip,
  TermLegend,
  TierChips,
} from "./ai-terms";
import { StoryProviders } from "./story-utils";

// The three words of the AI settings area, each with one icon wherever it
// appears: provider, tier, task. Told apart by icon and not by colour, because
// colour on these pages already carries state.

function Terms() {
  return (
    <StoryProviders>
      <Panel title={<PanelTitle term="tier">Model tiers</PanelTitle>}>
        <PanelBody>
          <TermLegend />
          <p>
            <TermChip term="provider">OpenAI-compatible</TermChip>{" "}
            <TermChip term="tier">Everyday cloud</TermChip>{" "}
            <TermChip term="task">17 tasks</TermChip>
          </p>
          <TierChips tiers={["cheap_cloud", "premium", "embeddings"]} />
          <ModelRef provider="gemini" model="gemini-3.5-flash" />
          <p>
            <RowOpen label="Edit Everyday cloud" />
            <RowOpen
              label="Edit Search and retrieval"
              refusal="Search and retrieval has no settings of its own."
            />
          </p>
        </PanelBody>
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof Terms> = {
  title: "Settings/AI/AI models/Terms",
  component: Terms,
};
export default meta;
type Story = StoryObj<typeof Terms>;

export const Default: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
