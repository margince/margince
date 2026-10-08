// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import { LocaleProvider, useLocale, useT } from "../i18n";
import { StoryProviders } from "../screens/story-utils";
import { groupChronology } from "../screens/timelinegroups";
import { activityTimeline } from "./activitytimeline";
import { Avatar, Card } from "./atoms";
import {
  GroupedTimelineList,
  type TimelineEntry,
  TimelineList,
} from "./composed";
import type { RecordContact } from "./participants";

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

// `activityTimeline` adapts the contract's activities to the rows above, and a
// record page mounts the result in this list. What is worth a picture is whose
// face a message wears: the page's own chip sits above the list, so a sender
// who IS the page's contact reads in the same colour, and anybody else in
// their own.

type Activity = components["schemas"]["Activity"];

const dana: RecordContact = { id: "p-dana", full_name: "Dana Buyer" };

// Two inbound messages of one conversation, filed against `links`, each naming
// its sender with the server's phrase when one is given.
function thread(links: Activity["links"], sender: string | null): Activity[] {
  return ["m-2", "m-1"].map((id, at) => ({
    id,
    kind: "email",
    subject: "Fleet renewal",
    body: "Can you hold the price until Friday?",
    occurred_at: `2026-08-1${2 - at}T12:00:00Z`,
    direction: "inbound",
    thread_key: "t-fleet",
    is_done: false,
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-08-10T12:00:00Z",
    updated_at: "2026-08-10T12:00:00Z",
    links,
    email_summary: sender
      ? {
          activity_id: id,
          occurred_at: `2026-08-1${2 - at}T12:00:00Z`,
          version: 1,
          subject: "Fleet renewal",
          preview: "Can you hold the price until Friday?",
          counterparty: sender,
          direction: "inbound",
          display_status: "team",
          move: "needs_reply",
          attachment_count: 0,
        }
      : undefined,
  }));
}

function Chronology({
  activities,
  about,
  names,
}: Readonly<{
  activities: Activity[];
  // The contact whose page this is, drawn as its own chip above the list.
  about?: RecordContact;
  // What the page can call each linked contact; absent resolves nobody.
  names?: Readonly<Record<string, string>>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const entries = activityTimeline(
    activities,
    undefined,
    undefined,
    names ? { nameOf: (_type, id) => names[id], t, locale } : undefined,
  );
  return (
    <StoryProviders>
      {about && <Avatar name={about.full_name} identity={about.id} size="md" />}
      <GroupedTimelineList
        zone={viewerZone()}
        groups={groupChronology(entries)}
      />
    </StoryProviders>
  );
}

const filedOnDana: Activity["links"] = [
  { entity_type: "contact", entity_id: dana.id },
];

// The phrase names the page's contact, so each face is keyed on her record and
// matches the chip above.
export const SenderIsThePagesContact: Story = {
  render: () => (
    <Chronology activities={thread(filedOnDana, "Dana Buyer")} about={dana} />
  ),
};

// Filed against Dana, written by somebody the phrase names instead: the face
// is his, in his own colour, never a stranger's name in Dana's.
export const SenderIsSomebodyElse: Story = {
  render: () => (
    <Chronology activities={thread(filedOnDana, "Bob Stranger")} about={dana} />
  ),
};

// Two contact records carrying one name: the thread names her once, and each
// face stays keyed on its own record.
export const TwoContactsShareAName: Story = {
  render: () => (
    <Chronology
      activities={thread(
        [
          { entity_type: "contact", entity_id: "p-ida" },
          { entity_type: "contact", entity_id: "p-ida-2" },
        ],
        null,
      )}
      names={{ "p-ida": "Ida Keller", "p-ida-2": "Ida Keller" }}
    />
  ),
};

// The sign-off and the quoted history fold behind their own control: the split
// is a heuristic, so a wrong guess stays one click from visible.
export const MailWithSignatureAndQuote: Story = {
  render: () => (
    <TimelineList
      zone={ZONE}
      entries={[
        entry("m1", "2026-08-13T09:12:00Z", {
          title: "Re: Rollout-Plan",
          direction: "inbound",
          counterparts: "Lena Fischer",
          body: [
            "From: lena.fischer@acme.de",
            "To: lars@gradion.com",
            "",
            "Hallo,",
            "",
            "der Plan sieht gut aus. Phase 1 ist realistisch, bei Phase 3 haben wir",
            "intern noch Klärungsbedarf mit dem Händlerteam. Details stehen unter",
            "https://acme.de/rollout bereit.",
            "",
            "Dienstag 14 Uhr würde bei uns passen.",
            "",
            "Mit freundlichen Grüßen",
            "Lena Fischer",
            "Acme GmbH · +49 89 123456",
            "",
            "Am 12.08.2026 um 09:14 schrieb Lars Jankowfsky:",
            "> Passt ein Termin nächste Woche für die Detailabstimmung?",
          ].join("\n"),
        }),
      ]}
    />
  ),
};

// A note is never folded: "Viele Grüße" opens it as prose, and a quoted line
// in a note is something a human typed.
export const NoteThatReadsLikeASignOff: Story = {
  render: () => (
    <TimelineList
      zone={ZONE}
      entries={[
        entry("n1", "2026-08-13T16:30:00Z", {
          kind: "note",
          title: "Nach dem Messegespräch",
          provenance: { kind: "human", self: true },
          body: [
            "Viele Grüße von der Messe ausgerichtet, Lena war sichtlich erfreut.",
            "> Sie fragte nochmal nach dem Händlerportal.",
          ].join("\n"),
        }),
      ]}
    />
  ),
};
