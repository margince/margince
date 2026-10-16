// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { EMPTY_DRAFT } from "../onboarding";
import {
  CompanyReviewScene,
  type ReviewSceneProps,
} from "./company-review-scene";
import type { DeckCard } from "./review-deck";
import "../../design-system/panel.css";

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

export const DeckRefused: Story = { args: { mode: "dossier" } };

export const WholeRecordRefused: Story = { args: { mode: "profile" } };

export const FactBoardRefused: Story = { args: { mode: "record" } };

export const DeckNotRefused: Story = {
  args: { mode: "dossier", error: null },
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
