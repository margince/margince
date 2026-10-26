import type { ReactNode } from "react";
import { AiPending } from "../../design-system/aipending";
import { PanelRow } from "../../design-system/panel";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { type OpenReceipt, WithheldNotice } from "../records/record360";
import { type AccountScan, scanIsLive } from "./accountscan";
import {
  nextCommitmentLine,
  type SuggestionAction,
  useSuggestionsBody,
} from "./company360";
import type { Company360 } from "./companytoday";
import {
  briefFooter,
  manualMoveRows,
  omitted,
  scanFoot,
  TODAY_SOURCES,
} from "./companytoday.lines";
import "./company360.css";

/**
 * What needs a contact on the account today: the agent's finds and the
 * manual moves. The foot says what the list counts down to and what it could
 * not read.
 */
export type TodayReading =
  | { state: "loading" }
  | { state: "failed" }
  | {
      state: "ready";
      rows: ReactNode[];
      // False when a source the rows are read from was withheld.
      complete: boolean;
      footer?: ReactNode;
      notice?: ReactNode;
    };

// What a reading is computed from: the account's composite read and the
// verbs the surface above can perform on what it says.
type TodayReadingInputs = Readonly<{
  companyId: string;
  view?: Company360;
  loading: boolean;
  // The composite read failed. Distinct from "still loading" and from "nothing
  // is happening on this account". All three draw a short section, and only
  // one of them is a fact about the account.
  failed: boolean;
  // Opens the pre-meeting brief for the day's meeting.
  onPrepareMeeting?: (activityId: string) => void;
  // Opens the composer anchored on the account and its recipient.
  onDraftTo?: (contactId: string) => void;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Opens a cited receipt or mail in the page's drawers. A suggestion resting
  // on an unanswered mail names that mail, and reading it is the reader's move.
  onOpenReceipt?: OpenReceipt;
  onOpenEmail?: (activityId: string) => void;
  // Performing a suggestion's own action. The composer, the deal and the
  // task form all live above this brief.
  onPerform?: (action: SuggestionAction) => void;
  // The reader's scan of the account, when the page holds one. Its merged
  // advice replaces the 360's own rows, and a read in flight draws the pending
  // row. The foot says who read what and when.
  scan?: AccountScan;
  // Narrows which suggestions the list draws, for a surface that already
  // shows the rest in a panel of its own.
  keep?: Parameters<typeof useSuggestionsBody>[0]["keep"];
}>;

export function useTodayReading({
  companyId,
  view,
  loading,
  failed,
  onPrepareMeeting,
  onDraftTo,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  onPerform,
  scan,
  keep,
}: TodayReadingInputs): TodayReading {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const suggestions = useSuggestionsBody({
    companyId,
    view,
    onOpenRecord,
    onOpenReceipt,
    onOpenEmail,
    onPerform,
    keep,
    advice: scan
      ? { findings: scan.findings, dropped: scan.findings_dropped }
      : undefined,
  });
  if (loading) {
    return { state: "loading" };
  }
  if (failed || !view) {
    return { state: "failed" };
  }
  // The read in flight, above the rows it will add to. The rules' rows stand
  // while Margince reads, and the pending row says more is coming.
  const scanRows: ReactNode[] = scanIsLive(scan)
    ? [
        <PanelRow key="scan" className="co-move co-move-reading">
          <AiPending
            label={t(
              scan?.state === "running"
                ? "today.scan.reading"
                : "today.scan.queued",
            )}
            lines={2}
          />
        </PanelRow>,
      ]
    : [];
  const manual = manualMoveRows({
    view,
    t,
    onPrepareMeeting,
    onDraftTo,
    hasDraftReply: suggestions.hasDraftReply,
  });
  const commitment = nextCommitmentLine(view, locale, t);
  const rows: ReactNode[] = [...scanRows, suggestions.rows, ...manual];
  const withheld = TODAY_SOURCES.filter((source) =>
    omitted(view, source.section),
  );
  return {
    state: "ready",
    rows,
    complete: withheld.length === 0,
    footer: briefFooter(
      commitment,
      suggestions.footer,
      scanFoot({ scan, t, plural, locale, recordZone }),
    ),
    notice: (
      <WithheldNotice sections={withheld.map((source) => t(source.label))} />
    ),
  };
}
