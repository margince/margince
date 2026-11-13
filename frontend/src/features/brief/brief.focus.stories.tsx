// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Eyebrow } from "../../design-system/eyebrow";
import { en } from "../../i18n/en";
import type { TaskType } from "../../lib/tasktype";
import {
  failedApprovalRow,
  legacyStalledRow,
  taskRow,
  waitingEmailRow,
} from "../../testing/brief.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { WorklistItem } from "../worklist/worklist.queries";
import { Triage } from "./brief.focus";

// THE FOCUS PANEL'S INSIDE: the card in hand on the stack of what is still
// behind it, and the ranked column beside it.
//
// What to check in each frame:
//   · the card reads label, work, whose row it is, verbs — the verbs across
//     its floor, the set-asides leading and the prepared move closing;
//   · the card hugs its own content and sits centred against the column, so a
//     long queue lengthens the column rather than stretching the card;
//   · the edges of two more cards show under it, and only as many as there
//     are rows behind;
//   · pressing a row in the column puts it in hand, and the card comes up off
//     the stack (the animation is CSS; a frame catches its end state).
//
// Both themes: the stack's edges, the card's ground and the indigo band above
// it are all `color-mix()` over canonical tokens and re-resolve on the flip.

const meta: Meta<typeof Triage> = {
  title: "Shell/Home/Focus",
  component: Triage,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof Triage>;

/** A day with a customer waiting at the top and five more behind it. */
function aDay(): WorklistItem[] {
  return [
    waitingEmailRow(),
    ...[1, 2, 3, 4, 5].map((n) =>
      taskRow(`followup-${n}`, `Follow up on customer commitment ${n}`),
    ),
  ];
}

/** The panel around it, so the card is judged in the width it is drawn at. */
function panel(rows: WorklistItem[], start = 0) {
  return () => {
    // The card's hand-off names the contact a reassignment moves a task away
    // from, and on the reader's own queue that is whoever `/me` says. Left to
    // the stub's fallback it answers a list-shaped body, which reads as a
    // malformed session: every grant fails closed and the frame draws a branch
    // no story here is named for.
    installFetchStub({ "GET /me": meRoute({}, { roles: ["rep"] }) });
    return (
      <StoryProviders>
        <Chosen rows={rows} start={start} />
      </StoryProviders>
    );
  };
}

function Chosen({
  rows,
  start,
}: Readonly<{ rows: WorklistItem[]; start: number }>) {
  const [at, setAt] = useState(start);
  const lead = rows[at];
  if (lead === undefined) {
    return null;
  }
  return (
    <section id="brief-today" className="brief-first">
      {/* The section's own head, because the card is judged against it: the
          same eyebrow the Brief draws over this. */}
      <div className="brief-first-head">
        <Eyebrow as="h2">{en["brief.first.title"]}</Eyebrow>
      </div>
      <Triage
        rows={rows}
        at={at}
        lead={lead}
        onChoose={(item) => setAt(rows.indexOf(item))}
        onOpenEmail={() => undefined}
        onContext={() => undefined}
      />
    </section>
  );
}

async function focusedCard(canvasElement: HTMLElement) {
  const card = await waitFor(() => {
    const found = canvasElement.querySelector<HTMLElement>(
      ".brief-triage-frame > article",
    );
    if (!found) throw new Error("the chosen task drew no card");
    return found;
  });
  return within(card);
}

/** The first row in hand: a customer waiting, with five behind it. */
export const InHand: Story = { render: panel(aDay()) };

/** Pressing the fourth row puts it in hand and says where it stands. */
export const Pressed: Story = {
  render: panel(aDay()),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: /^Follow up on customer commitment 3/,
      }),
    );
    const chosen = await focusedCard(canvasElement);
    await expect(
      await chosen.findByText("Follow up on customer commitment 3"),
    ).toBeTruthy();
    await expect(chosen.getByText("4")).toBeTruthy();
  },
};

/** A row about an account in hand: the account linked, and its own pair. */
export const AccountInHand: Story = {
  render: panel([
    {
      ...taskRow("renewal", "Send the renewal terms"),
      subject: { type: "company", id: "company-nordwind", label: "Nordwind" },
      company: {
        id: "company-nordwind",
        touch: {
          last_inbound_at: "2026-03-12T10:05:00Z",
          last_outbound_at: null,
        },
      },
    },
    taskRow("one", "Call the buyer"),
  ]),
};

/** A task whose name is longer than one headline line: it wraps inside the
    card, starting where the card's text starts. */
export const LongTaskInHand: Story = {
  render: panel([
    taskRow(
      "retrofit",
      "Call Alice back about the retrofit quote and the revised delivery window",
    ),
    taskRow("one", "Call the buyer"),
  ]),
};

const callBack: WorklistItem = {
  ...taskRow("call-back", "Call back Alice about the renewal"),
  contact: { id: "c-alice", label: "Alice Brandt" },
};

type TypedTask = Readonly<{
  type: TaskType;
  title?: string;
  primaryEmail?: string | null;
  extra?: Record<string, unknown>;
  routes?: Parameters<typeof installFetchStub>[0];
}>;

function typedPanel({
  type,
  title = callBack.title,
  primaryEmail = "alice@brandt.example",
  extra = {},
  routes = {},
}: TypedTask) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }, { roles: ["rep"] }),
      "GET /activities/call-back": () =>
        jsonResponse({
          id: "call-back",
          kind: "task",
          subject: title,
          occurred_at: "2026-09-08T09:00:00Z",
          is_done: false,
          task_type: type,
          source_activity_id: "call-1",
          links: [{ entity_type: "contact", entity_id: "c-alice" }],
          // The server names a mail target for a mail task only.
          ...(type === "reach_out" || type === "reply"
            ? {
                email_target: {
                  entity_type: "contact",
                  entity_id: "c-alice",
                  ...(primaryEmail ? {} : { refusal: "no_address" }),
                },
              }
            : {}),
          ...extra,
        }),
      "GET /activities/call-1": () =>
        jsonResponse({
          id: "call-1",
          kind: "call",
          content_state: "available",
          occurred_at: "2026-09-08T08:00:00Z",
        }),
      "GET /contacts/c-alice": () =>
        jsonResponse({
          id: "c-alice",
          full_name: "Alice Brandt",
          primary_email: primaryEmail,
          phones: [{ id: "ph-1", phone: "+491517654321", is_primary: true }],
        }),
      ...routes,
    });
    return (
      <StoryProviders>
        <Chosen
          rows={[{ ...callBack, title }, taskRow("one", "Call the buyer")]}
          start={0}
        />
      </StoryProviders>
    );
  };
}

/** A reply to a call in hand: the number to copy, a filled Call, Done
    outlined, and Next task at the end of the title. */
export const CallTaskInHand: Story = {
  render: typedPanel({ type: "reply", extra: { reply_channel: "call" } }),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await card.findByRole("button", { name: en["task.next.copyNumber"] });
    await expect(
      card.getByRole("link", { name: en["record.verb.call"] }),
    ).toBeTruthy();
  },
};
export const CallTaskInHandDark: Story = {
  ...CallTaskInHand,
  globals: { theme: "dark" },
};

/** Reach out to a contact with no address: Email refused, the reason one
    line under every verb, and Call is the filled verb. */
export const ReachOutNoAddressInHand: Story = {
  render: typedPanel({ type: "reach_out", primaryEmail: null }),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await card.findByText(en["task.next.noAddress"]);
  },
};

const offeredDeal = (id: string, name: string, amount: number) => () =>
  jsonResponse({
    id,
    name,
    status: "open",
    amount_minor: amount,
    currency: "EUR",
    expected_close_date: "2026-12-15",
  });

/** A deal question in hand: the deals as cards, then Link email thread to deal. */
export const DecideTaskInHand: Story = {
  render: typedPanel({
    type: "decide",
    title: "Which deal is the Nordwind mail about?",
    extra: {
      source_system: "which_deal",
      raw: { offered_deal_ids: ["d-1", "d-2"] },
    },
    routes: {
      "GET /deals/d-1": offeredDeal("d-1", "Nordwind renewal 2027", 42_000_000),
      "GET /deals/d-2": offeredDeal("d-2", "Nordwind pilot", 8_500_000),
    },
  }),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await card.findByRole("radio", { name: /renewal 2027/ });
  },
};
export const DecideTaskInHandDark: Story = {
  ...DecideTaskInHand,
  globals: { theme: "dark" },
};

/** A task to do: no step of its own, so Done is the filled button. */
export const DoTaskInHand: Story = {
  render: typedPanel({ type: "do", title: "Print the signed contract" }),
};

/** The last row of a short day: the one before it is listed under the card. */
export const NothingBehindIt: Story = {
  render: panel([waitingEmailRow(), taskRow("one", "Call the buyer")], 1),
};

const termination: WorklistItem = {
  occurred_precision: "instant",
  id: "00000000-0000-7000-8000-0000000d0001",
  source: "approval",
  kind: "lifecycle_change",
  category: "decisions",
  level: 1,
  band: "now",
  title: "Review Atlas account termination",
  because: [{ kind: "account_termination" }],
  consequence: "work_blocked",
  actions: ["decide"],
};
export const AccountTermination: Story = {
  render: panel([termination, waitingEmailRow()]),
};
export const AccountTerminationDark: Story = {
  ...AccountTermination,
  globals: { theme: "dark" },
};

export const ApprovedButNotRun: Story = {
  render: panel([failedApprovalRow(), taskRow("call", "Call the buyer")]),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await expect(card.getByText("Move deal to Won")).toBeTruthy();
    await expect(card.getByText(failedApprovalRow().detail ?? "")).toBeTruthy();
    await expect(
      card.getByRole("button", { name: en["worklist.verb.retry"] }),
    ).toBeTruthy();
  },
};

export const LegacyStalledDeal: Story = {
  render: panel([
    {
      ...legacyStalledRow(),
      deal: {
        amount_minor: 2500000,
        currency: "EUR",
        close_date_provisional: false,
      },
    },
    {
      ...legacyStalledRow(),
      id: "renewal",
      subject: { type: "deal", id: "deal-renewal", label: "Globex Renewal" },
      deal: {
        amount_minor: 1200000,
        currency: "EUR",
        close_date_provisional: false,
      },
    },
    detailedDeal(),
  ]),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await expect(
      card.getByRole("heading", { name: "Acme Expansion" }),
    ).toBeTruthy();
    await expect(card.getByText("€25,000.00")).toBeTruthy();
    await expect(
      canvasElement.querySelector(".worklist-row-context"),
    ).toBeNull();
    await expect(card.getAllByText("Acme Expansion")).toHaveLength(1);
    await expect(
      card.getByRole("link", { name: en["deal.openDeal"] }),
    ).toBeTruthy();
    await expect(card.queryByText("Why first?")).toBeNull();
    // Mark handled is folded into the card's menu, drawn outside the card.
    await expect(
      card.queryByRole("button", { name: en["brief.act"] }),
    ).toBeNull();
    await userEvent.click(
      card.getByRole("button", { name: /More actions for Acme Expansion/ }),
    );
    await expect(
      await within(canvasElement.ownerDocument.body).findByRole("button", {
        name: en["brief.act"],
      }),
    ).toBeTruthy();
  },
};

/** A deal closing soon: the card says so first, and keeps its verbs on one line. */
export const ClosingSoonDeal: Story = {
  render: panel([
    {
      ...legacyStalledRow(),
      kind: "closing_soon",
      because: [{ kind: "closing_soon" }, { kind: "no_next_step" }],
    },
    detailedDeal(),
  ]),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await expect(
      card.getByText(en["worklist.ask.brief.closing_soon"]),
    ).toBeTruthy();
    await expect(
      card.queryByText(en["worklist.because.closing_soon"]),
    ).toBeNull();
    await expect(
      card.getByRole("link", { name: en["deal.openDeal"] }),
    ).toBeTruthy();
    // Mark handled, Snooze and Dismiss wait in the card's menu.
    await expect(
      card.queryByRole("button", { name: en["brief.dismiss"] }),
    ).toBeNull();
  },
};

export const LegacyStalledDealDark: Story = {
  ...LegacyStalledDeal,
  globals: { theme: "dark" },
};
export const LegacyStalledDealPhone: Story = {
  ...LegacyStalledDeal,
  tags: ["uat-phone"],
};
function detailedDeal(): WorklistItem {
  return {
    ...legacyStalledRow(),
    id: "rollout",
    subject: { type: "deal", id: "deal-rollout", label: "Nordwind Rollout" },
    title: "Send the revised rollout proposal",
    verdict: {
      source: "brief_finding",
      line: "The buyer requested a revised proposal.",
    },
  };
}

export const DealWithNextAction: Story = {
  render: panel([detailedDeal()]),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await expect(
      card.getByRole("heading", { name: "Nordwind Rollout" }),
    ).toBeTruthy();
    await expect(
      card.getByText("Send the revised rollout proposal"),
    ).toBeTruthy();
    await expect(
      card.getByText("The buyer requested a revised proposal."),
    ).toBeTruthy();
  },
};

export const RecordedNextStepGap: Story = {
  render: panel([
    {
      ...legacyStalledRow(),
      because: [{ kind: "no_next_step" }, { kind: "material" }],
      deal: {
        amount_minor: 2500000,
        currency: "EUR",
        close_date_provisional: false,
      },
    },
  ]),
  play: async ({ canvasElement }) => {
    const card = await focusedCard(canvasElement);
    await waitFor(() =>
      expect(card.getByText(en["worklist.because.no_next_step"])).toBeVisible(),
    );
    await expect(
      card.getByRole("link", { name: en["deal.openDeal"] }),
    ).toBeVisible();
    await expect(
      card.queryByText(/typical|If ignored|Ranked above/),
    ).toBeNull();
  },
};
export const RecordedNextStepGapDark: Story = {
  ...RecordedNextStepGap,
  globals: { theme: "dark" },
};
export const RecordedNextStepGapPhone: Story = {
  ...RecordedNextStepGap,
  tags: ["uat-phone"],
};

export const LongTaskTitlePhone: Story = {
  tags: ["uat-phone"],
  render: panel([
    {
      ...taskRow(
        "long-call",
        "Call back about retrofit delivery and the revised installation schedule",
      ),
      subject: { type: "contact", id: "alice", label: "Alice" },
    },
  ]),
  play: async ({ canvasElement }) => {
    const title = within(canvasElement).getByRole("button", {
      name: "Call back about retrofit delivery and the revised installation schedule",
    });
    await expect(title).toHaveClass("btn-valuelabel");
    await expect(title.scrollWidth).toBeLessThanOrEqual(title.clientWidth + 1);
  },
};
export const LongTaskTitlePhoneDark: Story = {
  ...LongTaskTitlePhone,
  tags: ["uat-phone"],
  globals: { theme: "dark" },
};
