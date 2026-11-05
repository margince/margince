// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ScheduledSendsScreen } from "./scheduledsends";
import { SendRow } from "./scheduledsends.row";

// The queue behind "send later": what a rep has written that has not gone out
// yet, and the two things they can do about it.
//
// It is one contact's own list — an unsent body and its blind-copy list are not
// workspace-readable the way a sent activity is — so there is no owner column
// here and no sharing affordance, and none of these stories is about a role.
//
// The three groups are the reading, not a filter: `held` first because it is the
// only one waiting on a contact, `scheduled` as the queue proper, and everything
// settled below them. A rep with nothing scheduled reads ONE sentence rather
// than three empty blocks, which is `NothingScheduled`.
const meta: Meta = {
  title: "Records/Scheduled messages",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// Every moment is stated in the reader's own zone unless the send names another,
// so the fixtures below carry the browser's — except `crossZone`, which is the
// case the row has to explain rather than silently re-render.
const readerZone = Intl.DateTimeFormat().resolvedOptions().timeZone;

// Moments are built from the clock the story opens on, so the timeline always
// has a today, a tomorrow and a later day to show.
function inDays(days: number, hour: number): string {
  const at = new Date();
  at.setDate(at.getDate() + days);
  at.setHours(hour, 0, 0, 0);
  return at.toISOString();
}

const waiting = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  status: "scheduled",
  scheduled_at: inDays(1, 9),
  scheduled_tz: readerZone,
  subject: "The renewal quote",
  body: "Hi, attached is the renewal quote we discussed on Thursday.\nBest regards",
  to: ["ceo@acme.example"],
  version: 3,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
};

// Stopped at fire, and the row says why in words: `consent_withdrawn` is a wire
// token, and a rep cannot act on a wire token.
const held = {
  ...waiting,
  id: "019f7e65-fbf7-7114-b114-40af4af63ae9",
  status: "held",
  scheduled_at: inDays(-1, 17),
  held_reason: "consent_withdrawn",
  subject: "The follow-up",
  to: ["ops@acme.example", "cfo@acme.example", "legal@acme.example"],
};

// Picked in another zone, which the row NAMES. A rep who scheduled 09:00 from
// Berlin and reads from Hanoi must not be shown 14:00 with no explanation, and
// must not be shown 09:00 with no zone either.
const laterThisWeek = {
  ...waiting,
  id: "019f7e65-fbf7-7114-b114-40af4af63af1",
  scheduled_at: inDays(4, 14),
  subject: "The onboarding checklist",
};

const crossZone = {
  ...waiting,
  id: "019f7e65-fbf7-7114-b114-40af4af63aeb",
  scheduled_at: inDays(1, 15),
  scheduled_tz: readerZone === "Asia/Tokyo" ? "Europe/Berlin" : "Asia/Tokyo",
  subject: "The board update",
};

const settled = [
  {
    ...waiting,
    id: "019f7e65-fbf7-7114-b114-40af4af63aec",
    status: "sent",
    subject: "Last week's summary",
  },
  {
    ...waiting,
    id: "019f7e65-fbf7-7114-b114-40af4af63aed",
    status: "cancelled",
    subject: "The price change note",
  },
];

function stubQueue(rows: unknown[]): void {
  installFetchStub({ "GET /scheduled-sends": () => jsonResponse(rows) });
}

function Screen() {
  return (
    <StoryProviders>
      <ScheduledSendsScreen />
    </StoryProviders>
  );
}

export const FullQueue: Story = {
  render: () => {
    stubQueue([held, waiting, laterThisWeek, crossZone, ...settled]);
    return <Screen />;
  },
};

// The two verbs a waiting message offers. Only the queue proper carries them: a
// message that has gone offers neither, because the server would refuse both.
export const OnlyWaiting: Story = {
  render: () => {
    stubQueue([waiting, laterThisWeek, crossZone]);
    return <Screen />;
  },
};

// Moving is unconfirmed. The three common moments are one press.
// The picker below them starts at the send's own moment, so saving it untouched moves nothing.
export const MovingAMessage: Story = {
  render: () => {
    stubQueue([waiting]);
    return <Screen />;
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: /Reschedule: /,
      }),
    );
  },
};

// Withdrawing IS confirmed, and the asymmetry is the point: a moved message can
// be moved back, while a withdrawn one has to be written again from nothing —
// the approval it carried does not survive it.
export const WithdrawConfirm: Story = {
  render: () => {
    stubQueue([waiting]);
    return <Screen />;
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: /Withdraw: / }),
    );
  },
};

// One sentence, not three. A rep who has never scheduled anything is not reading
// three findings, and three empty blocks make a page that is simply clear look
// broken.
export const NothingScheduled: Story = {
  render: () => {
    stubQueue([]);
    return <Screen />;
  },
};

// The German copy, whose length is the thing worth looking at on a row that
// already carries a subject, a recipient line, a moment, a badge and two verbs.
export const FullQueueGerman: Story = {
  render: () => {
    stubQueue([held, waiting, laterThisWeek, crossZone]);
    return (
      <StoryProviders locale="de">
        <ScheduledSendsScreen />
      </StoryProviders>
    );
  },
};

// The Today cut of the queue: what goes out before midnight, and nothing else.
export const TodayOnly: Story = {
  render: () => {
    stubQueue([
      {
        ...waiting,
        id: "019f7e65-fbf7-7114-b114-40af4af63af2",
        scheduled_at: inDays(0, 23),
      },
      waiting,
    ]);
    return <Screen />;
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", { name: /Today/ }),
    );
  },
};

const rowProps = {
  readerZone,
  now: new Date(),
  timeOnly: false,
  isNext: false,
  movePending: false,
  onMove: () => {},
  onEdit: () => {},
  onWithdraw: () => {},
};

export const ChangedContext: Story = {
  render: () => (
    <StoryProviders>
      <ol className="scheduledsends-list">
        <SendRow
          {...rowProps}
          send={{ ...waiting, status: "held", held_reason: "followup_settled" }}
        />
        <SendRow
          {...rowProps}
          send={{ ...waiting, status: "held", held_reason: "meeting_canceled" }}
        />
      </ol>
    </StoryProviders>
  ),
};
export const ChangedContextPhone: Story = {
  ...ChangedContext,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A message the composer could write again: the row offers Edit beside Reschedule. */
export const Editable: Story = {
  render: () => (
    <StoryProviders>
      <ol className="scheduledsends-list">
        <SendRow
          {...rowProps}
          send={{
            ...waiting,
            status: "scheduled",
            links: [{ entity_type: "company", entity_id: "company-1" }],
          }}
        />
      </ol>
    </StoryProviders>
  ),
};

/** Edit refused, pointing at the line that says why: the composer has no field
 *  for the reason given when scheduling. Withdraw stays. */
export const EditRefused: Story = {
  render: () => (
    <StoryProviders>
      <ol className="scheduledsends-list">
        <SendRow
          {...rowProps}
          send={{
            ...waiting,
            status: "scheduled",
            subject: "The fair follow-up",
            links: [{ entity_type: "company", entity_id: "company-1" }],
            operator_reason: "Asked for the brochure at the fair",
          }}
        />
      </ol>
    </StoryProviders>
  ),
};
export const EditRefusedPhone: Story = {
  ...EditRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
