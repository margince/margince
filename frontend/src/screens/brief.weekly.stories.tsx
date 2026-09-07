// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import { userEvent, within } from "storybook/test";
import { useLocale } from "../i18n";
import {
  firstWeek,
  narratedWeek,
  PRIOR_WEEK_START,
  WEEK_START,
  type WeeklyReview,
  weeklyLearnings,
  weeklyOutlook,
  weeklyScorecard,
  wholeWeek,
} from "./brief.fixtures";
import { OutlookPanel } from "./brief.waterfall";
import { WeeklySection } from "./brief.weekly";
import { LearningsPanel } from "./brief.weekly.learnings";
import { ScorecardPanel } from "./brief.weekly.scorecard";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The week just gone, drawn on its own.
//
// `brief.stories.tsx` documents the whole morning, and the weekly reaches a
// reader there only through whichever view the dial is on — so the panel's own
// states had no frame of their own. They are worth one, because what this panel
// draws differently is a question of what the review SAYS, and three of those
// answers are SILENCES a reader has to be able to tell apart: nobody narrated
// the week, a pass ran and found it unremarkable, and there is no review at
// all. Two of them are the same blank space unless the panel says which.
//
// The three panels that hang off the same review — the landing, the scorecard
// and the lessons — are documented here too, each from a review already in
// hand. Each of them is drawn only when the snapshot carries its lane, so the
// section's own frames above show none of them.
//
// NO SESSION ROUTE ANYWHERE BELOW, and the absence is the deliberate kind: this
// panel consults no grant and no seat, so nothing here asks for `GET /me`. A
// `meRoute` line would be a claim about a principal the surface never reads.
//
// EVERY INSTANT IS FIXED. A review built from `new Date()` documents whichever
// day the catalog was opened on, and both dates this panel prints — the week it
// names and the day each deal line closed — would then say something different
// every time somebody looked.

/**
 * The panel with both of its reads answered.
 *
 * The review and the archive index are SEPARATE reads, so each frame states
 * both: the picker only appears above a rep who has more than one week, and a
 * frame that left the index to the stub's empty page would silently be a story
 * about a rep with no archive.
 */
function weekly(review: () => Response, weeks: readonly string[]): RouteMap {
  return {
    "GET /weekly-reviews": () => jsonResponse({ weeks }),
    "GET /weekly-reviews/latest": review,
  };
}

function panel(routes: RouteMap) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <WeeklySection />
      </StoryProviders>
    );
  };
}

/**
 * One panel of the weekly, from a review already in hand.
 *
 * The three panels below take their review as a PROP — the section above reads
 * it once and hands each of them their slice — so these frames route nothing.
 * The empty stub is still installed, because a frame that left the engine's own
 * fetch in place would let a stray read leave the iframe and resolve to
 * something that looks like an answer.
 */
function frozen(node: ReactNode) {
  return () => {
    installFetchStub({});
    return <StoryProviders>{node}</StoryProviders>;
  };
}

/**
 * The outlook with its dial live.
 *
 * `OutlookPanel` holds no horizon of its own — the two surfaces that draw it
 * (this one and the team's week) each keep their own, so a reader who turns one
 * does not turn the other — which means a frame has to hold it or the control
 * would be a control that cannot move.
 */
function OutlookFrame({
  outlook,
}: Readonly<{ outlook: NonNullable<WeeklyReview["outlook"]> }>) {
  const { locale } = useLocale();
  const [horizon, setHorizon] = useState("quarter");
  return (
    <OutlookPanel
      outlook={outlook}
      locale={locale}
      horizon={horizon}
      onHorizon={setHorizon}
    />
  );
}

const meta: Meta<typeof WeeklySection> = {
  title: "Shell/Brief weekly review",
  component: WeeklySection,
};
export default meta;
type Story = StoryObj<typeof WeeklySection>;

// The Monday the panel was designed for: the sentence marked as agent-authored,
// five outcomes read across as one comparison, the other five figures as a list
// under them, and the week's closed deals last.
export const NarratedWeek: Story = {
  render: panel(
    weekly(() => jsonResponse(narratedWeek), [WEEK_START, PRIOR_WEEK_START]),
  ),
};

// The honest degrade: the week was measured and nobody narrated it. Every
// number is still there, and the panel says the sentence is missing — a rep who
// met silence instead would conclude there was nothing to remark on, when the
// truth is that no pass ran.
export const WithoutItsSentence: Story = {
  render: panel(
    weekly(
      () =>
        jsonResponse({ ...narratedWeek, narrative: null, narrated_at: null }),
      [WEEK_START, PRIOR_WEEK_START],
    ),
  ),
};

// A pass that ran and found the week unremarkable: no sentence AND no notice.
// The stamp is the only thing separating this frame from the one above, which is
// exactly why both are here — on screen they differ by one line of caption.
export const QuietlyNarrated: Story = {
  render: panel(
    weekly(
      () => jsonResponse({ ...narratedWeek, narrative: null }),
      [WEEK_START, PRIOR_WEEK_START],
    ),
  ),
};

// A first week: no delta line under any slot, and no deal lines under the
// figures. A rep's first week did not stay level — it had nothing to stay level
// against, and "±0" beside every figure would claim a comparison nobody made.
export const FirstWeek: Story = {
  render: panel(weekly(() => jsonResponse(firstWeek), [WEEK_START])),
};

// A week whose wins cannot be priced. One deal in a currency with no usable
// rate makes the whole sum unanswerable — an open deal freezes no rate, and
// nothing is converted at an invented rate of 1 — so the slot falls back to the
// COUNT of deals won.
//
// The frame beside `NarratedWeek` is the point: "€96,500.00" and "3" are two
// different claims about one week, and a reader has to be able to tell which
// they are looking at without knowing the FX rules.
export const WinsCouldNotBePriced: Story = {
  render: panel(
    weekly(
      () => jsonResponse({ ...narratedWeek, pipeline: undefined }),
      [WEEK_START, PRIOR_WEEK_START],
    ),
  ),
};

// The archive, open. The picker is the only door to a past week — the product
// deliberately gives the retrospective no nav entry — so a header that drew no
// picker for a rep with weeks behind them would strand every one of them.
export const ArchiveOfWeeks: Story = {
  render: panel(
    weekly(
      () => jsonResponse(narratedWeek),
      [WEEK_START, PRIOR_WEEK_START, "2026-06-15", "2026-06-08"],
    ),
  ),
  play: async ({ canvasElement }) => {
    // The trigger, not the list: the listbox portals to the document, and a
    // canvas-scoped lookup for it would reject after the frame was taken.
    await userEvent.click(
      await within(canvasElement).findByRole("combobox", {
        name: "Open another week",
      }),
    );
  },
};

// The rep whose first Monday has not come round yet. A 404 is the honest answer
// here rather than a failure, and the panel says so in a sentence: a page of
// zeroes would claim a week that was measured and empty.
export const NoReviewYet: Story = {
  render: panel(weekly(() => jsonResponse({ title: "Not Found" }, 404), [])),
};

// The read refused. A failure and an absence are different facts, and this is
// the one the panel must not draw as "no review yet": nothing here says the
// week was empty, because nobody knows what the week held.
export const ReadRefused: Story = {
  render: panel(
    weekly(
      () => jsonResponse({ title: "Internal Server Error" }, 500),
      [WEEK_START],
    ),
  ),
};

// ── The week with every lane answered ───────────────────────────────────────

// The section above draws the figures; the three panels below hang off the same
// review and only appear when the snapshot carries their lane. `NarratedWeek`
// carries none of them, so the whole shape is only visible here.
export const EveryLane: Story = {
  render: panel(
    weekly(() => jsonResponse(wholeWeek), [WEEK_START, PRIOR_WEEK_START]),
  ),
};

// ── Where the week was landing ──────────────────────────────────────────────

// The frozen outlook: four figures across, a landing beside them with the
// measure that produced it, and the bridge from Monday's landing to Friday's
// under all of it. The dial moves between the three horizons the review froze.
export const Outlook: Story = {
  render: frozen(<OutlookFrame outlook={weeklyOutlook} />),
};

// No forecast was composed when the snapshot was written, said in WORDS.
//
// This is the frame the panel exists for: a team that forecast nothing and one
// that landed on nothing are different facts, and a strip of zeros would claim
// the second. Read beside `Outlook`, the difference is the whole point.
export const OutlookNotForecast: Story = {
  render: frozen(<OutlookFrame outlook={[]} />),
};

// ── How well the week went ──────────────────────────────────────────────────

// Both blocks, each a strip of its own. The meters under "with next step",
// "multi-threaded" and "close date sound" are all read against the same open
// count, which is what makes the three comparable at a glance.
export const Scorecard: Story = {
  render: frozen(<ScorecardPanel scorecard={weeklyScorecard} />),
};

// A rep who carried no leads. The lead block is ABSENT rather than zeroed —
// zeros here would read as failure at something nobody asked of them, and that
// is the one mistake this panel must not make.
export const ScorecardDealsOnly: Story = {
  render: frozen(<ScorecardPanel scorecard={{ deal: weeklyScorecard.deal }} />),
};

// ── What the week taught ────────────────────────────────────────────────────

// All four shapes a learning comes in, each beside what it rests on. The
// citations are drawn rather than folded away: a claim about cause is the one
// thing on this page a reader cannot check against anything else on it.
export const Learnings: Story = {
  render: frozen(<LearningsPanel learnings={weeklyLearnings} />),
};

// NOBODY LOOKED, which is not the same as FOUND NOTHING. A rep whose lane was
// unbound or whose provider was down has not been told their week held no
// lesson — and the sentence says so instead of leaving the panel blank.
export const LearningsNotRun: Story = {
  render: frozen(
    <LearningsPanel learnings={{ state: "not_run", items: [] }} />,
  ),
};
