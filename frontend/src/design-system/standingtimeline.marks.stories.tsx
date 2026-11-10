// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { UTC_ZONE } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { StandingTimeline, type TimelineMark } from "./standingtimeline";

const meta = {
  title: "Components/Text and data display/Standing timeline marks",
  component: StandingTimeline,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: "52rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof StandingTimeline>;
export default meta;

type Story = StoryObj<typeof meta>;

const mail = (
  key: string,
  at: string,
  when: string,
  side: TimelineMark["side"],
): TimelineMark => ({
  key,
  at,
  precision: "instant",
  kind: "email",
  side,
  what: "Email",
  title: "Re: walkthrough",
  detail: side === "theirs" ? "From Dana Reyes" : undefined,
  when,
});

/** A lone mail and a pair a day apart. Each chip's name carries its day, so
 *  neither reads as a second "Email" beside the record's own action. */
export const NamedByDay: Story = {
  args: {
    label: "Brandt Automotive over time",
    asOf: "2026-08-13T09:00:00Z",
    record: UTC_ZONE,
    events: [
      { key: "k", at: "2026-06-03", title: "Kickoff call", kind: "done" },
      { key: "c", at: "2026-09-30", title: "Expected close", kind: "end" },
    ],
    marks: [
      mail("1", "2026-06-10T09:00:00Z", "10 Jun", "theirs"),
      mail("2", "2026-08-11T09:00:00Z", "11 Aug", "theirs"),
      mail("3", "2026-08-12T09:00:00Z", "12 Aug", "ours"),
    ],
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const lone = await canvas.findByRole("button", { name: "Email, 10 Jun" });
    await expect(
      canvas.getByRole("button", { name: "2 entries, 11 Aug to 12 Aug" }),
    ).toBeInTheDocument();
    await expect(canvas.queryByRole("button", { name: "Email" })).toBeNull();
    lone.focus();
    await userEvent.keyboard("{Enter}");
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      await page.findByRole("region", { name: "Email" }),
    ).toHaveTextContent("From Dana Reyes · 10 Jun");
  },
};
