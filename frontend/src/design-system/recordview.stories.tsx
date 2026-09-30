// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Ellipsis } from "lucide-react";
import { Button } from "./atoms";
import type { TimelineEntry } from "./composed";
import { RecordView } from "./recordview";

const emailEntry: TimelineEntry = {
  id: "a1",
  kind: "email",
  title: "Re: Q3 renewal terms",
  atIso: "2026-07-01T09:12:00Z",
  provenance: { kind: "human", self: true },
};
const meetingEntry: TimelineEntry = {
  id: "a2",
  kind: "meeting",
  title: "Discovery call",
  atIso: "2026-06-24T14:00:00Z",
  provenance: { kind: "agent", agent: "capture" },
};
const noteEntry: TimelineEntry = {
  id: "a3",
  kind: "note",
  title: "Left a voicemail",
  atIso: "2026-06-20T16:30:00Z",
  provenance: { kind: "human", self: true },
};
const baseTimeline: TimelineEntry[] = [emailEntry, meetingEntry, noteEntry];

// Not `Meta<…>`: with a bare StoryObj it lets a story drop the required identity.
const meta = {
  title: "Components/Layout and structure/Record view",
  component: RecordView,
} satisfies Meta<typeof RecordView>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    name: "Acme GmbH",
    identity: "o-acme",
    subtitle: "Enterprise · Munich",
    zone: "Europe/Berlin",
    timeline: baseTimeline,
  },
};

// The sign-off and the quoted history fold behind their own control: the split
// is a heuristic, so a wrong guess stays one click from visible.
export const MailWithSignatureAndQuote: Story = {
  args: {
    name: "Acme GmbH",
    identity: "o-acme",
    subtitle: "Enterprise · Munich",
    zone: "Europe/Berlin",
    timeline: [
      {
        ...emailEntry,
        title: "Re: Rollout-Plan",
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
      },
      meetingEntry,
    ],
  },
};

// A note is never folded: "Viele Grüße" opens it as prose, and a quoted line
// in a note is something a human typed.
export const NoteThatReadsLikeASignOff: Story = {
  args: {
    name: "Acme GmbH",
    identity: "o-acme",
    zone: "Europe/Berlin",
    timeline: [
      {
        ...noteEntry,
        title: "Nach dem Messegespräch",
        body: [
          "Viele Grüße von der Messe ausgerichtet, Lena war sichtlich erfreut.",
          "> Sie fragte nochmal nach dem Händlerportal.",
        ].join("\n"),
      },
    ],
  },
};

export const WithRowActions: Story = {
  args: {
    name: "Acme GmbH",
    identity: "o-acme",
    subtitle: "Enterprise · Munich",
    zone: "Europe/Berlin",
    timeline: [
      {
        ...emailEntry,
        actions: <Button onClick={() => {}}>Reply</Button>,
      },
      {
        ...meetingEntry,
        actions: <Button onClick={() => {}}>Relink</Button>,
      },
      noteEntry,
    ],
  },
};

// Five verbs and a refusal are wider than the record column, so the group folds
// onto a second rung; framed at 520px, where the fold is the layout.
export const InlineVerbsWiderThanTheHeader: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 520 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    name: "Brandt Automotive GmbH",
    subtitle: "Automotive · München",
    zone: "Europe/Berlin",
    identity: "o-1",
    actionsInline: true,
    actions: (
      <>
        <Button onClick={() => {}}>E-Mail schreiben</Button>
        <Button onClick={() => {}}>Aktivität erfassen</Button>
        <Button onClick={() => {}}>Aufgabe anlegen</Button>
        <Button onClick={() => {}}>Deal anlegen</Button>
        <Button iconOnly aria-label="Weitere Aktionen" onClick={() => {}}>
          <Ellipsis aria-hidden="true" />
        </Button>
        <p className="t-caption">
          Du kannst diese Firma nicht ändern. Bitte den Inhaber, sie mit dir zu
          teilen.
        </p>
      </>
    ),
    timeline: baseTimeline,
  },
};

export const NarrowInlineActionsWithExplanation: Story = {
  ...InlineVerbsWiderThanTheHeader,
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 390 }}>
        <Story />
      </div>
    ),
  ],
};
