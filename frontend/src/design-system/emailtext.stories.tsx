import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { EmailText } from "./emailtext";

const meta = {
  title: "Components/Text and data display/Email text",
  component: EmailText,
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof EmailText>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Paragraphs: Story = {
  args: { body: "Hello Ada,\n\nCan we meet on Tuesday?\n\nBest regards\nSam" },
};
export const QuotedHistory: Story = {
  args: {
    body: "Tuesday works.\n\nOn Monday Ada wrote:\n> Can we meet on Tuesday?",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const summary = canvas.getByText("Show quoted history").closest("summary");
    await expect(summary).toHaveAttribute("aria-expanded", "false");
    await expect(summary?.lastElementChild).toHaveClass("expander-chevron");
    if (summary) await userEvent.click(summary);
    await expect(summary).toHaveAttribute("aria-expanded", "true");
    await expect(canvas.getByText(/Can we meet on Tuesday/)).toBeVisible();
  },
};
