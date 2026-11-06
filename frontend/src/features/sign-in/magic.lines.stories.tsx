// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { browserZone } from "../../format/timezone";
import type { MagicLine } from "../../lib/magic.queries";
import { StoryProviders } from "../../testing/story-utils";
import { MagicLaneSection } from "./magic.lines";

// The receipt's watching lane, where a connector that broke overnight names
// itself by its provider's readable name and the account it reads.

const system: MagicLine["actor"] = { type: "system", id: "capture" };
const NOT_UNDOABLE = {
  undoable: false,
  reason: "not_a_replayable_verb",
} as const;

const WATCHING: MagicLine[] = [
  {
    id: "00000000-0000-7000-8000-0000000c0001",
    occurred_at: "2026-09-13T05:10:00Z",
    lane: "watching",
    summary: {
      key: "magic.action.capture_reauth_required",
      values: { provider: "gcal" },
    },
    actor: system,
    undo: NOT_UNDOABLE,
  },
  {
    id: "00000000-0000-7000-8000-0000000c0002",
    occurred_at: "2026-09-13T06:20:00Z",
    lane: "watching",
    summary: {
      key: "magic.action.capture_sync_failing",
      values: { provider: "imap", account: "sales@example.org" },
    },
    actor: system,
    undo: NOT_UNDOABLE,
  },
];

function lane() {
  return (
    <StoryProviders>
      <Panel title="Since your last brief">
        <MagicLaneSection
          lane="watching"
          title="Needs restoring"
          rows={WATCHING}
          since="2026-09-12T18:00:00Z"
          zone={browserZone()}
          byDay={false}
        />
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof MagicLaneSection> = {
  title: "Shell/Home/Receipt lines",
  component: MagicLaneSection,
};
export default meta;
type Story = StoryObj<typeof MagicLaneSection>;

export const ConnectorsToRestore: Story = { render: lane };

export const ConnectorsToRestoreDark: Story = {
  globals: { theme: "dark" },
  render: lane,
};

export const UnsupportedReceipt: Story = {
  render: (args) => (
    <StoryProviders>
      <Panel title="Activity">
        <MagicLaneSection {...args} />
      </Panel>
    </StoryProviders>
  ),
  args: {
    lane: "done",
    title: "Activity",
    since: "2026-09-12T18:00:00Z",
    zone: browserZone(),
    byDay: false,
    rows: [
      {
        id: "00000000-0000-7000-8000-0000000c0001",
        occurred_at: "2026-09-13T05:10:00Z",
        actor: system,
        lane: "done",
        summary: { key: "newer_receipt_kind" },
      },
    ],
  },
};
export const UnsupportedReceiptDark: Story = {
  ...UnsupportedReceipt,
  globals: { theme: "dark" },
};

// A source failing for days dates its line from when it began, the longest
// time a line carries.
const FAILING: MagicLine[] = [
  {
    id: "00000000-0000-7000-8000-0000000c0003",
    occurred_at: "2026-09-13T06:20:00Z",
    lane: "watching",
    summary: {
      key: "magic.action.capture_sync_failing",
      values: {
        provider: "imap",
        account: "sales@example.org",
        failing_since: "2026-09-10T07:35:00Z",
      },
    },
    actor: system,
    undo: NOT_UNDOABLE,
  },
  {
    id: "00000000-0000-7000-8000-0000000c0004",
    occurred_at: "2026-09-13T05:10:00Z",
    lane: "watching",
    summary: {
      key: "magic.action.capture_reauth_required",
      values: { provider: "gcal" },
    },
    actor: system,
    undo: NOT_UNDOABLE,
  },
];

function failingLane() {
  return (
    <StoryProviders>
      <Panel title="Since your last brief">
        <MagicLaneSection
          lane="watching"
          title="Needs restoring"
          rows={FAILING}
          since="2026-09-12T18:00:00Z"
          zone={browserZone()}
          byDay={false}
        />
      </Panel>
    </StoryProviders>
  );
}

export const FailingSince: Story = { render: failingLane };

export const FailingSinceDark: Story = {
  globals: { theme: "dark" },
  render: failingLane,
};

function textLines(el: Element): number {
  const range = document.createRange();
  range.selectNodeContents(el);
  const tops = new Set(
    Array.from(range.getClientRects(), (r) => Math.round(r.top)),
  );
  return tops.size;
}

export const FailingSincePhone: Story = {
  render: failingLane,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const since = await within(canvasElement).findByText(/Failing since/);
    const sentence = since
      .closest(".magic-line")
      ?.querySelector(".magic-line-text");
    if (!sentence) {
      throw new Error("the failing source drew outside its line");
    }
    await expect(
      Math.round(sentence.getBoundingClientRect().width),
    ).toBeGreaterThan(Math.round(since.getBoundingClientRect().width));

    const shortTime = canvasElement.querySelector(
      ".magic-line-when:not(.magic-line-since)",
    );
    if (!shortTime) {
      throw new Error("the line without a failing source drew no time");
    }
    await expect(textLines(since)).toBeGreaterThan(1);
    await expect(textLines(shortTime)).toBe(1);
  },
};
