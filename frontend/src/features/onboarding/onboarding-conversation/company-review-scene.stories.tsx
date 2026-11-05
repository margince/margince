// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { StoryProviders } from "../../../testing/story-utils";
import { EMPTY_DRAFT } from "../onboarding";
import {
  CompanyReviewScene,
  type ReviewSceneProps,
} from "./company-review-scene";
import type { DeckCard } from "./review-deck";
import "../../../design-system/panel.css";

const CARD: DeckCard = {
  field: "icp",
  question: "Ideal customer",
  required: true,
  multiline: true,
  value: "",
};

const REFUSAL = "The company name must not be blank.";

const BASE: ReviewSceneProps = {
  mode: "dossier",
  onSwitchMode: () => {},
  onReadWhole: () => {},
  onSettle: () => {},
  goToField: null,
  proposal: {
    ready: true,
    fields: [],
    facts: [],
    open_questions: [],
    remaining_required_fields: [],
    draft_version: 1,
    proposal_hash: "story",
  },
  read: null,
  companyId: undefined,
  draft: EMPTY_DRAFT,
  setField: () => {},
  rows: {
    all: [],
    cards: [CARD],
    cardOf: () => CARD,
    openQuestions: 0,
  },
  answers: [],
  missing: [],
  selectedFactKeys: [],
  setSelectedFactKeys: () => {},
  confirm: { run: () => {}, pending: false, blockers: [], held: false },
  authorizing: false,
  error: REFUSAL,
};

function Scene(props: Partial<ReviewSceneProps>) {
  return (
    <StoryProviders>
      <CompanyReviewScene {...BASE} {...props} />
    </StoryProviders>
  );
}

const meta: Meta<typeof Scene> = {
  title: "Onboarding/Conversation/Company review scene",
  component: Scene,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof Scene>;

async function expectOneRefusal(canvasElement: HTMLElement) {
  const alerts = await within(canvasElement).findAllByRole("alert");
  await expect(alerts).toHaveLength(1);
  await expect(alerts[0]).toHaveTextContent(REFUSAL);
}

export const DeckRefused: Story = {
  args: { mode: "dossier" },
  play: ({ canvasElement }) => expectOneRefusal(canvasElement),
};

export const WholeRecordRefused: Story = {
  args: { mode: "profile" },
  play: ({ canvasElement }) => expectOneRefusal(canvasElement),
};

export const WholeRecordEditedRefused: Story = {
  args: {
    mode: "profile",
    draft: { ...EMPTY_DRAFT, edited: new Set(["display_name"]) },
  },
  play: ({ canvasElement }) => expectOneRefusal(canvasElement),
};

export const FactBoardRefused: Story = {
  args: { mode: "record" },
  play: ({ canvasElement }) => expectOneRefusal(canvasElement),
};

export const DeckNotRefused: Story = {
  args: { mode: "dossier", error: null },
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", {
      name: "Confirm profile",
    });
    await expect(within(canvasElement).queryByRole("alert")).toBeNull();
  },
};

export const DeckRefusedPhone: Story = {
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
  args: { mode: "dossier" },
};

export const WholeRecordRefusedPhone: Story = {
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
  args: { mode: "profile" },
};

function RefusedOnSave(props: Partial<ReviewSceneProps>) {
  const [error, setError] = useState<string | null>(null);
  return (
    <Scene
      {...props}
      error={error}
      confirm={{ ...BASE.confirm, run: () => setError(REFUSAL) }}
    />
  );
}

export const WholeRecordRefusedAfterSave: Story = {
  render: (args) => <RefusedOnSave {...args} />,
  args: {
    mode: "profile",
    draft: { ...EMPTY_DRAFT, edited: new Set(["display_name"]) },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const save = await canvas.findByRole("button", { name: "Save changes" });
    await userEvent.click(save);
    const alert = await canvas.findByRole("alert");
    await expect(alert).toHaveTextContent(REFUSAL);
    // The foot fills in mount order; the refusal must still lead it.
    await expect(getComputedStyle(alert).order).toBe("-1");
  },
};
