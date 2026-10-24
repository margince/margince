// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  TaskCompleteCheck,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import { en } from "../../i18n/en";
import { taskWriteKeys } from "../../lib/activitykeys";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { TaskDetailModal } from "./taskdetailmodal";

// Acting on a task from the record it belongs to: the tick, the quick-action
// row, and the detail modal a next-step row opens into. Mirrors company360.test.tsx's
// NextStepsWithVerbs pairing — the verbs take a live `useTaskUpdate` mutation
// rather than a stub function, so a story exercises the same wiring the record
// page does.

const meta: Meta = {
  title: "Patterns/Task actions",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The mutation is raised INSIDE StoryProviders, not in the component that
// renders it: useTaskUpdate reads the query client from context, so a caller
// that mounts the provider in its own return value is still outside it when
// the hook runs. Every story here splits along that line.
function TaskRowBody({ dueAt }: Readonly<{ dueAt?: string | null }>) {
  const update = useTaskUpdate(taskWriteKeys("company", "o-1"));
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "var(--space-2)",
      }}
    >
      <TaskCompleteCheck activityId="a-1" version={3} update={update} />
      <span>Send the renewal paperwork</span>
      <TaskQuickActions
        activityId="a-1"
        version={3}
        dueAt={dueAt}
        update={update}
        showComplete={false}
      />
    </div>
  );
}

function TaskRow({ dueAt }: Readonly<{ dueAt?: string | null }>) {
  installFetchStub({});
  return (
    <StoryProviders>
      <TaskRowBody dueAt={dueAt} />
    </StoryProviders>
  );
}

// Snooze is offered only for a dated task — there is no day to move an
// undated one to.
export const RowWithDueDate: Story = {
  render: () => <TaskRow dueAt="2026-08-01T09:00:00Z" />,
};

export const RowUndated: Story = { render: () => <TaskRow dueAt={null} /> };

const TRANSCRIPT = Array.from({ length: 40 }, (_, turn) =>
  turn % 2 === 0
    ? `Dana: Legal wants the renewal terms by Friday, round ${turn / 2 + 1}.`
    : "Mira: I will send the paperwork once the discount is signed off.",
).join("\n");

function sourceOf({
  emailRequest,
  meetingSource,
  contactTask,
}: Readonly<{
  emailRequest: boolean;
  meetingSource: boolean;
  contactTask?: "call" | "typed";
}>) {
  if (emailRequest) return "email-1";
  if (meetingSource) return "m-1";
  return contactTask === "call" ? "call-1" : null;
}

// The account first on the wire; the panel names the deal first.
function linksOf(filed: boolean, contactTask?: "call" | "typed") {
  if (filed) {
    return [
      { entity_type: "company", entity_id: "o-1" },
      { entity_type: "deal", entity_id: "d-1" },
    ];
  }
  return contactTask ? [{ entity_type: "contact", entity_id: "c-1" }] : [];
}

function DetailModal({
  emailRequest = false,
  meetingSource = false,
  filed = false,
  contactTask,
}: Readonly<{
  emailRequest?: boolean;
  meetingSource?: boolean;
  filed?: boolean;
  /** A task owed to one contact, read out of a call or typed by hand. */
  contactTask?: "call" | "typed";
}>) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/email-1": () =>
      jsonResponse({
        id: "email-1",
        kind: "email",
        subject: "Renewal paperwork",
        occurred_at: "2026-07-27T14:00:00Z",
        content_state: "available",
      }),
    "GET /activities/email-1/email-presentation": () =>
      jsonResponse({
        id: "email-1",
        lifecycle: "delivered",
        version: 1,
        occurred_at: "2026-07-27T14:00:00Z",
        summary: {
          activity_id: "email-1",
          occurred_at: "2026-07-27T14:00:00Z",
          version: 1,
          subject: "Renewal paperwork",
          display_status: "team",
          move: "needs_reply",
          attachment_count: 0,
        },
        body: "Please send the agreed renewal paperwork.",
        from: [],
        to: [],
        cc: [],
        bcc: [],
        bcc_withheld: false,
        attachments: [],
        links: [],
        thread: { members: [], next_cursor: null },
        can_reply: false,
        can_relink: false,
        access: {
          content_state: "available",
          display_status: "team",
          audience: "workspace",
          can_change: false,
          change_mode: "none",
        },
      }),
    "GET /activities/m-1": () =>
      jsonResponse({
        id: "m-1",
        kind: "meeting",
        subject: "Renewal call with Dana",
        body: TRANSCRIPT,
        occurred_at: "2026-07-27T14:00:00Z",
      }),
    "GET /activities/a-1": () =>
      jsonResponse({
        id: "a-1",
        kind: "task",
        subject: "Send the renewal paperwork",
        body: emailRequest
          ? null
          : "Draft went to legal on Tuesday; needs Dana's sign-off.",
        source_activity_id: sourceOf({
          emailRequest,
          meetingSource,
          contactTask,
        }),
        due_at: emailRequest ? null : "2026-08-01T09:00:00Z",
        occurred_at: "2026-07-28T09:00:00Z",
        is_done: false,
        assignee_id: "u-1",
        links: linksOf(filed, contactTask),
      }),
    "GET /activities/call-1": () =>
      jsonResponse({
        id: "call-1",
        kind: "call",
        subject: "Voicemail for Dana",
        occurred_at: "2026-07-27T14:00:00Z",
        content_state: "available",
      }),
    "GET /contacts/c-1": () =>
      jsonResponse({
        id: "c-1",
        full_name: "Dana Brandt",
        primary_email: "dana@brandt.example",
        phones: [{ id: "ph-1", phone: "+491517654321", is_primary: true }],
      }),
    "GET /deals/d-1": () =>
      jsonResponse({ id: "d-1", name: "Brandt renewal 2027" }),
    "GET /companies/o-1": () =>
      jsonResponse({ id: "o-1", display_name: "Brandt Automotive" }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-1", display_name: "Mira Voss" }],
        page: { has_more: false, next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <DetailModalBody />
    </StoryProviders>
  );
}

function DetailModalBody() {
  const update = useTaskUpdate(taskWriteKeys("company", "o-1"));
  return (
    <TaskDetailModal
      activityId="a-1"
      readOnly={false}
      onClose={() => {}}
      update={update}
    />
  );
}

export const DetailOpen: Story = { render: () => <DetailModal /> };

// Filed under a deal and its account: the deal, the more specific, leads.
export const FiledUnderDeal: Story = {
  render: () => <DetailModal filed />,
};

// A task read out of a call: Call, Email and Open contact above its own verbs.
export const DetailFromCall: Story = {
  render: () => <DetailModal contactTask="call" />,
  play: async () => {
    const body = within(document.body);
    await body.findByRole("link", { name: en["record.verb.call"] });
  },
};

// The same contact on a task typed by hand: no Call, since nothing says it is one.
export const DetailTypedByHand: Story = {
  render: () => <DetailModal contactTask="typed" />,
  play: async () => {
    const body = within(document.body);
    await body.findByRole("link", { name: en["task.next.openContact"] });
  },
};

export const CapturedEmailRequest: Story = {
  render: () => <DetailModal emailRequest />,
};

// The meeting a task was read out of, opened whole over the task.
export const SourceTranscriptOpen: Story = {
  render: () => <DetailModal meetingSource />,
  play: async () => {
    const body = within(document.body);
    await userEvent.click(
      await body.findByRole("button", { name: en["tasks.openSource"] }),
    );
    await body.findByRole("dialog", { name: "Renewal call with Dana" });
  },
};
