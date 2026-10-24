import type { StandingVerdict } from "../../design-system/howitstands";
import type { StandingDim } from "../../design-system/standingdims";
import type {
  TimelineEvent,
  TimelineGap,
} from "../../design-system/standingtimeline";
import { formatMoney } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import type { Translator, Words } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ProjectHealthState } from "./projecthealth.queries";
import { phaseMoverSentence } from "./projectphasemover";
import { isProjectPhase, PHASE_LABEL } from "./projects.form";
import type { Project360 } from "./projectsections";
import {
  commitmentsReading,
  dealsReading,
  type ProjectReading,
  projectReading,
  quietStretch,
  timeUsed,
} from "./projectstanding";
import { type ProjectMoment, projectMoments } from "./projectstanding.timeline";

// The words How it stands draws for a project, from the readings in
// projectstanding.ts. Dates are drawn in the record's zone.

type Ctx = Words & Readonly<{ zone: string }>;

const STATE_WORD: Readonly<Record<ProjectHealthState, MessageKey>> = {
  on_track: "projectHealth.state.onTrack",
  at_risk: "projectHealth.state.atRisk",
  off_track: "projectHealth.state.offTrack",
};

function day(iso: string, ctx: Ctx): string {
  return formatDayMonth(iso, ctx.locale, ctx.zone);
}

export function phaseName(phase: string, t: Translator): string {
  return isProjectPhase(phase) ? t(PHASE_LABEL[phase]) : phase;
}

function assessedSentence(
  reading: Extract<ProjectReading, { kind: "assessed" }>,
  ctx: Ctx,
): string {
  if (reading.note) {
    return reading.note;
  }
  const date = day(reading.assessedAt, ctx);
  return reading.author
    ? ctx.t("project.standing.assessedBy", { date, author: reading.author })
    : ctx.t("project.standing.assessedOn", { date });
}

/** A user's reading as the card's word and sentence; none is never on track. */
export function projectVerdict(view: Project360, ctx: Ctx): StandingVerdict {
  const reading = projectReading(view);
  switch (reading.kind) {
    case "assessed":
      return {
        word: ctx.t(STATE_WORD[reading.state]),
        sentence: assessedSentence(reading, ctx),
        tone: reading.tone,
      };
    case "withheld":
      return { word: ctx.t("state.withheld"), tone: "neutral" };
    default:
      return {
        word: ctx.t("standing.notAssessed"),
        sentence: ctx.t("project.standing.unassessed"),
        tone: "neutral",
      };
  }
}

function commitmentsDim(view: Project360, ctx: Ctx): StandingDim {
  const label = ctx.t("record.commitments");
  const reading = commitmentsReading(view);
  if (!reading) {
    return {
      key: "commitments",
      label,
      value: ctx.t("state.withheld"),
      tone: "neutral",
    };
  }
  const count = String(reading.overdue);
  if (reading.overdue > 0) {
    const ofOpen = ctx.plural("standing.ofOpen", reading.open, {
      count: String(reading.open),
    });
    return {
      key: "commitments",
      label,
      value: reading.overdueIsFloor
        ? ctx.plural("project.standing.overdueFloor", reading.overdue, {
            count,
          })
        : ctx.plural("standing.overdue", reading.overdue, { count }),
      sub: reading.firstOverdue
        ? `${ofOpen} · ${reading.firstOverdue}`
        : ofOpen,
      tone: reading.tone,
    };
  }
  return {
    key: "commitments",
    label,
    value:
      reading.open > 0
        ? ctx.plural("standing.open", reading.open, {
            count: String(reading.open),
          })
        : ctx.t("project.standing.noneOpen"),
    tone: reading.tone,
  };
}

function timeDim(view: Project360, ctx: Ctx): StandingDim {
  const label = ctx.t("project.standing.timeUsed");
  const used = timeUsed(view, ctx.zone);
  if (used.kind === "running") {
    return {
      key: "time",
      label,
      value: ctx.t("project.standing.day", { day: String(used.day) }),
      sub: ctx.t("project.standing.ofDays", {
        total: String(used.total),
        date: day(used.endsOn, ctx),
      }),
      tone: used.tone,
    };
  }
  if (used.kind === "notStarted") {
    return {
      key: "time",
      label,
      value: ctx.t("project.standing.notStarted"),
      sub: ctx.t("project.standing.startsOn", {
        date: day(used.startsOn, ctx),
      }),
      tone: "neutral",
    };
  }
  return {
    key: "time",
    label,
    value: ctx.t("project.standing.noDates"),
    tone: "neutral",
  };
}

function dealsDim(view: Project360, ctx: Ctx): StandingDim {
  const label = ctx.t("project.deals.title");
  const reading = dealsReading(view);
  if (!reading) {
    return {
      key: "deals",
      label,
      value: ctx.t("state.withheld"),
      tone: "neutral",
    };
  }
  if (reading.total === 0) {
    return {
      key: "deals",
      label,
      value: ctx.t("project.standing.noDeals"),
      tone: "neutral",
    };
  }
  const counts = { won: String(reading.won), total: String(reading.total) };
  return {
    key: "deals",
    label,
    value: reading.isFloor
      ? ctx.t("project.standing.dealsWonFloor", counts)
      : ctx.t("project.standing.dealsWon", counts),
    sub: reading.openName
      ? ctx.t("project.standing.dealOpen", { name: reading.openName })
      : undefined,
    tone: reading.tone,
  };
}

/** The three rows beside the verdict: Commitments, Time used, Deals. */
export function projectDims(view: Project360, ctx: Ctx): StandingDim[] {
  return [commitmentsDim(view, ctx), timeDim(view, ctx), dealsDim(view, ctx)];
}

function momentTitle(moment: ProjectMoment, ctx: Ctx): string {
  switch (moment.what) {
    case "opened":
      return ctx.t("project.standing.event.opened", {
        phase: phaseName(moment.phase, ctx.t),
      });
    case "signed":
      return ctx.t("project.standing.event.signed");
    case "won":
      return ctx.t("project.standing.event.won", { name: moment.name });
    case "due":
      return moment.subject;
    case "update":
      return moment.subject ?? ctx.t("project.standing.lastUpdate");
    default:
      return ctx.t("project.standing.event.end");
  }
}

function momentSub(moment: ProjectMoment, ctx: Ctx): string {
  const date = day(moment.at, ctx);
  switch (moment.what) {
    case "opened":
      return `${date} · ${phaseMoverSentence(moment.by, true, ctx.t)}`;
    case "signed":
      return `${date} · ${moment.title}`;
    case "update":
      return ctx.t("project.standing.event.lastUpdate", { date });
    case "won":
      return moment.amountMinor != null && moment.currency
        ? `${date} · ${formatMoney(moment.amountMinor, moment.currency, ctx.locale)}`
        : date;
    case "due":
      return moment.kind === "late"
        ? ctx.t("project.standing.event.dueLate", { date })
        : ctx.t("project.standing.event.due", { date });
    default:
      return date;
  }
}

export function projectEvents(view: Project360, ctx: Ctx): TimelineEvent[] {
  return projectMoments(view).map((moment) => ({
    key: moment.key,
    at: moment.at,
    kind: moment.kind,
    title: momentTitle(moment, ctx),
    sub: momentSub(moment, ctx),
    // The newest update is a filed activity, so its mark is not drawn twice.
    activityIds:
      moment.what === "update"
        ? (view.activities?.data ?? [])
            .filter((row) => row.occurred_at === moment.at)
            .map((row) => row.id)
        : undefined,
  }));
}

/** The band of silence before today, when nothing was filed for a week. */
export function projectGap(
  view: Project360,
  ctx: Ctx,
): TimelineGap | undefined {
  const quiet = quietStretch(view);
  if (!quiet) {
    return undefined;
  }
  return {
    from: quiet.at,
    title: ctx.plural("standing.days", quiet.daysAgo, {
      count: String(quiet.daysAgo),
    }),
    sub: ctx.t("project.standing.gapSub"),
  };
}
