// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { HomeScreen } from "./home";
import {
  type Approval,
  bundle,
  deals,
  digest,
  lapsed,
  NOT_FOUND,
  narratedWeek,
  pipelineRows,
  quietRun,
  ranked,
  readingsDay,
  report,
  singles,
  WEEK_START,
  type WeeklyReview,
  type Worklist,
} from "./home.fixtures";
import type { MorningBrief, MorningDigest } from "./home.queries";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// Home — the morning handover, in the states a reader actually arrives at.
//
// The page has two moods and the order between them is the whole design: while
// decisions are waiting they LEAD (they are the only thing here with a
// deadline), and once the deck is clear the ranked queue leads. Both are frames
// below, because a catalog that only ever showed one of them would document
// half a page.
//
// Read every frame in BOTH themes with the toolbar's Theme control (it flips
// `data-theme` exactly the way the shell does). Nothing here is theme-aware in
// its own right, and that is precisely why it needs looking at: every colour on
// the deck's urgency edge, the staging tray, the readings strip and the rail's
// panels is a `color-mix()` of a canonical token, so a surface can be correct in
// light and wrong in dark.
//
// EVERY INSTANT IS FIXED. A fixture built with `new Date()` documents whatever
// day the catalog was opened on, and the two things on this page that read a
// clock — the greeting band and a proposal's expiry — would then say something
// different every time somebody looked. The one exception is deliberate and
// unavoidable: the greeting reads the real hour, because Home passes it its own
// clock. Expiries are therefore either ABSENT (calm, and stable forever) or a
// fixed instant in the past (the lapsed frame, which stays lapsed).

// ── The harness ─────────────────────────────────────────────────────────────

type Frame = {
  /** The pending queue. Every frame states it, because "none waiting" is the
   *  state that flips the page's order and is never a default worth guessing. */
  approvals: Approval[];
  /** The ranked run, or null for the honest 404 (no run has been made yet). */
  brief: MorningBrief | null;
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
   *  therefore did not render an empty Home, it crashed every frame in this
   *  file on `queue.length` of undefined. */
  day?: Worklist;
  /** Extra routes a frame's own play() needs. */
  extra?: RouteMap;
};

/**
 * One Home, with every read it fans out to answered.
 *
 * Five independent reads and no combined "my day" endpoint, so each of them is
 * routed on its own here — which is the point rather than bookkeeping: a frame
 * can refuse ONE of them and show that the other four still render.
 */
function home({
  approvals,
  brief,
  digest: overnight = digest,
  weekly = narratedWeek,
  pipeline = () => report(pipelineRows),
  day = readingsDay(),
  extra = {},
}: Frame) {
  return () => {
    // Mutable per render so a play() that commits a verdict sees the queue the
    // commit left behind, rather than the one it started with.
    const decided = new Set<string>();
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /approvals": () =>
        jsonResponse({
          data: approvals.filter((approval) => !decided.has(approval.id)),
          page: { next_cursor: null, has_more: false },
        }),
      // Keyed on the fixture's own id: spelling it out here would let a renamed
      // or reordered fixture fall through to the stub's empty page, which reads
      // as a successful send and leaves the frame waiting on a card that was
      // never cleared.
      [`POST /approvals/${singles[0].id}/approve`]: () => {
        decided.add(singles[0].id);
        return jsonResponse({ ...singles[0], status: "approved" });
      },
      "GET /brief": () =>
        brief ? jsonResponse(brief) : jsonResponse(NOT_FOUND, 404),
      "GET /weekly-reviews": () => jsonResponse({ weeks: [WEEK_START] }),
      "GET /weekly-reviews/latest": () =>
        weekly ? jsonResponse(weekly) : jsonResponse(NOT_FOUND, 404),
      "GET /digest": () =>
        overnight ? jsonResponse(overnight) : jsonResponse(NOT_FOUND, 404),
      "GET /deals": () =>
        jsonResponse({ data: deals, page: { next_cursor: null } }),
      "GET /organizations/org-nordwind": () =>
        jsonResponse({ id: "org-nordwind", display_name: "Nordwind Logistik" }),
      "GET /organizations/org-acme": () =>
        jsonResponse({ id: "org-acme", display_name: "Acme Fördertechnik" }),
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
      "GET /worklist": () => jsonResponse(day),
      "POST /reports/deals-by-stage": () => pipeline(),
      ...extra,
    });
    return (
      <StoryProviders>
        <HomeScreen />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof HomeScreen> = {
  title: "Shell/Home",
  component: HomeScreen,
};
export default meta;
type Story = StoryObj<typeof HomeScreen>;

// ── The deck ────────────────────────────────────────────────────────────────

// The morning it was designed for: four decisions waiting (three proposals and
// one act's bundle), a ranked queue under them, and the context rail beside.
// Decisions LEAD, because they are the only thing here with a deadline.
export const MorningDeck: Story = {
  render: home({ approvals: [...singles, ...bundle], brief: ranked }),
};

// The last card. "0 more behind" is drawn rather than hidden: a reader deciding
// one at a time is owed the size of what is left, including when it is nothing.
export const LastCard: Story = {
  render: home({ approvals: [singles[0]], brief: ranked }),
};

// The tray, which is the undo the backend does not have: a recorded decision
// cannot be reversed, so the verdict sits here — locally, nothing sent — until
// somebody presses commit.
export const StagedTray: Story = {
  render: home({ approvals: [...singles], brief: ranked }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Accept" }),
    );
    await canvas.findByText("1 decision staged");
  },
};

// The earned moment: everything the reader answered has gone, and the deck says
// how many and when. Reached by accepting one and deferring the other — "later"
// keeps its card pending, which is what leaves the deck with nothing waiting
// while the queue still holds something.
export const DeckCleared: Story = {
  render: home({ approvals: [singles[0], singles[1]], brief: ranked }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Accept" }),
    );
    await userEvent.click(await canvas.findByRole("button", { name: "Later" }));
    await userEvent.click(
      await canvas.findByRole("button", { name: "Send staged decisions" }),
    );
    await canvas.findByText("Deck clear");
  },
};

// Nothing is waiting, so the ORDER FLIPS: the ranked queue leads and the deck
// stands under it saying so. The question has stopped being "what needs me" and
// become "what do I do first".
export const RankedQueueLeads: Story = {
  render: home({ approvals: [], brief: ranked }),
};

// A proposal that ran out of time. The card keeps its place and its content —
// the reader still needs to know what was proposed — but the Accept control is
// gone rather than drawn to be refused.
export const ExpiredCard: Story = {
  render: home({ approvals: [lapsed], brief: ranked }),
};

// ── The ranked queue ────────────────────────────────────────────────────────

// The first morning: no run has ever been made, so the page offers to make one
// instead of drawing an empty queue that looks like a failure.
export const NoBriefYet: Story = {
  render: home({ approvals: [], brief: null }),
};

// A run that ranked nothing. The honest quiet, with no invented urgency —
// distinct from the frame above, which has no run at all.
export const QuietRun: Story = {
  render: home({ approvals: [], brief: quietRun }),
};

// ── The rail ────────────────────────────────────────────────────────────────

// Before the first nightly run there is no digest, so the Overnight panel is
// absent rather than a row of zeros: a fabricated count is worse than a missing
// one, because a reader cannot tell it apart from a real one.
export const DigestAbsent: Story = {
  render: home({ approvals: [...singles], brief: ranked, digest: null }),
};

// The one place connector health reaches a reader without visiting Settings. A
// degraded source is news — said in Settings' own vocabulary, with the way to
// fix it — while a healthy one stays silent, as it does in every other frame
// here: a permanent green row is noise.
export const ConnectorUnhealthy: Story = {
  render: home({
    approvals: [...singles],
    brief: ranked,
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
  render: home({
    approvals: [],
    brief: ranked,
    weekly: { ...narratedWeek, narrative: null, narrated_at: null },
  }),
};

/** A pass that ran and found the week unremarkable. No sentence and no notice
 *  — the stamp is what makes this different from the state above. */
export const WeeklyQuietlyNarrated: Story = {
  render: home({
    approvals: [],
    brief: ranked,
    weekly: { ...narratedWeek, narrative: null },
  }),
};

export const OnePanelRefused: Story = {
  render: home({
    approvals: [...singles],
    brief: ranked,
    pipeline: () =>
      jsonResponse({ title: "Forbidden", code: "forbidden" }, 403),
  }),
};

// The other half of the same honesty: the figures ARRIVED, and a field mask kept
// rows out of them. Saying so is the difference between a partial answer and a
// wrong one.
export const PipelinePartial: Story = {
  render: home({
    approvals: [],
    brief: ranked,
    pipeline: () => report(pipelineRows, 4),
  }),
};
