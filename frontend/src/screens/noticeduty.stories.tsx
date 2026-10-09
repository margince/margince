// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { NoticeDuty } from "./noticeduty";
import { StoryProviders, stubWithSession } from "./story-utils";
import type { WorklistItem } from "./worklist.queries";

// The privacy-notice duty on the worklist's pane: what the duty is, what its
// deadline rests on, and the three ways to discharge it.

const CONTACT = "01a05500-0000-7000-8000-0000000000aa";

const IMPORTED: WorklistItem = {
  id: "01a05500-0000-7000-8000-0000000000cc",
  source: "notice_case",
  category: "system",
  level: 2,
  consequence: "legal_deadline_missed",
  because: [],
  actions: ["open"],
  kind: "art14",
  due_at: "2026-02-03T09:00:00Z",
  owner: { kind: "user", id: "u-1", label: "Anna Weber" },
  acquisition: {
    kind: "purchased_or_imported",
    occurred_at: "2026-01-03T09:00:00Z",
    captured_at: "2026-01-04T09:00:00Z",
    captured_by: "human:u-1",
    captured_by_name: "Anna Weber",
  },
  subject: { type: "contact", id: CONTACT, label: "Steve Leichsenring" },
};

// The door recorded no date, and a connector rather than a colleague wrote it.
const UNDATED: WorklistItem = {
  ...IMPORTED,
  owner: { kind: "unassigned" },
  acquisition: {
    kind: "mailbox_history",
    occurred_at: null,
    captured_at: "2026-01-04T09:00:00Z",
    captured_by: "connector:gmail",
    captured_by_name: null,
  },
};

const UNEVIDENCED: WorklistItem = { ...IMPORTED, acquisition: null };

function duty(item: WorklistItem) {
  return () => {
    stubWithSession({}, { contact: ["read", "update"] });
    return (
      <StoryProviders>
        <NoticeDuty item={item} contactId={CONTACT} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof NoticeDuty> = {
  title: "Records/Worklist/Privacy notice duty",
  component: NoticeDuty,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof NoticeDuty>;

export const Imported: Story = { render: duty(IMPORTED) };

export const DateUnknown: Story = { render: duty(UNDATED) };

/** No evidence row: the pane says so instead of presenting a bare deadline. */
export const NoEvidence: Story = { render: duty(UNEVIDENCED) };

export const ImportedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: duty(IMPORTED),
};

export const ImportedDark: Story = {
  globals: { theme: "dark" },
  render: duty(IMPORTED),
};
