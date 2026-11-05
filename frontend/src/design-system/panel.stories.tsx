// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import {
  Badge,
  Button,
  Disclosure,
  EmptyState,
  Field,
  OverflowMenu,
  TextInput,
} from "./atoms";
import { DataTable } from "./datatable";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
  PanelPlate,
  PanelRow,
  RailPanel,
  type RailPanelState,
} from "./panel";
import { RowOpen } from "./rowopen";
import { SettingList, SettingRow } from "./settingrow";
import type { SectionDetail } from "./surfacestate";

// The titled-card shape: a header band, an optional footer band, and two ways
// to fill the middle. PanelBody holds padded prose; PanelRow runs to the edges.
const meta: Meta<typeof Panel> = {
  title: "Components/Layout and structure/Panel",
  component: Panel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 420 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof Panel>;

export const WithBody: Story = {
  args: {
    title: "Overview",
    children: (
      <PanelBody>
        <p>A short paragraph of read-only prose, padded to the panel edge.</p>
      </PanelBody>
    ),
  },
};

// The intro line in the four places it lands, drawn together because what this
// component owns is an INTERVAL and an interval is only judged against another
// one. Left to a screen sheet the same sentence stood at three distances from
// what it described. Reading down: --space-4 to the rows; nothing at all when
// the line IS the body, because whatever follows the body brings its own top
// padding; the same --space-4 between two stacked lines, and again under the
// pair; and the same --space-4 to a field, paid as the difference over the
// `.form-stack` gap that already separates the fields.
export const WithIntro: StoryObj = {
  decorators: [
    (Story) => (
      <div style={{ display: "grid", gap: "var(--space-4)" }}>
        <Story />
      </div>
    ),
  ],
  render: () => (
    <>
      <Panel title="Lead vocabulary">
        <PanelBody>
          <PanelIntro>
            The stages a lead moves through before it becomes a deal.
          </PanelIntro>
        </PanelBody>
        <PanelRow>Enquiry</PanelRow>
        <PanelRow>Qualified</PanelRow>
      </Panel>

      <Panel title="Data residency">
        <PanelBody>
          <PanelIntro>
            Records for this workspace are stored in the Frankfurt region.
          </PanelIntro>
        </PanelBody>
      </Panel>

      <Panel title="Offer templates">
        <PanelBody>
          <PanelIntro>
            The wording an offer is built from, and the currency it is priced
            in.
          </PanelIntro>
          <PanelIntro>
            You may read these templates. Editing them is an administrator's.
          </PanelIntro>
          <SettingList>
            <SettingRow
              label="Default template"
              value="Retrofit — standard"
              control={<Button>Change</Button>}
            />
          </SettingList>
        </PanelBody>
      </Panel>

      <Panel title="Sender address">
        <PanelBody className="form-stack">
          <PanelIntro>
            Replies to mail this workspace sends arrive at this address.
          </PanelIntro>
          <Field label="Reply-to">
            {(control) => (
              <TextInput {...control} defaultValue="hello@brandt.example" />
            )}
          </Field>
        </PanelBody>
      </Panel>
    </>
  ),
};

export const WithTitleAction: Story = {
  args: {
    title: "Open items",
    titleAction: <Badge tone="accent">3</Badge>,
    children: (
      <>
        <PanelRow>First row</PanelRow>
        <PanelRow>Second row</PanelRow>
        <PanelRow>Third row</PanelRow>
      </>
    ),
  },
};

export const WithFooter: Story = {
  args: {
    title: "Recent activity",
    children: (
      <>
        <PanelRow>Alpha note logged</PanelRow>
        <PanelRow>Beta call completed</PanelRow>
      </>
    ),
    footer: (
      <>
        <span>Two of six shown</span>
        <Button>See all</Button>
      </>
    ),
  },
};

// `titleLevel`: the outline, and only the outline. Put this beside `WithBody`
// above — the two titles are drawn at the same size, and what differs is that
// this one is an h3 a screen reader reads as sitting INSIDE the section above
// it rather than beside it. A title that shrank with its depth said the group
// mattered less than the box holding it, which is not what nesting means.
export const NestedTitleLevel: Story = {
  args: {
    title: "Passports",
    titleLevel: 3,
    children: (
      <PanelBody>
        <p>Two active, one revoked this morning.</p>
      </PanelBody>
    ),
  },
};

// A name longer than the pane at 320px: the verbs drop under it unsqueezed, and
// the title wraps whole. Wider, the band returns to one 56px line.
export const LongTitleWraps: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 320 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    title: "Brandt Automotive Zulieferbetriebe Deutschland GmbH & Co. KG",
    titleAction: (
      <>
        <Badge tone="accent">7</Badge>
        <Button>Add</Button>
      </>
    ),
    children: (
      <>
        <PanelRow>Contract performance</PanelRow>
        <PanelRow>Legitimate interest — account management</PanelRow>
      </>
    ),
  },
};

// German at a report tile's width: the verb does not fit beside the title,
// so it drops under it and the title keeps every word. `lang` lets a compound
// break at a syllable.
export const GermanTitleBesideAction: Story = {
  decorators: [
    (Story) => (
      <div lang="de" style={{ maxWidth: 300 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    title: "Zeit in der aktuellen Phase",
    titleAction: <Button variant="link">Datensätze anzeigen</Button>,
    children: (
      <PanelBody>
        <p>Offene Pipeline heute</p>
      </PanelBody>
    ),
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const verb = await canvas.findByRole("button", {
      name: "Datensätze anzeigen",
    });
    const title = canvas.getByRole("heading", {
      name: "Zeit in der aktuellen Phase",
    });
    await expect(title.scrollWidth).toBeLessThanOrEqual(title.clientWidth);
    await expect(verb.getBoundingClientRect().top).toBeGreaterThanOrEqual(
      title.getBoundingClientRect().bottom,
    );
  },
};

// At 390px the verb drops under a title it would cut. A title wider than the
// pane wraps.
export const PhoneVerbUnderTitle: Story = {
  tags: ["uat-phone"],
  args: {
    title: "Connected mailboxes and calendars",
    titleAction: <Button>Add connector</Button>,
    children: (
      <>
        <PanelRow>gmail.com</PanelRow>
        <PanelRow>outlook.com</PanelRow>
      </>
    ),
  },
};

const SETS = ["Pricing playbook", "Security answers"];

function OpeningRows() {
  const [opened, setOpened] = useState("nothing yet");
  return (
    <Panel title="Document sets">
      {SETS.map((name) => (
        <PanelRow key={name} record onOpen={() => setOpened(name)}>
          <span style={{ flex: 1 }}>{name}</span>
          <OverflowMenu label={`Actions for ${name}`}>
            <Button>Archive</Button>
          </OverflowMenu>
          <RowOpen label={`Open ${name}`} onOpen={() => setOpened(name)} />
        </PanelRow>
      ))}
      <PanelBody>
        <p>Opened: {opened}</p>
      </PanelBody>
    </Panel>
  );
}

// `onOpen` makes the whole row a pointer target; the chevron is the keyboard
// path, and a press on the row's menu stays the menu's.
export const RowOpensOnPress: Story = {
  render: () => <OpeningRows />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: "Open Security answers" });
    await userEvent.click(canvas.getByText("Pricing playbook"));
    await expect(canvas.getByText("Opened: Pricing playbook")).toBeVisible();
    await userEvent.click(
      canvas.getByRole("button", { name: "Actions for Security answers" }),
    );
    await expect(canvas.getByText("Opened: Pricing playbook")).toBeVisible();
    await userEvent.keyboard("{Escape}");
    canvas.getByRole("button", { name: "Open Security answers" }).focus();
    await userEvent.keyboard("{Enter}");
    await expect(canvas.getByText("Opened: Security answers")).toBeVisible();
  },
};

export const Untitled: Story = {
  args: {
    children: (
      <PanelBody>
        <p>A panel with no header at all — the body alone.</p>
      </PanelBody>
    ),
  },
};

// tone="accent": the ONE card on a page that asks for a move rather than
// reporting state. Put two of these on a page and you have no lead.
export const AccentTone: Story = {
  args: {
    tone: "accent",
    title: "Worth doing next",
    children: (
      <>
        <PanelRow>Write to Anna Brandt — nobody has since March.</PanelRow>
        <PanelRow>The renewal closes in eleven days and has no owner.</PanelRow>
      </>
    ),
    footer: <span>Two moves, both yours</span>,
  },
};

// tone="warning": the same lead, when the FINDING is the bad news — a
// relationship that went quiet, a promise that is late. The tone is the
// reading, which is why it recolours the card's own frame instead of adding a
// badge inside it, and why the geometry does not move: an accent lead and a
// warning lead are one card in two states, not two cards.
//
// Check it in both themes. `--warning` inverts across them (a dark amber-brown on
// light, a bright amber on dark) while `--warningBg` and `--warningBorder` stay the
// same translucent amber, so the title's contrast against the band is the one
// thing to look at twice.
export const WarnTone: Story = {
  args: {
    tone: "warning",
    title: "Gone quiet",
    children: (
      <>
        <PanelRow>Nobody has written to Anna Brandt since 4 March.</PanelRow>
        <PanelRow>She asked for pricing and never got it.</PanelRow>
      </>
    ),
    footer: <span>Two sources · updated an hour ago</span>,
  },
};

// The rest of the state vocabulary, on one page: `info` for the lead whose work
// is still running, `success` for the one whose finding is that it landed,
// `danger` for the finding that cannot be undone, `discovery` for a capability
// this reader has not met. Same geometry at every tone — put them beside
// `AccentTone` and only the band's colour moves.
export const StateTones: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-4)" }}>
      <Panel tone="info" title="Still reading">
        <PanelRow>Four of eleven pages read. Nothing to do yet.</PanelRow>
      </Panel>
      <Panel tone="success" title="The renewal is signed">
        <PanelRow>Countersigned this morning, filed against the deal.</PanelRow>
      </Panel>
      <Panel tone="danger" title="The mailbox stopped">
        <PanelRow>No message has been read since 04:12. Reconnect it.</PanelRow>
      </Panel>
      <Panel tone="discovery" title="Deal Rooms are new here">
        <PanelRow>
          A buyer can read the price and the plan in one place.
        </PanelRow>
      </Panel>
    </div>
  ),
};

// tone="ai": the panel a MACHINE wrote or read. It is the same lead geometry
// again, in the --ai family, and it says something different from the other
// two: not "this is the ask" or "this is the bad news" but "Margince produced
// this". That is why it is not a third accent — indigo carries one meaning
// across the whole product, so a panel takes it only when a machine really did
// write or read what is inside.
//
// The two halves of the indigo vocabulary in one head, because they answer
// different questions and a surface usually needs both: `tone="ai"` says a
// machine wrote the BODY, and the `Badge tone="ai"` in the band says this panel
// offers an AI verb — the "AI-assisted" tag the assistant surface carries, in
// the header rather than beside the rows, so a reader finds it in the same place
// on every such panel. The verb itself is `aiQuiet` and not `ai`: on ground the
// family has already tinted, a filled indigo control reads as the panel's call
// to action when it is its footnote.
//
// Check it in both themes. --aiText lifts on dark (a pale indigo) while
// --aiLight and --aiMed stay the same translucent indigo, so the title's
// contrast against the band — and the badge's against the same band — is the
// thing to look at twice.
export const AiTone: Story = {
  args: {
    tone: "ai",
    title: "Brandt Automotive GmbH · 360",
    titleAction: <Badge tone="ai">AI-assisted</Badge>,
    actions: <Button variant="aiQuiet">Read it again</Button>,
    children: (
      <>
        <PanelRow>
          It is your move. Akeneo wrote last on 5 August — 16 days ago.
        </PanelRow>
        <PanelRow>Three invoices are past due, the oldest by 18 days.</PanelRow>
      </>
    ),
    footer: <span>Read from 41 records · 14:22</span>,
  },
};

// A tone TINTS, and nothing else: put this beside `WithTitleAction` and the two
// bands are the same height and the two titles the same size. What differs is
// the ground, the hairline's colour and the title's colour — the indigo head
// used to hug its own content and so stood at a height of its own on a page of
// panels.
export const AiToneRowsOnly: Story = {
  args: {
    tone: "ai",
    title: "What changed on this account",
    titleAction: <Badge tone="ai">AI-assisted</Badge>,
    children: (
      <>
        <PanelRow>Two contacts left the buying group in July.</PanelRow>
        <PanelRow>The renewal date moved forward by three weeks.</PanelRow>
      </>
    ),
  },
};

// actions: verbs that CHANGE the panel, in their own band under the body. The
// footer reports; this acts. A caller renders it only when the content is
// real — an "add" button under a section whose read failed offers a write
// nobody can say makes sense.
export const WithActions: Story = {
  args: {
    title: "Deals",
    children: (
      <>
        <PanelRow>Renewal — €48,000</PanelRow>
        <PanelRow>Expansion, EU — €12,500</PanelRow>
      </>
    ),
    actions: <Button>Add a deal</Button>,
  },
};

// PanelPlate: the recessed plate that separates what IS from what to DO. The
// context sits on the plate, the moves run full-bleed on the panel's own
// ground, and a reader can tell the two apart before reading a word of either.
export const WithPlate: Story = {
  args: {
    tone: "accent",
    title: "Today",
    children: (
      <>
        <PanelPlate>
          <p>Their move — Anna replied on Tuesday and is waiting on pricing.</p>
        </PanelPlate>
        <PanelRow>Send the revised quote.</PanelRow>
        <PanelRow>Book the technical review.</PanelRow>
      </>
    ),
  },
};

// Two groups under one head: each named by a PanelGroupHead one level in from
// the pane's title, its verb on the same line, and an empty group as a plate
// rather than a sentence. The verb keeps its place whether the group has rows
// or not.
export const WithGroups: Story = {
  args: {
    title: "Commercial",
    children: (
      <>
        <PanelGroupHead
          title="Deals"
          level="h3"
          action={<Button variant="ghost">New deal</Button>}
        />
        <PanelRow>Fleet retrofit 2026 · €48,000 · Proposal</PanelRow>
        <PanelGroupHead
          title="Projects"
          level="h3"
          action={<Button variant="ghost">Attach project</Button>}
        />
        <PanelBody>
          <EmptyState plate title="No projects yet">
            A project is the body of work a deal is about.
          </EmptyState>
        </PanelBody>
      </>
    ),
  },
};

// What `interactive` promises: ONE press target filling the row, so pointing
// anywhere in it aims at the same thing. Inline here rather than as a class,
// because a story is not the place a screen's button styling comes from.
const fullRowPress = {
  display: "block",
  width: "100%",
  padding: 0,
  border: 0,
  background: "transparent",
  font: "inherit",
  color: "inherit",
  textAlign: "left",
  cursor: "pointer",
} as const;

// interactive, both states in one panel — the point is the COMPARISON, because
// the defect this prop fixes is only visible as one: run the pointer down the
// column and the two rows that light up are the two you can press, while the
// three that report a reading stay put. Held together as one hover on every
// row, a panel of readings claimed all five were targets.
//
// Check it in both themes: `--groundHover` sits a different distance from the
// row's ground in each.
export const InteractiveRows: Story = {
  args: {
    title: "Quarterly target",
    children: (
      <>
        <PanelRow interactive>
          <button type="button" style={fullRowPress}>
            Q3 — new business, EU
          </button>
        </PanelRow>
        <PanelRow interactive>
          <button type="button" style={fullRowPress}>
            Q4 — renewals
          </button>
        </PanelRow>
        <PanelRow>Renewal date — in 68 days</PanelRow>
        <PanelRow>Set by Lars Ohlsen on 4 March</PanelRow>
        <PanelRow>Weighted against the open pipeline</PanelRow>
      </>
    ),
  },
};

// `record`: one row per thing, so its rule reaches the pane's edges and it
// takes a table row's hover whether or not it is pressable.
export const RecordRows: Story = {
  args: {
    title: "Model tiers",
    children: (
      <>
        <PanelBody>
          <PanelIntro>Which model answers at each depth of work.</PanelIntro>
        </PanelBody>
        <PanelRow record>Fast — triage and short drafts</PanelRow>
        <PanelRow record>Balanced — replies and summaries</PanelRow>
        <PanelRow record>Deep — account reviews</PanelRow>
      </>
    ),
  },
};

type Member = Readonly<{ name: string; role: string; seen: string }>;

const MEMBERS: Member[] = [
  { name: "Lena Fischer", role: "Administrator", seen: "Today" },
  { name: "Bernd Kral", role: "Sales rep", seen: "Yesterday" },
];

function textX(element: Element): number {
  const range = document.createRange();
  range.selectNodeContents(element);
  return range.getBoundingClientRect().left;
}

// The three lists that bleed, each under an intro: the first row's text stands
// at the intro's x while its row spans the pane.
export const BleedListsLineUp: StoryObj = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-4)" }}>
      <Panel title="Members">
        <PanelBody>
          <PanelIntro>Everyone who can sign in to this workspace.</PanelIntro>
        </PanelBody>
        <DataTable
          bleed
          label="Members"
          columns={[
            { key: "name", header: "Name", render: (row: Member) => row.name },
            { key: "role", header: "Role", render: (row: Member) => row.role },
            {
              key: "seen",
              header: "Last seen",
              render: (row: Member) => row.seen,
            },
          ]}
          rows={MEMBERS}
          rowKey={(row) => row.name}
        />
        <PanelBody>
          <PanelIntro>An invitation expires after seven days.</PanelIntro>
        </PanelBody>
      </Panel>
      <Panel title="Roles">
        <PanelBody>
          <PanelIntro>What each role may read and change.</PanelIntro>
        </PanelBody>
        <SettingList bleed="records">
          <SettingRow
            label="Administrator"
            description="Every setting and every record."
            control={<Button variant="ghost">Edit</Button>}
          />
          <SettingRow
            label="Sales rep"
            description="Their own deals and the accounts on them."
            control={<Button variant="ghost">Edit</Button>}
          />
        </SettingList>
      </Panel>
      <Panel title="Model tiers">
        <PanelBody>
          <PanelIntro>Which model answers at each depth of work.</PanelIntro>
        </PanelBody>
        <PanelRow record>Fast — triage and short drafts</PanelRow>
        <PanelRow record>Deep — account reviews</PanelRow>
      </Panel>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Everyone who can sign in to this workspace.");
    const panels = [...canvasElement.querySelectorAll(".panel")];
    expect(panels).toHaveLength(3);
    for (const panel of panels) {
      const intro = panel.querySelector(".panel-intro");
      const row = panel.querySelector(
        "tbody tr, .settinglist > .settingrow, .panel-row",
      );
      const text = panel.querySelector(
        "tbody td, .settingrow .t-name, .panel-row",
      );
      if (!intro || !row || !text) throw new Error("a bleed panel lost a part");
      expect(textX(text)).toBeCloseTo(textX(intro), 0);
      expect(row.getBoundingClientRect().width).toBeCloseTo(
        panel.clientWidth,
        0,
      );
    }
  },
};

// A panel inside a dialog. The dialog's own title is the h2 these sit under, so
// the panel takes an h3 and the outline reads as one nesting rather than two
// unrelated top-level headings.
export const TitleInsideADialog: Story = {
  args: {
    title: "Goal for this meeting",
    titleLevel: 3,
    tone: "accent",
    children: (
      <PanelBody>
        Agree the pilot scope so the retrofit quote can be issued this month.
      </PanelBody>
    ),
  },
};

// The states RailPanel accepts. `stale` and `partial` are absent from RailPanel
// itself — SurfaceState renders each of them as a caveat BESIDE the rows, and a
// RailPanel shows rows on `ready` alone, so a stale one was a caveat over an
// empty card. They are catalogued on SurfaceState, which is the component that
// can render them.
const RAIL_PANEL_STATES: readonly RailPanelState[] = [
  "ready",
  "empty",
  "withheld",
  "unavailable",
  "loading",
  "failed",
];

// Every field at once, and each state reads exactly one of them: `failed` the
// retry, `stale` the as-of, `partial` the count still missing, `withheld` its
// sentence. A state whose field is absent falls back to the generic line,
// which is the floor rather than the target.
const DETAIL: SectionDetail = {
  onRetry: () => undefined,
  staleAsOf: "9:15 this morning",
  remaining: 4,
  withheldReason:
    "Deal amounts on this account are readable by its owner and by finance.",
};

// A Disclosure standing in the panel directly, as one row among rows: its
// summary sits on the pane's gutter at the row's interval, with no second
// hairline under the header band, and the rows it opens run edge to edge like
// the ones above it. Set free-standing the atom drew its own rule and inset,
// and the caret stood hard against the card's edge.
export const WithDisclosure: Story = {
  render: () => (
    <Panel title="Overnight">
      <Disclosure summary="Sync details">
        <PanelRow>3 messages synced</PanelRow>
        <PanelRow>1 contact created</PanelRow>
      </Disclosure>
    </Panel>
  ),
};

// RailPanel on `ready`: the children are handed to Panel undecorated, so rows
// run edge to edge under the header band instead of sitting inside a padded
// body. That full-bleed shape is the reason RailPanel exists rather than being
// a Panel the caller wraps by hand.
export const RailPanelReady: StoryObj = {
  render: () => (
    <RailPanel
      title="Who is on this deal"
      state="ready"
      emptyLabel="Nobody is named on this deal yet."
      footer={<p className="t-caption">3 named, 1 unreachable</p>}
    >
      <PanelRow>Lena Fischer — decision maker</PanelRow>
      <PanelRow>Frédéric de Gombert — economic buyer</PanelRow>
      <PanelRow>Bernd Kral — technical evaluator</PanelRow>
    </RailPanel>
  ),
};

// Everything that is not `ready`: the message is padded inside a PanelBody,
// and the card-level figure is shown only where there is one to report. A
// withheld or unavailable section has no figure — printing the last one it had
// would attach a count to rows the reader is not seeing.
//
// Drawn together on purpose. "There is nothing here" and "you may not read
// this" make the SAME shape on screen and mean opposite things, and the only
// way to judge them is side by side. The footer is passed to every one of
// them, which is what makes this story able to fail: a panel that stopped
// withholding it would look perfectly reasonable in isolation.
export const RailPanelMessageStates: StoryObj = {
  decorators: [
    (Story) => (
      <div
        style={{
          display: "grid",
          gap: "var(--space-4)",
          gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
        }}
      >
        <Story />
      </div>
    ),
  ],
  render: () => (
    <>
      {RAIL_PANEL_STATES.filter((state) => state !== "ready").map((state) => (
        <RailPanel
          key={state}
          title={`Who is on this deal — ${state}`}
          state={state}
          emptyLabel="Nobody is named on this deal yet."
          detail={DETAIL}
          footer={<p className="t-caption">3 named, 1 unreachable</p>}
        >
          <PanelRow>Lena Fischer — decision maker</PanelRow>
        </RailPanel>
      ))}
    </>
  ),
};

type DaySpend = Readonly<{ task: string; calls: string }>;

const SPEND: DaySpend[] = [
  { task: "Triage", calls: "1,204" },
  { task: "Drafts", calls: "312" },
];

const spendColumns = [
  { key: "task", header: "Task", render: (row: DaySpend) => row.task },
  {
    key: "calls",
    header: "Calls",
    align: "end" as const,
    render: (row: DaySpend) => row.calls,
  },
];

function SyncRows() {
  return (
    <SettingList bleed="settings">
      <SettingRow
        label="Mail sync"
        control={<Badge tone="success">On</Badge>}
      />
      <SettingRow label="History window" value="30 days" control={null} />
    </SettingList>
  );
}

// Every way a settings pane stacks its bands. Each boundary draws one line,
// edge to edge like the head's, or none; `panelseams.test.tsx` holds the count.
function BandsAndSeamsPanes() {
  return (
    <div style={{ display: "grid", gap: "var(--space-4)" }}>
      <Panel title="Body after body">
        <PanelBody>
          <PanelIntro>
            Messages from these domains are never captured.
          </PanelIntro>
        </PanelBody>
        <PanelBody>Three domains, added by two members.</PanelBody>
        <PanelBody>
          <Disclosure summary="Change history">
            <p>Renamed on Monday, added on Friday.</p>
          </Disclosure>
        </PanelBody>
      </Panel>
      <Panel title="Lists and bodies">
        <SyncRows />
        <PanelBody>
          <p className="t-caption">Sync runs every five minutes.</p>
        </PanelBody>
        <SyncRows />
      </Panel>
      <Panel title="Spend">
        <PanelGroupHead title="Spend by task" level="h3" />
        <DataTable
          bleed
          label="Spend by task"
          columns={spendColumns}
          rows={SPEND}
          rowKey={(row) => row.task}
        />
        <PanelBody>
          <Disclosure summary="Show days" open>
            <p>Twelve calls on Monday, four on Tuesday.</p>
          </Disclosure>
        </PanelBody>
        <PanelGroupHead title="Spend by model" level="h3" />
        <PanelBody>One model answered every call this month.</PanelBody>
      </Panel>
      <Panel title="Company">
        <SettingList bleed="settings">
          <SettingRow label="Name" value="Acme Pumps" control={null} />
          <Disclosure summary="Market" open>
            <SettingList>
              <SettingRow
                label="Sells"
                value="Industrial pumps"
                control={null}
              />
              <SettingRow label="Buyers" value="Water plants" control={null} />
            </SettingList>
          </Disclosure>
        </SettingList>
        <PanelRow>Rows run after the list</PanelRow>
        <PanelRow>with their own inset rule between them</PanelRow>
        <PanelBody>And a body closes the pane.</PanelBody>
      </Panel>
    </div>
  );
}

async function groupHeadsReadUnderTheTitle(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const group = await canvas.findByRole("heading", { name: "Spend by task" });
  const title = canvas.getByRole("heading", { name: "Spend" });
  expect(group.tagName).toBe("H3");
  expect(Number.parseFloat(getComputedStyle(group).fontSize)).toBeLessThan(
    Number.parseFloat(getComputedStyle(title).fontSize),
  );
}

export const BandsAndSeams: StoryObj = {
  render: () => <BandsAndSeamsPanes />,
  play: ({ canvasElement }) => groupHeadsReadUnderTheTitle(canvasElement),
};

export const BandsAndSeamsDark: StoryObj = {
  globals: { theme: "dark" },
  render: () => <BandsAndSeamsPanes />,
  play: ({ canvasElement }) => groupHeadsReadUnderTheTitle(canvasElement),
};

export const BandsAndSeamsPhone: StoryObj = {
  tags: ["uat-phone"],
  render: () => <BandsAndSeamsPanes />,
  play: ({ canvasElement }) => groupHeadsReadUnderTheTitle(canvasElement),
};

/** The head as one line with no band: the small title beside its action and
 *  no rule under them. A card under a verdict reads as a list, not a zone. */
export const FlatHead: Story = {
  render: () => (
    <Panel
      flatHead
      title="Also on your list"
      titleAction={<Button variant="link">All tasks</Button>}
    >
      <PanelBody>Rows sit inside the card's own padding.</PanelBody>
    </Panel>
  ),
  play: async ({ canvasElement }) => {
    const region = within(canvasElement).getByRole("region", {
      name: "Also on your list",
    });
    await expect(region.querySelector(".panel-head-flat")).not.toBeNull();
  },
};
