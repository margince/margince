// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en } from "../i18n/en";
import { BriefScreen } from "./brief";
import {
  type Approval,
  bundle,
  deals,
  decisionRow,
  digest,
  lapsed,
  NOT_FOUND,
  narratedWeek,
  pipelineRows,
  readingsDay,
  report,
  singles,
  taskRow,
  WEEK_START,
  type WeeklyReview,
  type Worklist,
  waitingEmailRow,
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
};

function brief({
  approvals,
  digest: overnight = digest,
  weekly = narratedWeek,
  pipeline = () => report(pipelineRows),
  day,
  extra = {},
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
    return (
      <StoryProviders>
        <BriefScreen />
      </StoryProviders>
    );
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
    await within(canvasElement).findByText("2 priorities in focus");
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
// The row in hand is a customer's MESSAGE: the canonical email row names it —
// who wrote, when, the subject at the headline rung, their own words under
// it — and the verbs that answer it stand under the words. The queue beside
// it holds the rest of the day; pressing a row puts it in hand instead.
export const SixPriorities: Story = {
  render: brief({
    approvals: [],
    day: readingsDay({}, [
      waitingEmailRow(),
      promise,
      otherPromise,
      ...[1, 2, 3, 4].map((n) =>
        taskRow(`followup-${n}`, `Follow up on customer commitment ${n}`),
      ),
    ]),
    // Sonya's own record, for the two moments the row in hand reads off it.
    extra: {
      "GET /contacts/contact-sonya/360": () =>
        jsonResponse({
          contact: { id: "contact-sonya", full_name: "Sonya Beck" },
          last_inbound_at: "2026-09-03T16:46:00Z",
          last_outbound_at: "2026-08-28T09:12:00Z",
          sections_omitted: [],
        }),
    },
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
