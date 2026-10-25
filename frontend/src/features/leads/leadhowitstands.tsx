import { EntityRef } from "../../components/entityref";
import { HowItStands } from "../../design-system/howitstands";
import { statTone } from "../../design-system/readingtone";
import type { RecordTimeline } from "../../design-system/recordtimeline";
import { StandingDims } from "../../design-system/standingdims";
import { ScoreRing } from "../../design-system/standingring";
import { StandingTimeline } from "../../design-system/standingtimeline";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { formatDayMonth } from "../../format/format";
import { leadIdentityName } from "../../format/leadname";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { addedViaFor } from "../../lib/leadorigins";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { activityMarks } from "../../lib/standingmarks";
import { useLeadStatusChanges } from "./leadhistory";
import type { Lead, LeadWriter } from "./leads";
import { LeadStepper } from "./leads.stepper";
import { useLeadScore } from "./leadscore.queries";
import { leadStanding } from "./leadstanding";
import {
  ladderDates,
  leadDims,
  leadFigures,
  leadScoreReading,
  type ReadingContext,
} from "./leadstandingparts";
import { leadTimeline } from "./leadstandingtimeline";

/**
 * How the lead stands: the verdict `leadStanding` reaches, and the ladder with
 * the day it entered each rung. Under them sit the score ring and its two rows,
 * its time on one axis and the four figures. Every part is a rule over the lead's
 * own record, so the card says "From CRM records".
 */
export function LeadHowItStands({
  lead,
  writer,
  terminalReasonId,
  thread,
  asOf,
  onQualify,
  onDisqualify,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  // The page's one sentence on why the lead takes no step, named by every
  // rung rather than drawn under each.
  terminalReasonId: string;
  // The lead's unfiltered activity page: its timeline and figures read it.
  thread: RecordTimeline;
  asOf: string;
  onQualify: () => void;
  onDisqualify: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const record = useRecordZone();
  const ctx: ReadingContext = { t, locale, zone, record, asOf };
  const changes = useLeadStatusChanges(lead);
  const explain = useLeadScore(lead);
  const verdict = leadStanding(lead, t, locale, zone, record);
  const factors =
    explain.isSuccess && explain.data.explained
      ? (explain.data.current?.factors ?? [])
      : undefined;
  const score = leadScoreReading(lead, factors, ctx);
  const timeline = leadTimeline(
    lead,
    thread.activities,
    addedViaFor(lead, t),
    ctx,
  );
  return (
    <HowItStands
      date={formatDayMonth(asOf, locale, zone)}
      verdict={{
        word: verdict.word,
        // A merged-away lead's reader wants the lead it became, which is
        // where its timeline, consent and score now live.
        sentence: lead.merged_into_id ? (
          <>
            {verdict.sentence}{" "}
            <EntityRef kind="lead" id={lead.merged_into_id} />
          </>
        ) : (
          verdict.sentence
        ),
        tone: verdict.tone,
      }}
      heroLayout="below"
      hero={
        <LeadStepper
          lead={lead}
          variant="bars"
          tone={verdict.tone}
          dates={ladderDates(lead, changes, ctx)}
          pending={writer.patch.isPending}
          reasonId={writer.readOnly ? terminalReasonId : undefined}
          onStep={(status) => {
            // One write at a time: a status sent while another save is in
            // flight races it for If-Match.
            if (!writer.patch.isPending && !writer.readOnly) {
              writer.save({ status });
            }
          }}
          onQualify={onQualify}
          onDisqualify={onDisqualify}
        />
      }
      ring={
        <ScoreRing
          score={score.score}
          tone={score.tone}
          caption={t("lead.score")}
          label={score.label}
          peek={score.peek}
        />
      }
      dims={
        <StandingDims label={t("lead.score")} dims={leadDims(factors, ctx)} />
      }
      timeline={
        <StandingTimeline
          label={t("standing.timeline.label", {
            name: leadIdentityName(lead) || t("lead.unnamed"),
          })}
          asOf={asOf}
          events={timeline.events}
          gap={timeline.gap}
          marks={activityMarks(thread.activities, {
            ...ctx,
            onOpen: () =>
              navigate({ screen: "leads", id: lead.id, id2: "history" }),
          })}
        />
      }
      figures={
        <StatStrip density="plate">
          {leadFigures(
            lead,
            thread.activities,
            ctx,
            thread.isSuccess && !thread.hasNextPage,
          ).map((figure) => (
            <StatCard
              key={figure.key}
              label={figure.label}
              value={figure.value}
              detail={figure.detail}
              tone={figure.tone ? statTone(figure.tone) : undefined}
            />
          ))}
        </StatStrip>
      }
    />
  );
}
