// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import { OutcomeReviewModal } from "./outcomereviewmodal";

const meta: Meta<typeof OutcomeReviewModal> = {
  title: "Records/Deal 360/Write outcome choices",
  component: OutcomeReviewModal,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    open: true,
    onClose: () => {},
    dealId: "11111111-1111-4111-8111-111111111111",
    closingOccurrenceId: "22222222-2222-4222-8222-222222222222",
    template: {
      id: "33333333-3333-4333-8333-333333333333",
      key: "win_review",
      label: "Win review",
      outcome: "won",
      version: 1,
      active: true,
      system: true,
      created_at: "2026-09-01T10:00:00Z",
      updated_at: "2026-09-01T10:00:00Z",
      questions: [
        { key: "why", label: "Why did we win?", type: "text", required: true },
        {
          key: "reasons",
          label: "Reasons",
          type: "multiselect",
          required: true,
          options: ["Fit, scope", "Trust", "Other"],
        },
      ],
    },
  },
};
export default meta;
type Story = StoryObj<typeof meta>;
export const RequiredChoices: Story = {};
export const RequiredChoicesDark: Story = { globals: { theme: "dark" } };
