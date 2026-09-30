// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { StoryProviders } from "../screens/story-utils";
import { groupChronology } from "../screens/timelinegroups";
import { activityTimeline } from "./activitytimeline";
import { Avatar, Card } from "./atoms";
import { GroupedTimelineList } from "./composed";
import type { RecordContact } from "./participants";

// The adapter from the contract's activities to the chronicle's rows, drawn
// through the list a record page mounts it into. What is worth a picture is
// whose face a message wears: the page's own chip sits above the list, so a
// sender who IS the page's contact reads in the same colour, and anybody else
// in their own.

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
    about,
  );
  return (
    <Card style={{ maxWidth: 640 }}>
      {about && <Avatar name={about.full_name} identity={about.id} size="md" />}
      <GroupedTimelineList
        zone={viewerZone()}
        groups={groupChronology(entries)}
      />
    </Card>
  );
}

const meta = {
  title: "Components/Text and data display/Activity timeline",
  component: Chronology,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof Chronology>;

export default meta;
type Story = StoryObj<typeof meta>;

const filedOnDana: Activity["links"] = [
  { entity_type: "contact", entity_id: dana.id },
];

// The phrase names the page's contact, so each face is keyed on her record and
// matches the chip above.
export const SenderIsThePagesContact: Story = {
  args: { activities: thread(filedOnDana, "Dana Buyer"), about: dana },
};

// Filed against Dana, written by somebody the phrase names instead: the face
// is his, in his own colour, never a stranger's name in Dana's.
export const SenderIsSomebodyElse: Story = {
  args: { activities: thread(filedOnDana, "Bob Stranger"), about: dana },
};

// Two contact records carrying one name: the thread names her once, and each
// face stays keyed on its own record.
export const TwoContactsShareAName: Story = {
  args: {
    activities: thread(
      [
        { entity_type: "contact", entity_id: "p-ida" },
        { entity_type: "contact", entity_id: "p-ida-2" },
      ],
      null,
    ),
    names: { "p-ida": "Ida Keller", "p-ida-2": "Ida Keller" },
  },
};
