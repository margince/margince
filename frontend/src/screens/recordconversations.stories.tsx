// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { TimelineEntry, TimelineGroup } from "../design-system/composed";
import { ConversationList } from "./recordconversations";
import { StoryProviders } from "./story-utils";

// The record's exchanges as CONVERSATIONS: one row per thread, whose move it
// is, and — expanded — the same TimelineRow the chronicle renders. Two facts
// the screenshots below carry that a single glance would miss: a thread that
// ended on their word reads "Your move", never the reverse, and a group cut
// by the page's edge says so with the "may continue earlier" caption rather
// than looking finished.

function entry(
  kind: TimelineEntry["kind"],
  overrides: Partial<TimelineEntry> = {},
): TimelineEntry {
  return {
    id: overrides.id ?? `${kind}-1`,
    kind,
    title: overrides.title ?? kind,
    atIso: overrides.atIso ?? "2026-07-01T10:00:00Z",
    provenance: { kind: "human", self: false },
    ...overrides,
  };
}

function group(
  id: string,
  entries: TimelineEntry[],
  partial = false,
): TimelineGroup {
  return {
    id,
    kind: entries.length > 1 ? "thread" : "single",
    entries,
    partial,
  };
}

function List({ groups }: Readonly<{ groups: readonly TimelineGroup[] }>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <ConversationList groups={groups} zone="UTC" />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof List> = {
  title: "Records/Conversations",
  component: List,
};
export default meta;
type Story = StoryObj<typeof List>;

// Two threads, one waiting on us and one waiting on them, each with more than
// one message so the thread count reads as plural.
export const Populated: Story = {
  render: () => (
    <List
      groups={[
        group("thread-1", [
          entry("email", {
            id: "renewal-2",
            title: "Re: Renewal terms",
            atIso: "2026-08-20T09:00:00Z",
            direction: "inbound",
            counterparts: "Ida Keller",
            body: "Can we push the renewal date by two weeks while procurement signs off?",
          }),
          entry("email", {
            id: "renewal-1",
            title: "Renewal terms",
            atIso: "2026-08-18T09:00:00Z",
            direction: "outbound",
            counterparts: "Ida Keller",
            body: "Here is the renewal proposal for the next term.",
          }),
        ]),
        group("thread-2", [
          entry("message", {
            id: "onboarding-2",
            title: "Onboarding checklist",
            atIso: "2026-08-19T14:00:00Z",
            direction: "outbound",
            counterparts: "Marc Dubois",
            body: "Sent over the checklist — let us know if anything is unclear.",
          }),
          entry("message", {
            id: "onboarding-1",
            title: "Onboarding kickoff",
            atIso: "2026-08-17T14:00:00Z",
            direction: "inbound",
            counterparts: "Marc Dubois",
            body: "We are ready to start onboarding whenever you are.",
          }),
        ]),
      ]}
    />
  ),
};

// The same two threads in dark: "Your move" and the counterpart's name carry
// no colour of their own, but the row's inbound/outbound marks do, so this is
// the story that shows whether either mark still reads against the dark card.
export const PopulatedDark: Story = {
  ...Populated,
  globals: { theme: "dark" },
};

// No conversation-kind groups at all — a fact about the account, drawn as the
// honest EmptyState rather than a list with nothing in it.
export const Empty: Story = { render: () => <List groups={[]} /> };

// The page's edge cut this thread's older half, so it may continue earlier
// than what the reader can see. The caption sits beside the expand control,
// not folded into the count, because the count is what the page HOLDS and the
// caption is what the page cannot promise.
export const Partial: Story = {
  render: () => (
    <List
      groups={[
        group(
          "thread-1",
          [
            entry("email", {
              id: "support-2",
              title: "Re: Support ticket 4021",
              atIso: "2026-08-21T09:00:00Z",
              direction: "inbound",
              counterparts: "Priya Shah",
              body: "Thanks, that resolved it on our end.",
            }),
            entry("email", {
              id: "support-1",
              title: "Support ticket 4021",
              atIso: "2026-08-15T09:00:00Z",
              direction: "outbound",
              counterparts: "Priya Shah",
              body: "We are looking into the reported outage now.",
            }),
          ],
          true,
        ),
      ]}
    />
  ),
};
