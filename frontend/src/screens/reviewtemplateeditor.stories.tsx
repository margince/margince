// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof ReviewTemplateEditor> = {
  title: "Settings/Sales/Outcome reviews/Edit questions",
  component: ReviewTemplateEditor,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    onClose: () => {},
    template: {
      id: "11111111-1111-4111-8111-111111111111",
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
          required: false,
          options: ["Fit, scope", "Trust", "Other"],
        },
      ],
    },
  },
};
export default meta;
type Story = StoryObj<typeof meta>;
export const MultipleChoices: Story = {};
export const MultipleChoicesDark: Story = { globals: { theme: "dark" } };
