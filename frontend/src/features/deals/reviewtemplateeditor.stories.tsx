// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, within } from "storybook/test";
import { translate } from "../../i18n";
import { StoryProviders } from "../../testing/story-utils";
import { ReviewTemplateEditor } from "./reviewtemplateeditor";

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
/** The drawer: question 1 carries the prefill note; a list question its choices. */
export const Drawer: Story = {};
export const DrawerDark: Story = { globals: { theme: "dark" } };
export const DrawerPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A built-in question shows in the reader's language; an added one as typed. */
export const Vietnamese: Story = {
  render: (args) => (
    <StoryProviders locale="vi">
      <ReviewTemplateEditor
        {...args}
        template={{
          ...args.template,
          label_key: "reviewTemplates.builtin.win_review",
          questions: [
            {
              key: "why_we_won",
              label: "Why did we win?",
              label_key: "reviewTemplates.builtin.question.why_we_won",
              type: "text",
              required: true,
            },
            ...args.template.questions.slice(1),
          ],
        }}
      />
    </StoryProviders>
  ),
  // The drawer fades in, so its name and fields are read, not its opacity.
  play: async () => {
    const drawer = await screen.findByRole("dialog", {
      name: translate("vi", "reviewTemplates.builtin.win_review"),
    });
    await expect(
      within(drawer).getByDisplayValue(
        translate("vi", "reviewTemplates.builtin.question.why_we_won"),
      ),
    ).toBeInTheDocument();
  },
};
