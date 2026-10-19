import { Panel, PanelBody } from "../../design-system/panel";
import type { RecordTimeline } from "../../design-system/recordtimeline";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import {
  RecordReading,
  TimelineThread,
  TodayPanel,
} from "../records/record360";
import { RecordListsPanel } from "../records/recordlists";
import { LeadBrief } from "./leadbrief";
import { MergedLeadPanel } from "./leadmerged";
import { LeadReadings } from "./leadreadings";
import type { Lead, LeadWriter } from "./leads";
import { PromotedLeadPanel, type PromotionRecord } from "./leads.promotion";
import { LeadScoreCard } from "./leads.score";
import { LeadManualSignals } from "./leadsignals";
import { leadStanding } from "./leadstanding";
import { leadTodoRows } from "./leadtoday";

/**
 * The call on a lead: where it stands, in a word, as a fact the server already
 * decided. The lead's own thread sits under it: what was said, the silence
 * since, what is dated ahead.
 */
function LeadCall({
  lead,
  thread,
  onOpenEmail,
}: Readonly<{
  lead: Lead;
  thread: RecordTimeline;
  // The page's one drawer, handed down to the thread. A conversation the
  // thread names and cannot open is the one place in the product that does
  // that.
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const standing = leadStanding(lead, t, locale, viewerZone());
  return (
    <LeadBrief
      standing={{ label: standing.label, tone: standing.tone }}
      because={standing.because}
      restsOn={standing.restsOn}
    >
      <TimelineThread thread={thread} onOpenEmail={onOpenEmail} />
    </LeadBrief>
  );
}

export function LeadOverviewPane({
  lead,
  id,
  writer,
  promotion,
  terminalReasonId,
  thread,
  onOpenEmail,
}: Readonly<{
  lead: Lead;
  id: string;
  writer: LeadWriter;
  promotion: PromotionRecord;
  terminalReasonId: string;
  // The lead's unfiltered timeline read, which the thread under the call is
  // drawn from. It is the whole read, so its failure reaches the call too.
  thread: RecordTimeline;
  // The page's one email drawer, for the thread under the call.
  onOpenEmail: (activityId: string) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // The lead carries no task id of its own. So the panel head's way out and
  // the "Next task" row's own verb both open the same queue.
  const onOpenTasks = () => navigate({ screen: "worklist" });
  return (
    <div className="record-stack">
      {/* The readings open the overview, as they do on every record page. */}
      <LeadReadings lead={lead} />
      {/* A merged-away lead's page leads with where it went, for the same
          reason: the reader arrived asking what happened to this prospect. */}
      {lead.merged_into_id && (
        <MergedLeadPanel mergedIntoId={lead.merged_into_id} />
      )}
      {/* A promoted lead's page leads with what the promotion did. The reader
          arrived asking whether this became a contact, and which one. */}
      {lead.promoted_contact_id && (
        <PromotedLeadPanel lead={lead} promotion={promotion} />
      )}
      {/* One reading, in parts, the shape every record page reads in. The call
          with the lead's own thread under it comes first, then what needs a
          contact. Under them sit two sections a reader consults rather than
          reads: why it scores what it scores, and what the rep knows. */}
      <RecordReading>
        <LeadCall lead={lead} thread={thread} onOpenEmail={onOpenEmail} />
        <TodayPanel onOpenTasks={onOpenTasks} tasksLabel={t("today.workQueue")}>
          {leadTodoRows(lead, t, locale, recordZone, onOpenTasks)}
        </TodayPanel>
        {/* Full width, in sequence, rather than side by side (RecordReadingPair):
            the score card is one line and the signals form is tall, and a
            short card beside a long one reads as a layout that ran out of
            content rather than as two readings a rep consults in order. */}
        <LeadScoreCard
          lead={lead}
          id={id}
          writer={writer}
          terminalReasonId={terminalReasonId}
        />
        <Panel title={t("lead.signalsTitle")}>
          <PanelBody>
            <LeadManualSignals
              // Keyed by lead: a half-typed input for one lead must not be
              // submitted against the next one the reader navigates to.
              key={id}
              id={id}
              readOnlyReason={writer.readOnlyReason}
            />
          </PanelBody>
        </Panel>
        {/* The server answers no lists for an archived lead. */}
        {!lead.archived_at && (
          <RecordListsPanel entityType="lead" entityId={id} />
        )}
      </RecordReading>
    </div>
  );
}
