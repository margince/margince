// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Card } from "./atoms";
import {
  GroupedTimelineList,
  type TimelineEntry,
  TimelineList,
} from "./composed";

const meta: Meta<typeof TimelineList> = {
  title: "Components/Text and data display/Timeline list",
  component: TimelineList,
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Card style={{ maxWidth: 640 }}>
          <Story />
        </Card>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof TimelineList>;

const ZONE = "Europe/Berlin";

function entry(
  id: string,
  atIso: string,
  overrides: Partial<TimelineEntry>,
): TimelineEntry {
  return {
    id,
    kind: "email",
    title: "Re: Fleet renewal",
    atIso,
    provenance: { kind: "connector", connector: "gmail" },
    ...overrides,
  };
}

// Every row carries its time under its day: the day alone cannot say which of
// two calls came first.
export const Flat: Story = {
  render: () => (
    <TimelineList
      zone={ZONE}
      entries={[
        entry("f1", "2026-08-14T15:20:00Z", {
          direction: "inbound",
          counterparts: "Dana Buyer",
          body: "Can you hold the price until Friday?",
        }),
        entry("f2", "2026-08-14T09:05:00Z", {
          kind: "call",
          title: "Call about the renewal",
          direction: "outbound",
          counterparts: "Dana Buyer",
          provenance: { kind: "human", self: true },
        }),
        entry("f3", "2026-08-12T13:00:00Z", {
          kind: "meeting",
          title: "Quarterly review",
          counterparts: "Dana Buyer, Marc Dubois",
          provenance: { kind: "agent", agent: "capture" },
        }),
        entry("f4", "2026-08-11T10:30:00Z", {
          kind: "note",
          title: "Budget approved for Q4",
          provenance: { kind: "human", self: false },
          // A note is drawn as markdown; the mail above stays plain text.
          body: "## Next steps\n\n- Send the revised quote\n- Book the **review** call\n\nTerms: https://example.com/terms",
        }),
      ]}
    />
  ),
};

const threadMembers = [
  entry("t3", "2026-08-13T16:40:00Z", {
    threadKey: "t-fleet",
    direction: "inbound",
    counterparts: "Dana Buyer",
    body: "Friday works. Send the revised quote.",
  }),
  entry("t2", "2026-08-13T11:10:00Z", {
    threadKey: "t-fleet",
    direction: "outbound",
    counterparts: "Dana Buyer",
    provenance: { kind: "human", self: true },
    body: "We can hold it until Friday.",
  }),
  entry("t1", "2026-08-12T08:55:00Z", {
    threadKey: "t-fleet",
    direction: "inbound",
    counterparts: "Dana Buyer",
    body: "Can you hold the price until Friday?",
  }),
];

const bulkCopies = ["Dana Buyer", "Marc Dubois", "Ida Keller"].map(
  (name, index) =>
    entry(`b${index + 1}`, "2026-08-10T07:30:00Z", {
      title: "Price list 2027",
      direction: "outbound",
      counterparts: name,
      provenance: { kind: "human", self: true },
      body: "The new price list is attached.",
    }),
);

// A thread stands open as one card; a bulk send folds behind its newest copy;
// the last group may continue past the page, and says so.
export const Grouped: Story = {
  render: () => (
    <GroupedTimelineList
      zone={ZONE}
      groups={[
        {
          id: "s1",
          kind: "single",
          partial: false,
          entries: [
            entry("s1", "2026-08-14T09:05:00Z", {
              kind: "call",
              title: "Call about the renewal",
              direction: "outbound",
              counterparts: "Dana Buyer",
              provenance: { kind: "human", self: true },
            }),
          ],
        },
        { id: "t3", kind: "thread", partial: false, entries: threadMembers },
        { id: "b1", kind: "bulk", partial: false, entries: bulkCopies },
        {
          id: "b1-old",
          kind: "bulk",
          partial: true,
          entries: bulkCopies.map((copy) => ({
            ...copy,
            id: `${copy.id}-old`,
            atIso: "2026-07-01T07:30:00Z",
            title: "Summer closing hours",
          })),
        },
      ]}
    />
  ),
};
