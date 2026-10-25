// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import type { components } from "../api/schema";
import { formatActivityWhen } from "../format/activitywhen";
import { UTC_ZONE } from "../format/timezone";
import { useLocale } from "../i18n";
import { en } from "../i18n/en";
import { StoryProviders } from "../testing/story-utils";
import { EmailEnvelope } from "./emailmessage";

// The envelope's date line, read through the one activity-time reader. A
// message logged with only its day shows that day and no clock.

type EmailPresentation = components["schemas"]["EmailPresentation"];

const ZONE = UTC_ZONE;

function presentation(
  overrides: Partial<EmailPresentation> = {},
): EmailPresentation {
  return {
    id: "act-1",
    lifecycle: "delivered",
    occurred_at: "2026-09-01T09:00:00Z",
    occurred_precision: "instant",
    version: 1,
    summary: {
      activity_id: "act-1",
      occurred_at: "2026-09-01T09:00:00Z",
      occurred_precision: "instant",
      version: 1,
      subject: "Pilot slots for the review",
      display_status: "team",
      move: "needs_reply",
      attachment_count: 0,
    },
    body: "",
    from: [{ address: "minh@example.com", display_name: "Nhật Minh Nguyễn" }],
    to: [{ address: "lars@example.com", display_name: "Lars" }],
    cc: [],
    bcc: [],
    bcc_withheld: false,
    attachments: [],
    links: [],
    thread: { members: [], next_cursor: null },
    can_reply: false,
    can_relink: false,
    access: {
      content_state: "available",
      display_status: "team",
      audience: "workspace",
      can_change: false,
      change_mode: "none",
    },
    ...overrides,
  };
}

function Envelope({ shown }: Readonly<{ shown: EmailPresentation }>) {
  const { locale } = useLocale();
  return (
    <EmailEnvelope
      presentation={shown}
      formatWhen={(at, precision) =>
        formatActivityWhen(at, precision, locale, ZONE)
      }
    />
  );
}

const meta = {
  title: "Components/Text and data display/Email envelope",
  component: Envelope,
  parameters: { layout: "padded" },
  render: (args) => (
    <StoryProviders>
      <Envelope {...args} />
    </StoryProviders>
  ),
} satisfies Meta<typeof Envelope>;
export default meta;

type Story = StoryObj<typeof meta>;

export const SentAtATime: Story = { args: { shown: presentation() } };

export const KnownByItsDay: Story = {
  args: {
    shown: presentation({
      occurred_at: "2026-09-01T12:00:00Z",
      occurred_precision: "day",
    }),
  },
  play: async () => {
    const when = (await screen.findByText(en["email.detail.when"])).closest(
      "p",
    );
    await expect(when).toHaveTextContent("2026");
    await expect(when?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};
