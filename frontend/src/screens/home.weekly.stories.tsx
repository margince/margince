// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import {
  firstWeek,
  narratedWeek,
  PRIOR_WEEK_START,
  WEEK_START,
} from "./home.fixtures";
import { WeeklySection } from "./home.weekly";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The week just gone, drawn on its own.
//
// `home.stories.tsx` documents the whole morning, and the weekly reaches a
// reader there only through whichever view the dial is on — so the panel's own
// states had no frame of their own. They are worth one, because what this panel
// draws differently is a question of what the review SAYS, and three of those
// answers are SILENCES a reader has to be able to tell apart: nobody narrated
// the week, a pass ran and found it unremarkable, and there is no review at
// all. Two of them are the same blank space unless the panel says which.
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

const meta: Meta<typeof WeeklySection> = {
  title: "Shell/Home weekly review",
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
