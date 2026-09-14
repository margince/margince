// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useEffect } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { announceAddressChanged } from "../app/router";
import { Shell } from "../app/shell";
import { en } from "../i18n/en";
import { BriefScreen } from "./brief";
import {
  type Approval,
  bundle,
  deals,
  decisionRow,
  digest,
  lapsed,
  leadRow,
  meetingRow,
  NOT_FOUND,
  narratedWeek,
  overnightRow,
  pipelineRows,
  readingsDay,
  report,
  singles,
  taskRow,
  WEEK_START,
  type WeeklyReview,
  type Worklist,
  wholeDecisions,
  wholeMeetings,
} from "./brief.fixtures";
import type { MorningDigest } from "./brief.queries";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Decisions share the ranked agenda. Their full proposal opens in the same
// review drawer used by the worklist, and accepting one refreshes the agenda.
// Fixed fixtures keep both themes and the expired state reproducible.

type Frame = {
  /** Proposals available to the agenda and its review drawers. */
  approvals: Approval[];
  /** The nightly digest, or null for the 404 an installation answers before its
   *  first run. */
  digest?: MorningDigest | null;
  /** The week just gone, or null for the 404 a rep sees before their first
   *  full week. Its two null-narrative states are the ones worth a screenshot:
   *  a week nobody narrated, and a week a pass found unremarkable. */
  weekly?: WeeklyReview | null;
  /** What the pipeline report answers. A refusal is a state of its own. */
  pipeline?: () => Response;
  /** The morning as the worklist answers it. Stated rather than left to the
   *  stub's fallback, because the fallback is a LIST page — `{data, page}` —
   *  and `/worklist` answers a `Worklist`, whose `queue` the walk reads to
   *  decide whether to ask for a second page. A fallback of the wrong envelope
   *  therefore did not render an empty Brief, it crashed every frame in this
   *  file on `queue.length` of undefined. */
  day?: Worklist;
  /** Extra routes a frame's own play() needs. */
  extra?: RouteMap;
  /** What stands around the screen, inside the providers: the shell, here. */
  frameWith?: (screen: ReactNode) => ReactNode;
};

function brief({
  approvals,
  digest: overnight = digest,
  weekly = narratedWeek,
  pipeline = () => report(pipelineRows),
  day,
  extra = {},
  frameWith = (screen) => screen,
}: Frame) {
  return () => {
    const decided = new Set<string>();
    const approvalRoutes: RouteMap = {};
    for (const approval of approvals) {
      approvalRoutes[`GET /approvals/${approval.id}`] = () =>
        jsonResponse(approval);
      approvalRoutes[`POST /approvals/${approval.id}/approve`] = () => {
        decided.add(approval.id);
        return jsonResponse({ ...approval, status: "approved" });
      };
    }
    const morning = () => {
      if (day) return day;
      const pending = approvals.filter((approval) => !decided.has(approval.id));
      const decisions = pending.map((approval) => ({
        ...decisionRow(approval.id, approval.kind === "send_email"),
        kind: approval.kind,
        title: approval.summary ?? approval.kind,
        due_at: approval.expires_at ?? undefined,
      }));
      return readingsDay(
        { review: pending.length },
        [...decisions, ...readingsDay().queue],
        [wholeDecisions(pending.length), wholeMeetings(2)],
      );
    };
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /approvals": () =>
        jsonResponse({
          data: approvals.filter(
            (approval) => approval.bundle_id && !decided.has(approval.id),
          ),
          page: { next_cursor: null, has_more: false },
        }),
      ...approvalRoutes,
      "GET /weekly-reviews": () => jsonResponse({ weeks: [WEEK_START] }),
      "GET /weekly-reviews/latest": () =>
        weekly ? jsonResponse(weekly) : jsonResponse(NOT_FOUND, 404),
      "GET /digest": () =>
        overnight ? jsonResponse(overnight) : jsonResponse(NOT_FOUND, 404),
      "GET /deals": () =>
        jsonResponse({ data: deals, page: { next_cursor: null } }),
      "GET /companies/company-nordwind": () =>
        jsonResponse({
          id: "company-nordwind",
          display_name: "Nordwind Logistik",
        }),
      "GET /companies/company-acme": () =>
        jsonResponse({
          id: "company-acme",
          display_name: "Acme Fördertechnik",
        }),
      "GET /projects/01a00000-0000-7000-8000-000000000001": () =>
        jsonResponse({
          id: "01a00000-0000-7000-8000-000000000001",
          name: "ERP replacement",
        }),
      "GET /projects/01a00000-0000-7000-8000-000000000002": () =>
        jsonResponse({
          id: "01a00000-0000-7000-8000-000000000002",
          name: "Depot rollout",
        }),
      // The screen reads this before it draws anything: the team toggle, the
      // coverage line and the readings strip are all cuts of this one answer.
      "GET /worklist": () => jsonResponse(morning()),
      "GET /worklist/handled": () =>
        jsonResponse({
          as_of: morning().as_of,
          receipts: [],
          truncated: false,
        }),
      "POST /reports/deals-by-stage": () => pipeline(),
      ...extra,
    });
    return <StoryProviders>{frameWith(<BriefScreen />)}</StoryProviders>;
  };
}

const meta: Meta<typeof BriefScreen> = {
  parameters: { layout: "fullscreen" },
  title: "Shell/Home",
  component: BriefScreen,
};
export default meta;
type Story = StoryObj<typeof BriefScreen>;

export const DecisionsInToday: Story = {
  render: brief({ approvals: [...singles, ...bundle] }),
};

export const OneDecision: Story = {
  render: brief({ approvals: [singles[1]] }),
};

async function openDecision(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByRole("button", { name: "Decide" }));
  return within(
    await within(canvasElement.ownerDocument.body).findByRole("dialog", {
      name: "Your decision",
    }),
  );
}

export const DecisionOpened: Story = {
  render: brief({ approvals: [singles[1]] }),
  play: async ({ canvasElement }) => {
    const drawer = await openDecision(canvasElement);
    await drawer.findByRole("button", { name: en["brief.approval.approve"] });
  },
};

export const DecisionApproved: Story = {
  render: brief({ approvals: [singles[1]] }),
  play: async ({ canvasElement }) => {
    const drawer = await openDecision(canvasElement);
    await userEvent.click(
      await drawer.findByRole("button", { name: en["brief.approval.approve"] }),
    );
    await waitFor(() =>
      expect(
        within(canvasElement).queryByRole("button", { name: "Decide" }),
      ).toBeNull(),
    );
    await within(canvasElement).findByText("2 focus cards");
  },
};

export const NoDecisions: Story = {
  render: brief({ approvals: [] }),
};

export const ExpiredDecision: Story = {
  render: brief({ approvals: [lapsed] }),
  play: async ({ canvasElement }) => {
    const drawer = await openDecision(canvasElement);
    await drawer.findAllByText(en["decision.expired"]);
    await expect(
      drawer.queryByRole("button", { name: en["brief.approval.email"] }),
    ).toBeNull();
  },
};

// ── The rail ────────────────────────────────────────────────────────────────

// Before the first nightly run there is no digest, so the Overnight panel is
// absent rather than a row of zeros: a fabricated count is worse than a missing
// one, because a reader cannot tell it apart from a real one.
export const DigestAbsent: Story = {
  render: brief({ approvals: [...singles], digest: null }),
};

// The one place connector health reaches a reader without visiting Settings. A
// degraded source is news — said in Settings' own vocabulary, with the way to
// fix it — while a healthy one stays silent, as it does in every other frame
// here: a permanent green row is noise.
export const ConnectorUnhealthy: Story = {
  render: brief({
    approvals: [...singles],
    digest: {
      ...digest,
      connectors: [
        {
          provider: "gmail",
          status: "reauth_required",
          last_sync_error_class: "auth",
        },
      ],
    },
  }),
};

// One read refused while the other four are healthy. That is the whole reason
// this page fans out to five independent reads: the pipeline says the figure
// could not be loaded — a refusal, not an absence — and the deck, the queue, the
// digest and the quiet list are untouched beside it.
/** The honest degrade: the week was measured and nobody narrated it. The
 *  numbers are all there, and the panel says the sentence is missing rather
 *  than letting the reader conclude there was nothing to say. */
export const WeeklyWithoutItsSentence: Story = {
  render: brief({
    approvals: [],
    weekly: { ...narratedWeek, narrative: null, narrated_at: null },
  }),
};

/** A pass that ran and found the week unremarkable. No sentence and no notice
 *  — the stamp is what makes this different from the state above. */
export const WeeklyQuietlyNarrated: Story = {
  render: brief({
    approvals: [],
    weekly: { ...narratedWeek, narrative: null },
  }),
};

export const OnePanelRefused: Story = {
  render: brief({
    approvals: [...singles],
    pipeline: () =>
      jsonResponse({ title: "Forbidden", code: "forbidden" }, 403),
  }),
};

// The other half of the same honesty: the figures ARRIVED, and a field mask kept
// rows out of them. Saying so is the difference between a partial answer and a
// wrong one.
export const PipelinePartial: Story = {
  render: brief({
    approvals: [],
    pipeline: () => report(pipelineRows, 4),
  }),
};

const promise = taskRow(
  "promise",
  "Prepare a comparison showing two products and both translation options.",
);
const otherPromise = {
  ...taskRow(
    "other-promise",
    "Prepare the revised comparison sheet for the same customer.",
  ),
  due_at: "2026-09-11T21:59:59Z",
};
export const SixPriorities: Story = {
  render: brief({
    approvals: [],
    day: readingsDay({}, [
      promise,
      otherPromise,
      ...[1, 2, 3, 4].map((n) =>
        taskRow(`followup-${n}`, `Follow up on customer commitment ${n}`),
      ),
    ]),
  }),
};
export const TaskEvidence: Story = {
  render: brief({
    approvals: [],
    day: readingsDay({}, [promise, otherPromise]),
    extra: {
      "GET /activities/promise": () =>
        jsonResponse({
          id: "promise",
          kind: "task",
          subject: promise.title,
          body: "Original commitment from the first meeting: prepare both translation variants by 9 September. A later meeting may record a separate deadline; confirm which commitment remains before closing either task.",
          occurred_at: "2026-09-07T10:00:00Z",
          version: 1,
          is_done: false,
        }),
    },
  }),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      (
        await within(canvasElement).findAllByRole("button", {
          name: en["brief.focus.context"],
        })
      )[0],
    );
  },
};
export const SixPrioritiesPhone: Story = {
  ...SixPriorities,
  tags: ["uat-phone"],
};

// ── Home inside the shell ───────────────────────────────────────────────────
//
// Every frame above renders the screen alone. These render it where a reader
// meets it: under the top bar, beside the rail, at the width the shell leaves
// it — which is the only place a Focus grid's column count or a drawer's
// scrim can be judged. The address is set to `#/home` so the shell heads and
// grids the page as the router would.

function InShell({ children }: Readonly<{ children: ReactNode }>) {
  const client = useQueryClient();
  if (client.getQueryData(["company"]) === undefined) {
    client.setQueryData(["company"], {
      company_id: "company-1",
      display_name: "Gradion GmbH",
    });
  }
  useEffect(() => {
    const before = window.location.hash;
    window.location.hash = "#/home";
    announceAddressChanged();
    return () => {
      window.location.hash = before;
      announceAddressChanged();
    };
  }, []);
  return (
    <Shell onOpenSearch={() => undefined} counts={{ inbox: 12, tasks: 4 }}>
      {children}
    </Shell>
  );
}

/** A mixed morning: a customer waiting, a deal drifting, a meeting to prepare,
 *  two promises and a proposal the runner staged. The cards say why each is
 *  here; the one door to the queue is the Focus footer. */
const mixedMorning: Frame = {
  approvals: [singles[1]],
  day: readingsDay(
    { review: 1 },
    [
      // The product's rows name their subject, which is what gives a card its
      // address; the bare fixtures leave it out, and a card with no address
      // is a card with no door.
      { ...leadRow("lead-1"), subject: { type: "lead", id: "lead-1" } },
      overnightRow("deal-1", "d-1"),
      meetingRow("meet-1", false),
      promise,
      {
        ...decisionRow(singles[1].id, false),
        kind: singles[1].kind,
        title: singles[1].summary ?? singles[1].kind,
      },
      otherPromise,
    ],
    [wholeDecisions(1), wholeMeetings(2)],
  ),
  extra: {
    "GET /ai/usage": () =>
      jsonResponse({
        days: [],
        budget: { monthly_tokens: 100, spent_tokens: 20, band: "normal" },
      }),
  },
};

function inShell(frame: Frame) {
  return brief({
    ...frame,
    frameWith: (screen) => <InShell>{screen}</InShell>,
  });
}

export const InTheShell: Story = {
  render: inShell(mixedMorning),
};

export const InTheShellQueueOpen: Story = {
  render: inShell(mixedMorning),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("link", {
        name: en["brief.feed.fullWorklist"],
      }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("dialog", {
      name: en["brief.queue.title"],
    });
  },
};

export const InTheShellDecisionOpen: Story = {
  render: inShell(mixedMorning),
  play: async ({ canvasElement }) => {
    await openDecision(canvasElement);
  },
};

export const InTheShellPhone: Story = {
  render: inShell(mixedMorning),
  tags: ["uat-phone"],
};
