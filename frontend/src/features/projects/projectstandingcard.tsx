import type { ReactNode } from "react";
import { HowItStands } from "../../design-system/howitstands";
import { StandingDims } from "../../design-system/standingdims";
import { StandingTimeline } from "../../design-system/standingtimeline";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { PROJECT_PHASES } from "../../format/projectphase";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { reveal } from "../../lib/reveal";
import { activityMarks } from "../../lib/standingmarks";
import { PhaseStepper } from "./projectphase";
import { PROJECT_COMMITMENTS_ANCHOR, ProjectFigures } from "./projectreadings";
import { isProjectPhase, type ProjectPhase } from "./projects.form";
import type { Project360 } from "./projectsections";
import { phaseTimes } from "./projectstanding";
import { commitmentsPeek, dealsPeek, timePeek } from "./projectstanding.peeks";
import {
  projectDims,
  projectEvents,
  projectGap,
  projectVerdict,
} from "./projectstanding.words";

// How it stands on a project. It draws a user's health reading as the word, the
// phase ladder under it, three readings beside it, the dated axis and the
// figures. Nothing here is model-written, so the sources pill reads from CRM
// records.

// The days under each rung ("9 days", "18 days · now") and a dash for a rung
// the project has not reached. Closed is a way out, not a rung.
function phaseSubs(
  view: Project360,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): Partial<Record<ProjectPhase, ReactNode>> {
  const subs: Partial<Record<ProjectPhase, ReactNode>> = {};
  for (const phase of PROJECT_PHASES) {
    if (phase !== "closed") {
      subs[phase] = "—";
    }
  }
  for (const [phase, time] of phaseTimes(view)) {
    if (!isProjectPhase(phase)) {
      continue;
    }
    const count = { count: formatNumber(time.days, locale) };
    subs[phase] = plural(
      time.current ? "standing.daysNow" : "standing.days",
      time.days,
      count,
    );
  }
  return subs;
}

export function ProjectStanding({
  view,
  refusedReasonId,
  onMove,
  onOpenDocuments,
}: Readonly<{
  view: Project360;
  refusedReasonId?: string;
  onMove: (to: ProjectPhase) => void;
  onOpenDocuments: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const ctx = { t, plural, locale, zone };
  const peeks = {
    commitments: commitmentsPeek(view, ctx, reveal(PROJECT_COMMITMENTS_ANCHOR)),
    deals: dealsPeek(view, ctx),
  };
  return (
    <HowItStands
      date={formatDayMonth(view.as_of, locale, zone)}
      verdict={projectVerdict(view, ctx)}
      heroLayout="below"
      hero={
        <PhaseStepper
          variant="bars"
          phase={view.project.phase}
          subs={phaseSubs(view, plural, locale)}
          refusedReasonId={refusedReasonId}
          pending={false}
          onMove={onMove}
        />
      }
      dims={
        <StandingDims
          label={t("project.standing.dims")}
          dims={projectDims(view, ctx).map((dim) => ({
            ...dim,
            peek:
              dim.key === "time"
                ? timePeek(view, ctx)
                : peeks[dim.key as keyof typeof peeks],
          }))}
        />
      }
      timeline={
        <StandingTimeline
          label={t("standing.timeline.label", {
            name: view.project.name,
          })}
          asOf={view.as_of}
          record={zone}
          gap={projectGap(view, ctx)}
          events={projectEvents(view, ctx)}
          marks={activityMarks(view.activities?.data ?? [], {
            ...ctx,
            record: zone,
            asOf: view.as_of,
          })}
        />
      }
      figures={<ProjectFigures view={view} onOpenDocuments={onOpenDocuments} />}
    />
  );
}
