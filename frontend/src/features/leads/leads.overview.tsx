import type { RecordTimeline } from "../../design-system/recordtimeline";
import {
  LeadEnquiryCard,
  LeadKnowCard,
  LeadQualifyPreviewCard,
} from "./leadcards";
import { LeadHowItStands } from "./leadhowitstands";
import { LeadNextBar } from "./leadnext";
import type { Lead, LeadWriter } from "./leads";
import type { PromotionRecord } from "./leads.promotion";
import { LeadScoreCard } from "./leads.score";

export function LeadOverviewPane({
  lead,
  id,
  writer,
  promotion,
  terminalReasonId,
  thread,
  asOf,
  onOpenEmail,
  onQualify,
  onDisqualify,
}: Readonly<{
  lead: Lead;
  id: string;
  writer: LeadWriter;
  promotion: PromotionRecord;
  terminalReasonId: string;
  // The lead's unfiltered activity page. A filter is a view of the History
  // tab, and a reading over a narrowed page would be false.
  thread: RecordTimeline;
  // The instant the page read the lead at; every reading here is made at it.
  asOf: string;
  onOpenEmail: (activityId: string) => void;
  onQualify: () => void;
  onDisqualify: () => void;
}>) {
  return (
    <>
      {!lead.archived_at && (
        <LeadNextBar
          lead={lead}
          activities={thread.activities}
          asOf={asOf}
          refusedReasonId={writer.readOnly ? terminalReasonId : undefined}
          onQualify={onQualify}
        />
      )}
      <LeadHowItStands
        lead={lead}
        writer={writer}
        terminalReasonId={terminalReasonId}
        thread={thread}
        asOf={asOf}
        onQualify={onQualify}
        onDisqualify={onDisqualify}
      />
      <div className="lead-cards">
        <div className="lead-cards-stack">
          <LeadScoreCard
            lead={lead}
            writer={writer}
            terminalReasonId={terminalReasonId}
          />
          <LeadKnowCard
            id={id}
            readOnlyReason={writer.readOnlyReason}
            reasonId={writer.readOnly ? terminalReasonId : undefined}
          />
        </div>
        <div className="lead-cards-stack">
          <LeadEnquiryCard thread={thread} onOpenEmail={onOpenEmail} />
          <LeadQualifyPreviewCard lead={lead} promotion={promotion} />
        </div>
      </div>
    </>
  );
}
