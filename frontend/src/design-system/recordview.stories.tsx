// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Ellipsis } from "lucide-react";
import { Badge, Button } from "./atoms";
import { FactList } from "./factlist";
import { Panel, PanelBody } from "./panel";
import { RecordBack, RecordView } from "./recordview";
import type { TimelineEntry } from "./timelineentry";

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

// Not `Meta<…>`: with a bare StoryObj it lets a story drop the required identity.
const meta = {
  title: "Components/Layout and structure/Record view",
  component: RecordView,
} satisfies Meta<typeof RecordView>;
export default meta;

type Story = StoryObj<typeof meta>;

const verbs = (
  <>
    <Button onClick={() => {}}>Log activity</Button>
    <Button onClick={() => {}}>Add task</Button>
    <Button iconOnly aria-label="More actions" onClick={() => {}}>
      <Ellipsis aria-hidden="true" />
    </Button>
  </>
);

const details = (
  <FactList
    flat
    facts={[
      { key: "domain", term: "Domain", value: "brandt.example" },
      { key: "industry", term: "Industry", value: "Automotive" },
      { key: "owner", term: "Owner", value: "Lena Fischer" },
    ]}
  />
);

// The record every page draws: the head over its details on the left, the tabs
// over the feed on the right. The pane in the feed rests on its hairline alone;
// only the two cards carry a shadow.
export const Default: Story = {
  args: {
    name: "Brandt Automotive GmbH",
    identity: "o-1",
    zone: "Europe/Berlin",
    nameBadge: (
      <>
        <span>Automotive</span>
        <a className="entity-link" href="#/companies/o-1">
          brandt.example
        </a>
      </>
    ),
    pulse: (
      <>
        <Badge tone="success">Customer</Badge>
        <Badge>Shared</Badge>
      </>
    ),
    actions: verbs,
    aside: details,
    tabs: <span>Overview · History · Contacts</span>,
    children: (
      <Panel title="Next">
        <PanelBody>The open tab's feed.</PanelBody>
      </Panel>
    ),
  },
};

// Opened from a queue that offers the way back to it.
export const WithWayBack: Story = {
  args: {
    ...Default.args,
    back: <RecordBack href="#/home?queue=1" label="Back to Worklist" />,
  },
};

// The story under the open tab, with a row action each entry may carry.
export const WithTimeline: Story = {
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

// A long German name wraps beside the mark for up to three lines and is cut
// after that. The verbs share the card's width whatever the words.
export const LongGermanName: Story = {
  args: {
    ...Default.args,
    name: "Brandt Automobilzulieferer und Sondermaschinenbau Gesellschaft mit beschränkter Haftung",
    actions: (
      <>
        <Button onClick={() => {}}>Aktivität erfassen</Button>
        <Button onClick={() => {}}>Aufgabe erstellen</Button>
        <Button iconOnly aria-label="Weitere Aktionen" onClick={() => {}}>
          <Ellipsis aria-hidden="true" />
        </Button>
      </>
    ),
  },
};

// A record with nothing but its name: the head stands alone, and no empty
// details section is drawn under it.
export const Bare: Story = {
  args: {
    name: "New company",
    identity: "o-new",
    zone: "Europe/Berlin",
    children: <p>Nothing logged yet.</p>,
  },
};

// Under 900px of row the details card folds above the feed and takes the full
// width; framed at 390px, where the fold is the layout.
export const Narrow: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 390 }}>
        <Story />
      </div>
    ),
  ],
  args: { ...Default.args },
};
