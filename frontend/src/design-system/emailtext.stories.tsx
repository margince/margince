import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import { EmailReading, EmailText } from "./emailtext";

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
  args: {
    body: "Hello Ada,\n\nCan we meet on Tuesday?\n\nBest regards\nSam",
    subject: "Meeting",
  },
};
export const QuotedHistory: Story = {
  args: {
    body: "Tuesday works.\n\nOn Monday Ada wrote:\n> Can we meet on Tuesday?",
    subject: "Re: Meeting",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const summary = canvas
      .getByText(englishCopy["email.detail.showQuoted"])
      .closest("summary");
    await expect(summary).toHaveAttribute("aria-expanded", "false");
    await expect(summary?.lastElementChild).toHaveClass("expander-chevron");
    if (summary) await userEvent.click(summary);
    await expect(summary).toHaveAttribute("aria-expanded", "true");
    await expect(canvas.getByText(/Can we meet on Tuesday/)).toBeVisible();
  },
};

// An Outlook forward has no forward line, so only its subject keeps the
// forwarded mail in the message.
export const OutlookForward: Story = {
  args: {
    body: "FYI\n\nFrom: Ada\nSent: Monday\nTo: Sam\n\nCan we meet on Tuesday?",
    subject: "FW: Meeting",
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText(/Can we meet on Tuesday/)).toBeVisible();
    await expect(
      canvas.queryByText(englishCopy["email.detail.showQuoted"]),
    ).toBeNull();
  },
};
export const NoText: Story = {
  args: { body: "", subject: null },
  render: () => (
    <EmailReading
      activityId="01a05500-0000-7000-8000-0000000000a1"
      language={null}
      body=""
      subject={null}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(englishCopy["email.thread.noText"]),
    ).toBeVisible();
    await expect(
      canvas.queryByRole("button", {
        name: englishCopy["readerRendering.show"],
      }),
    ).toBeNull();
  },
};
