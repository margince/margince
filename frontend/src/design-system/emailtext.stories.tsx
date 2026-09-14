import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { EmailText } from "./emailtext";

const meta = {
  title: "Design System/Email text",
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
};
