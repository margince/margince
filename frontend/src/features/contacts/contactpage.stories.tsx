// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import type { ContactTab } from "../../lib/contacttab";
import { ContactDealsTab } from "./contactdeals";
import { ContactMeetingsTab } from "./contactmeetings";
import { ContactPageV2 } from "./contactpage";
import { providerCompletedProfile } from "./contactprovider.fixtures";
import { ContactTimelineTab } from "./contacttabs";
import { ContactToday } from "./contacttoday";
import "./contact360.css";
import {
  callMessengersRoute,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  chatConversation,
  meetingPrepMoment,
  page,
  populated,
  type View,
  withheld,
} from "./contactpage.fixtures";

// The contact record page V2 (ADR-0096): the whole page behind its three reads,
// the lead moment in both tints, and the tabs. The rail, the overview cards and
// the research drawer have their own galleries in the contactpage.*.stories
// files beside this one.
//
// These galleries show what the live stack cannot. Every seeded demo contact
// carries full RBAC and a clean sections_omitted, so a reader never sees a
// withheld reading. That state exists only here.

const meta: Meta = {
  title: "Records/Contact 360/Page",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The same record with a moment on the amber ladder rung: a relationship that
// stopped rather than one that is merely upcoming, so both tints of the lead
// card are on screen across the gallery.
const goneQuietMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "gone_quiet:p-1",
  evidence_fingerprint: "fp-quiet-1",
  rule: "gone_quiet",
  rule_version: "v1",
  headline: "Dana has gone quiet for 18 days.",
  why_now:
    "No reply in 18 days after two outbound messages — the gone-quiet rung fired ahead of meeting prep.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-1",
      label: "Re: retrofit timeline",
      snippet: "Can we push the fleet retrofit review back a week?",
      observed_at: "2026-08-01T10:15:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Send a check-in",
    state: "available",
  },
  secondary_actions: [
    {
      kind: "ask_colleague",
      label: "Ask Sam Rivera",
      destination: { surface: "record", entity_id: "u-2" },
      state: "available",
    },
  ],
};

// The seven rungs of the ladder LeadMoment/LeadMomentWarning above never
// reach: ContactToday renders one ContactMomentRule per fixture, and a gallery
// with only meeting_prep and gone_quiet on screen hides the other seven the
// component can render. Spread across them: two evidence items (the
// "sources" plural), a `will_confirm` action, a `blocked` one with its
// reason, and a freshness older than the moment's own headline date.

const reEngagedMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "re_engaged:p-1",
  evidence_fingerprint: "fp-reengaged-1",
  rule: "re_engaged",
  rule_version: "v1",
  headline: "Dana wrote back after six weeks quiet.",
  why_now: "A reply landed after a long silence, the door is open again.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-3",
      label: "Re: still interested in the retrofit",
      observed_at: "2026-08-13T08:00:00Z",
    },
    {
      type: "activity",
      id: "a-1",
      label: "Re: retrofit timeline",
      observed_at: "2026-08-01T10:15:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Welcome her back",
    state: "will_confirm",
  },
  secondary_actions: [
    { kind: "schedule_meeting", label: "Book a catch-up", state: "available" },
  ],
};

const jobChangeMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "job_change:p-1",
  evidence_fingerprint: "fp-jobchange-1",
  rule: "job_change",
  rule_version: "v1",
  headline: "Dana moved to Head of Fleet at Brandt Automotive.",
  why_now: "A recorded employment change: the buying context here just moved.",
  confidence: "observed_fact",
  // Older than the headline's own evidence, so the "updated N days ago" wording
  // reads as a real gap rather than same-day freshness.
  freshness_at: "2026-08-05T09:00:00Z",
  evidence: [
    {
      type: "relationship_change",
      label: "Employment updated: Head of Fleet at Brandt Automotive GmbH",
      observed_at: "2026-08-05T09:00:00Z",
    },
  ],
  recommended_action: {
    kind: "open_record",
    label: "Review the account",
    destination: { surface: "record", entity_type: "company" },
    state: "available",
  },
  secondary_actions: [
    {
      kind: "draft_reply",
      label: "Congratulate her",
      state: "blocked",
      blocked_reason: "No consent recorded for marketing outreach yet.",
    },
  ],
};

const overduePromiseMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "overdue_promise:p-1:c-1",
  evidence_fingerprint: "fp-overdue-1",
  rule: "overdue_promise",
  rule_version: "v1",
  headline: "The updated retrofit quote is three days late.",
  why_now: "A commitment past its own due date, still open.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-1",
      label: "Re: retrofit timeline",
      snippet: "Can we push the fleet retrofit review back a week?",
      observed_at: "2026-08-01T10:15:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Send the quote",
    state: "available",
  },
};

const publicSignalMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "public_signal:p-1",
  evidence_fingerprint: "fp-publicsignal-1",
  rule: "public_signal",
  rule_version: "v1",
  headline: "Dana posted about the depot's EV rollout timeline.",
  why_now: "A public statement that bears on the retrofit conversation.",
  confidence: "medium",
  freshness_at: "2026-08-11T09:00:00Z",
  evidence: [
    {
      type: "activity",
      label: 'LinkedIn post: "2027 is the depot\'s EV deadline"',
      observed_at: "2026-08-11T09:00:00Z",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Reference it in your next note",
    state: "available",
  },
};

const missingNextStepMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "missing_next_step:p-1",
  evidence_fingerprint: "fp-missingstep-1",
  rule: "missing_next_step",
  rule_version: "v1",
  headline: "The retrofit walkthrough has no next step booked after it.",
  why_now: "A meeting is on the calendar with nothing recorded for after it.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "task",
      label: "Fleet retrofit walkthrough, 20 Aug",
      observed_at: "2026-08-20T13:00:00Z",
    },
  ],
  recommended_action: {
    kind: "schedule_meeting",
    label: "Book the follow-up",
    state: "available",
  },
};

const thinRelationshipMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "thin_relationship:p-1",
  evidence_fingerprint: "fp-empty",
  rule: "thin_relationship",
  rule_version: "v1",
  headline: "No interactions recorded",
  why_now: "No interactions or colleagues found in the records you can see.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "log_activity",
    label: "Log an interaction",
    state: "available",
  },
};

// Rung 10, the quiet-success case: nothing needs the reader today, and it
// renders through this same component rather than an empty card
const nothingNeededMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "nothing_needed:p-1",
  evidence_fingerprint: "fp-nothingneeded-1",
  rule: "nothing_needed",
  rule_version: "v1",
  headline: "Nothing needs you on this account today.",
  why_now: "Every open loop is answered and the next meeting is booked.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [
    {
      type: "activity",
      id: "a-2",
      label: "Fleet retrofit walkthrough, 20 Aug",
      observed_at: "2026-08-20T13:00:00Z",
    },
  ],
  recommended_action: {
    kind: "open_record",
    label: "Open record",
    destination: {
      surface: "record",
      entity_type: "contact",
      entity_id: "p-1",
    },
    state: "available",
  },
};

// --- Page: the whole ContactPageV2 behind its three reads --------------------

// The guard as the page reads it: one purpose that permits mail, and a phone
// purpose nobody has decided. `allowed` for any email purpose is what makes the
// lead verb pressable.
const guardAllowsMail: components["schemas"]["ContactConsentGuardEntry"][] = [
  {
    purpose_key: "business_correspondence",
    purpose_label: "Business correspondence",
    purpose_class: "business_correspondence",
    channel: "email",
    verdict: "allowed",
    reason: "She wrote to you on 1 Aug 2026.",
    qualifying_event: {
      kind: "inbound_message",
      occurred_at: "2026-08-01T10:15:00Z",
      source_entity_type: "activity",
      source_entity_id: "a-1",
    },
  },
  {
    purpose_key: "phone_outreach",
    purpose_label: "Phone outreach",
    purpose_class: "phone_outreach",
    channel: "phone",
    verdict: "unknown",
    reason: "No consent recorded.",
  },
];

// The same guard with the one purpose refused. No seeded contact reaches this
// state either, so the refused lead verb renders only here.
const guardRefusesMail: components["schemas"]["ContactConsentGuardEntry"][] = [
  {
    purpose_key: "marketing",
    purpose_label: "Marketing",
    purpose_class: "marketing",
    channel: "email",
    verdict: "blocked",
    reason: "She opted out on 12 Jul 2026.",
  },
];

// Reachable both ways: the address the record already carries and the channel
// conversation beside it.
const mailAndChat: View = {
  ...populated,
  contact: {
    ...populated.contact,
    reachability: chatConversation.contact.reachability,
  },
  activities: {
    data: [
      ...(populated.activities?.data ?? []),
      ...(chatConversation.activities?.data ?? []),
    ],
    page,
  },
};

// No address, and no channel conversation to answer: the mail thread on the
// record is history, not a way to reach anybody.
const unreachable: View = {
  ...populated,
  contact: { ...populated.contact, emails: [], primary_email: null },
};

function Page({
  view = populated,
  guardEntries = guardAllowsMail,
  tab = "overview",
}: Readonly<{
  view?: View;
  guardEntries?: components["schemas"]["ContactConsentGuardEntry"][];
  tab?: ContactTab;
}>) {
  installFetchStub({
    // The page mounts capability-aware chrome, so the session has to be routed:
    // the stub refuses to guess one, and an unrouted probe fails every grant
    // closed — the embedded rail would render read-only no matter what the
    // fixture below grants, which is not what a permitted reader sees.
    "GET /me": meRoute({ contact: ["read", "update"] }),
    "GET /installation/settings": callMessengersRoute(),
    "GET /contacts/p-1/360": () => jsonResponse(view),
    "GET /contacts/p-1/brief": () =>
      jsonResponse({
        contact_id: "p-1",
        generated_at: "2026-08-13T09:00:00Z",
        generated_by: "deterministic",
        sentences: [
          {
            text: "Dana Buyer leads fleet operations at Brandt Automotive and is the champion on the retrofit work.",
            evidence: [{ entity_type: "contact", entity_id: "p-1" }],
          },
          {
            text: "She asked to push the retrofit review back a week and has not replied since.",
            evidence: [{ entity_type: "activity", entity_id: "a-1" }],
          },
        ],
      }),
    "GET /contacts/p-1/consent/guard": () =>
      jsonResponse({ contact_id: "p-1", entries: guardEntries }),
    // A channel has no name until the directory supplies one, so a page story
    // about a chat-only contact has to serve it: without this the lead verb
    // falls back to the raw provider id, which is the resolver's honest
    // behaviour and not what these stories are about.
    "GET /channel-providers": () =>
      jsonResponse({
        data: [
          {
            provider: "zalo_oa",
            label: "Zalo OA",
            credential_model: "workspace_bot",
            supplies_transport: true,
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <ContactPageV2 id="p-1" tab={tab} />
    </StoryProviders>
  );
}

export const PageStory: Story = { name: "Page", render: () => <Page /> };

// A provider is connected and nobody has looked this contact up, so the tab
// strip carries a dot on "Research". The dot is decorative, since the panel
// behind it says the same thing in words. This story checks whether the
// invitation is visible from a page the reader is already on.
const neverBought: View = {
  ...populated,
  provider_profiles: [
    {
      ...providerCompletedProfile,
      state: "never_run",
      provider: "surfe",
      retrieved_at: null,
      emails: [],
      mobile_phones: [],
      linkedin_url: null,
      current_employment: undefined,
      job_history: [],
      location: null,
      departments: [],
      seniorities: [],
      latest_run: undefined,
      contributing_runs: undefined,
      categories_not_requested: [],
    },
  ],
};

export const PageLookupWaiting: Story = {
  name: "Page · a lookup nobody has run",
  render: () => <Page view={neverBought} />,
};

export const PageDataAndTools: Story = {
  name: "Page · Data & tools tab",
  render: () => <Page view={neverBought} tab="research" />,
};

// The lead verb names the transport the composer would pick, so the header
// reads differently for every shape of reachability — and that is exactly what
// no live stack shows: the seeded demo contacts all have an address.
export const PageChannelOnly: Story = {
  name: "Page · reachable only on a channel",
  render: () => <Page view={chatConversation} />,
};

// Both ways open, so the drawer will ask which. The verb must promise neither:
// a green Email that lands on a picker named one of several transports and then
// asked about it.
export const PageTwoTransports: Story = {
  name: "Page · two transports",
  render: () => <Page view={mailAndChat} />,
};

// Nothing to write to. The consent verdict is `allowed` here on purpose: it is
// decided per purpose and says nothing about whether the record carries a
// transport, which is why the verb needs its own refusal.
export const PageNoTransport: Story = {
  name: "Page · no way to reach them",
  render: () => <Page view={unreachable} />,
};

// The other refusal, on a contact who IS reachable. Two facts, two sentences —
// a rep told the wrong one goes looking in the wrong record.
export const PageConsentRefused: Story = {
  name: "Page · consent refuses",
  render: () => <Page guardEntries={guardRefusesMail} />,
};

// --- Lead moment: ContactToday in both tints ---------------------------------

export const LeadMoment: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <ContactToday
          view={populated}
          moment={meetingPrepMoment}
          onAction={() => {}}
        />
      </div>
    </StoryProviders>
  ),
};

export const LeadMomentWarning: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 720 }}>
        <ContactToday
          view={populated}
          moment={goneQuietMoment}
          onAction={() => {}}
        />
      </div>
    </StoryProviders>
  ),
};

// The seven rungs LeadMoment and LeadMomentWarning above don't reach, stacked
// rather than split into seven near-identical stories: each one differs only
// in its moment, so a designer scanning this gallery sees the whole ladder in
// one scroll instead of hunting seven sidebar entries for the same panel.
const REMAINING_MOMENTS: ReadonlyArray<components["schemas"]["ContactMoment"]> =
  [
    reEngagedMoment,
    jobChangeMoment,
    overduePromiseMoment,
    publicSignalMoment,
    missingNextStepMoment,
    thinRelationshipMoment,
    nothingNeededMoment,
  ];

export const LeadMomentLadder: Story = {
  render: () => (
    <StoryProviders>
      <div
        style={{
          maxWidth: 720,
          display: "flex",
          flexDirection: "column",
          gap: "var(--space-4)",
        }}
      >
        {REMAINING_MOMENTS.map((moment) => (
          <ContactToday
            view={populated}
            key={moment.claim_key}
            moment={moment}
            onAction={() => {}}
          />
        ))}
      </div>
    </StoryProviders>
  ),
};

// The rung the ladder above never reaches, on the shape that makes the panel
// say one thing once: the server writes a promise's headline FROM the task,
// and the same task is on the record's own list. The move names it, the chip
// under the move would name it a second time and the list a third, so the
// card keeps the ask and drops both copies.
const openPromiseMoment: components["schemas"]["ContactMoment"] = {
  claim_key: "open_promise:p-1:a-9",
  evidence_fingerprint: "fp-openpromise-1",
  rule: "open_promise",
  rule_version: "v1",
  headline: "You owe them: send the retrofit quote",
  why_now: "A commitment with a date on it, still open.",
  confidence: "observed_fact",
  freshness_at: "2026-08-13T09:00:00Z",
  evidence: [{ type: "task", id: "a-9", label: "Send the retrofit quote" }],
  recommended_action: {
    kind: "complete_task",
    label: "Open the task",
    state: "available",
    destination: { surface: "task", entity_type: "activity", entity_id: "a-9" },
  },
};

const promised: View = {
  ...populated,
  moment: openPromiseMoment,
  next_steps: {
    data: [
      {
        occurred_precision: "instant",
        id: "a-9",
        kind: "task",
        subject: "Send the retrofit quote",
        occurred_at: "2026-08-10T09:00:00Z",
        due_at: "2026-08-14T09:00:00Z",
        is_done: false,
        source: "manual",
        captured_by: "human:u1",
        created_at: "2026-08-10T09:00:00Z",
        updated_at: "2026-08-10T09:00:00Z",
      },
      {
        occurred_precision: "instant",
        id: "a-8",
        kind: "task",
        subject: "Book the depot walkthrough",
        occurred_at: "2026-08-09T09:00:00Z",
        is_done: false,
        source: "manual",
        captured_by: "human:u1",
        created_at: "2026-08-09T09:00:00Z",
        updated_at: "2026-08-09T09:00:00Z",
      },
    ],
    page,
  },
};

export const LeadMomentPromised: Story = {
  name: "Lead moment · a promise already on the list",
  render: () => {
    // The only lead-moment story with a task row under the move, so it is the
    // only one whose verb reads the session. Routed here because the card is
    // rendered on its own: the stub's list-shaped fallback reads as a
    // malformed session, which closes every grant and draws a refused button
    // instead of the one this story is about.
    installFetchStub({ "GET /me": meRoute({ activity: ["read", "update"] }) });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <ContactToday
            view={promised}
            moment={openPromiseMoment}
            onAction={() => {}}
          />
        </div>
      </StoryProviders>
    );
  },
};

// The next task came out of a call, so its bar offers Call beside Open task.
const calledBack: View = {
  ...populated,
  next_steps: {
    data: [
      {
        occurred_precision: "instant",
        id: "a-7",
        kind: "task",
        subject: "Call Dana back about the depot retrofit",
        occurred_at: "2026-08-10T09:00:00Z",
        due_at: "2026-08-12T09:00:00Z",
        is_done: false,
        source_activity_id: "call-7",
        links: [{ entity_type: "contact", entity_id: "p-1" }],
        source: "manual",
        captured_by: "human:u1",
        created_at: "2026-08-10T09:00:00Z",
        updated_at: "2026-08-10T09:00:00Z",
      },
    ],
    page,
  },
};

export const LeadNextTaskFromCall: Story = {
  name: "Lead moment · next task from a call",
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }),
      "GET /activities/call-7": () =>
        jsonResponse({
          id: "call-7",
          kind: "call",
          content_state: "available",
          occurred_at: "2026-08-09T15:00:00Z",
        }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <ContactToday
            view={calledBack}
            moment={nothingNeededMoment}
            onAction={() => {}}
          />
        </div>
      </StoryProviders>
    );
  },
  play: async () => {
    await screen.findByRole("link", { name: en["record.verb.call"] });
  },
};

// The move is about that same task, so its bar offers Call beside the reply.
const owedCallMoment: components["schemas"]["ContactMoment"] = {
  ...openPromiseMoment,
  claim_key: "open_promise:p-1:a-7",
  headline: "You owe them: Call Dana back about the depot retrofit",
  evidence: [
    {
      type: "task",
      id: "a-7",
      label: "Call Dana back about the depot retrofit",
    },
  ],
  recommended_action: {
    kind: "draft_reply",
    label: "Write to them",
    state: "available",
  },
};

export const LeadMomentFromCall: Story = {
  name: "Lead moment · owed a call back",
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "update"] }),
      "GET /activities/call-7": () =>
        jsonResponse({
          id: "call-7",
          kind: "call",
          content_state: "available",
          occurred_at: "2026-08-09T15:00:00Z",
        }),
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <ContactToday
            view={calledBack}
            moment={owedCallMoment}
            onAction={() => {}}
          />
        </div>
      </StoryProviders>
    );
  },
  play: async () => {
    await screen.findByRole("link", { name: en["record.verb.call"] });
  },
};

// --- Drawers: the surfaces the page opens over itself ------------------------
//
// Each renders open, since a closed drawer paints nothing. Writing to the
// contact is the one composer every record opens: compose.stories.tsx.

const opensDialog = (name: string) => async () => {
  await userEvent.click(await screen.findByRole("button", { name }));
  await screen.findByRole("dialog");
};

export const ConsentDrawer: Story = {
  name: "Page · consent drawer",
  render: () => <Page />,
  play: opensDialog("Manage consent and proof history"),
};

export const DetailsOnPhone: Story = {
  name: "Page · details on a phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <Page />,
  play: opensDialog("Show details and permissions"),
};

// --- The tabs beside Overview -----------------------------------------------
//
// Each reads the overview's 360, permitted and withheld: the withheld half is
// the one a stubbed empty state would misreport as "there is none".

// Tabs read the session for viewer attribution and available actions.
function tabViewer(): void {
  installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
}

export const TabTimeline: Story = {
  render: () => {
    tabViewer();
    return (
      <StoryProviders>
        <ContactTimelineTab contactId="p-1" view={populated} />
      </StoryProviders>
    );
  },
};

export const TabTimelineWithheld: Story = {
  render: () => {
    tabViewer();
    return (
      <StoryProviders>
        <ContactTimelineTab contactId="p-1" view={withheld} />
      </StoryProviders>
    );
  },
};

export const TabDeals: Story = {
  render: () => {
    tabViewer();
    return (
      <StoryProviders>
        <ContactDealsTab view={populated} />
      </StoryProviders>
    );
  },
};

export const TabDealsWithheld: Story = {
  render: () => {
    tabViewer();
    return (
      <StoryProviders>
        <ContactDealsTab view={withheld} />
      </StoryProviders>
    );
  },
};

export const TabMeetings: Story = {
  render: () => {
    tabViewer();
    return (
      <StoryProviders>
        <ContactMeetingsTab view={populated} />
      </StoryProviders>
    );
  },
};

export const TabMeetingsWithheld: Story = {
  render: () => {
    tabViewer();
    return (
      <StoryProviders>
        <ContactMeetingsTab view={withheld} />
      </StoryProviders>
    );
  },
};
