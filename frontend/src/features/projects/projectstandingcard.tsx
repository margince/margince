import type { ReactNode } from "react";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { HowItStands } from "../../design-system/howitstands";
import { ProvenanceTag } from "../../design-system/provenance";
import { useRecordTimeline } from "../../design-system/recordtimeline";
import { StandingDims } from "../../design-system/standingdims";
import { StandingTimeline } from "../../design-system/standingtimeline";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { PROJECT_PHASES } from "../../format/projectphase";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { useCompanyZone } from "../../lib/companyzone";
import { useSourceOpener } from "../../lib/opensource";
import { reveal } from "../../lib/reveal";
import { latestWord } from "../../lib/standinglatest";
import { withStandingLine } from "../../lib/standingline";
import { activityMarks } from "../../lib/standingmarks";
import { useViewerZone } from "../../lib/viewerzone";
import { useUndoProjectHealth } from "./projecthealth.queries";
import { PhaseStepper } from "./projectphase";
import { PROJECT_COMMITMENTS_ANCHOR, ProjectFigures } from "./projectreadings";
import { isProjectPhase, type ProjectPhase } from "./projects.form";
import type { Project360 } from "./projectsections";
import { phaseTimes, projectReading } from "./projectstanding";
import { commitmentsPeek, dealsPeek, timePeek } from "./projectstanding.peeks";
import {
  projectDims,
  projectEvents,
  projectGap,
  projectVerdict,
} from "./projectstanding.words";

// How it stands on a project. It draws the health reading as the word, the
// phase ladder under it, three readings beside it, the dated axis and the
// figures. A reading the health job wrote carries the agent tag, its reason and
// an undo. That job judges from filed activity and tasks, so the sources pill
// still reads from CRM records; a colleague's reading names its author instead.

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

// The health job's reading: the agent tag, the one-sentence reason, and the
// undo that puts back what stood before it.
function AIHealthSentence({
  projectId,
  assessmentId,
  sentence,
  refusedReasonId,
}: Readonly<{
  projectId: string;
  assessmentId: string;
  sentence: ReactNode;
  refusedReasonId?: string;
}>) {
  const t = useT();
  const undo = useUndoProjectHealth(projectId);
  return (
    <span>
      <ProvenanceTag provenance={{ kind: "agent", job: "project_health" }} />{" "}
      {sentence}{" "}
      <Button
        variant="link"
        reasonId={refusedReasonId}
        pending={undo.isPending}
        onClick={() => undo.mutate(assessmentId)}
      >
        {t("projectHealth.undoAI")}
      </Button>
      <ErrorLine inline error={undo.error} />
    </span>
  );
}

export function ProjectStanding({
  view,
  refusedReasonId,
  onMove,
  onOpenDocuments,
  onOpenHistory,
}: Readonly<{
  view: Project360;
  refusedReasonId?: string;
  onMove: (to: ProjectPhase) => void;
  onOpenDocuments: () => void;
  // Where a mark's title goes: the project's history.
  onOpenHistory?: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const ctx = { t, plural, locale, zone, companyZone: useCompanyZone() };
  // The 360's page seeds the line; a wider zoom pages older rows from the list.
  const history = useRecordTimeline("project", view.project.id, {
    firstPage: view.activities,
    enabled: Boolean(view.activities),
  });
  const peeks = {
    commitments: commitmentsPeek(view, ctx, reveal(PROJECT_COMMITMENTS_ANCHOR)),
    deals: dealsPeek(view, ctx),
  };
  const reading = projectReading(view);
  const aiHealth = reading.kind === "assessed" && reading.byAI;
  // The health job's own sentence keeps its undo, so the line stands aside.
  const openSource = useSourceOpener();
  const line = withStandingLine(
    projectVerdict(view, ctx),
    aiHealth ? undefined : view.standing_line,
    { rows: history.activities, t, locale, zone, open: openSource },
  );
  const verdict = line.verdict;
  return (
    <HowItStands
      date={formatDayMonth(view.as_of, locale, zone)}
      sources={
        line.sources ??
        (reading.kind === "assessed" && !reading.byAI
          ? { by: "colleague" }
          : { by: "records" })
      }
      verdict={
        aiHealth
          ? {
              ...verdict,
              sentence: (
                <AIHealthSentence
                  projectId={view.project.id}
                  assessmentId={reading.id}
                  sentence={verdict.sentence}
                  refusedReasonId={refusedReasonId}
                />
              ),
            }
          : verdict
      }
      citations={line.citations}
      latest={latestWord(history.activities, {
        ...ctx,
        record: zone,
        asOf: view.as_of,
        onOpen: onOpenHistory,
      })}
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
          history={history}
          marks={activityMarks(history.activities, {
            ...ctx,
            record: zone,
            asOf: view.as_of,
            onOpen: onOpenHistory,
          })}
        />
      }
      figures={<ProjectFigures view={view} onOpenDocuments={onOpenDocuments} />}
    />
  );
}
