// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { EmptyState } from "../design-system/atoms";
import { FactList } from "../design-system/factlist";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { ProblemError, type QueryLike } from "./common";
import { HealthCard } from "./healthcard";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The shell three System health cards wear, with the body reduced to the least
// that can stand in for one: what is worth looking at here is the four
// decisions the shell makes, and a real reading would put its own layout in
// front of every one of them.
//
// The four, in the order the stories below take them: the reading and its
// stamp; an empty report drawn by the BODY rather than by the gate, because on
// these cards "nothing is queued" is the finding; the withheld notice, which
// keeps the card's place instead of removing it; and the read that failed
// inside the card's own boundary.
//
// The copy is the job-health card's, which is the first caller — the shell
// carries no words of its own, so a story of it has to borrow a real card's.

type Report = Readonly<{ readAt: string; waiting: number; retrying: number }>;

const BUSY: Report = {
  readAt: "13 Aug 2026, 09:30",
  waiting: 12,
  retrying: 2,
};
const IDLE: Report = { readAt: "13 Aug 2026, 09:30", waiting: 0, retrying: 0 };

// A hand-built QueryLike, not a routed fetch: the card takes the query as a
// prop, so its four states are four VALUES. Producing each one through a stub
// would exercise react-query and leave the shell's own branching to luck.
function queryState(partial: Partial<QueryLike<Report>>): QueryLike<Report> {
  return {
    isPending: false,
    isError: false,
    error: null,
    data: undefined,
    // Inert: there is no reading behind this story to fetch again. The retry is
    // on screen because the failed state draws one, and pressing it is not what
    // the story is about.
    refetch: () => null,
    ...partial,
  };
}

function Shell({
  canSee,
  query,
}: Readonly<{ canSee: boolean; query: QueryLike<Report> }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <HealthCard
      title={t("settings.jobs")}
      sub={t("settings.jobsSub")}
      withheld={t("jobs.adminOnly")}
      canSee={canSee}
      query={query}
      footer={(report) => t("jobs.generatedAt", { time: report.readAt })}
    >
      {(report) =>
        report.waiting === 0 && report.retrying === 0 ? (
          <EmptyState>{t("jobs.workspaceEmpty")}</EmptyState>
        ) : (
          <FactList
            facts={[
              {
                key: "waiting",
                term: t("jobs.workspaceKinds"),
                value: t("jobs.count.waiting", {
                  count: formatNumber(report.waiting, locale),
                }),
              },
              {
                key: "retrying",
                term: t("jobs.failures"),
                value: t("jobs.count.retrying", {
                  count: formatNumber(report.retrying, locale),
                }),
              },
            ]}
          />
        )
      }
    </HealthCard>
  );
}

// Every story serves /me: the shell probes the session before it will state a
// denial, so one that routed nothing would sit on the probe's pending body and
// none of the four states below would ever be drawn.
function story(canSee: boolean, query: QueryLike<Report>) {
  return () => {
    installFetchStub({
      "GET /me": canSee
        ? meRoute({ job_health: ["read"] })
        : meRoute({}, { roles: ["rep"], seat: "read" }),
    });
    return (
      <StoryProviders>
        <Shell canSee={canSee} query={query} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof HealthCard> = {
  title: "Settings/Governance/System health/Health card shell",
  component: HealthCard,
};
export default meta;

type Story = StoryObj<typeof HealthCard>;

/** A reading, with the stamp under it saying when it was taken. */
export const Reading: Story = {
  render: story(true, queryState({ data: BUSY })),
};

/**
 * An empty report, which is a finding rather than a gap: the queue is idle.
 *
 * The shell passes no `empty` predicate to its gate for exactly this — the
 * generic "nothing here" would understate the one thing the card exists to
 * say — so the sentence comes from the body and the stamp still dates it.
 */
export const NothingQueued: Story = {
  render: story(true, queryState({ data: IDLE })),
};

/**
 * A reader without the grant. The card keeps its place and says so, because an
 * absent card on a page this reader opens for its other sections would read as
 * "nothing is queued" — a claim about the installation nobody made.
 *
 * No footer: the query's cache outlives a grant, so a stamp under a withheld
 * body would date a reading the card is no longer showing.
 */
export const Withheld: Story = {
  render: story(false, queryState({ data: BUSY })),
};

/**
 * The read failed, in the server's own words and inside the card's boundary.
 *
 * These bodies derive every line from a payload a background system writes, so
 * they have more ways to give out than the panels beside them; without the
 * boundary the whole tab would go with one of them.
 */
export const ReadingFailed: Story = {
  render: story(
    true,
    queryState({
      isError: true,
      error: new ProblemError({
        detail: "the job-health report could not be assembled",
      }),
    }),
  ),
};

/** The wait, labelled with what is being fetched rather than a bare spinner. */
export const StillReading: Story = {
  render: story(true, queryState({ isPending: true })),
};

// The reading in dark, where the panel's header band, the footer rule and the
// FactList hairline are all re-derived from tokens a step apart: the three have
// to stay three different weights once the whole plate darkens under them.
export const ReadingDark: Story = {
  globals: { theme: "dark" },
  render: story(true, queryState({ data: BUSY })),
};
