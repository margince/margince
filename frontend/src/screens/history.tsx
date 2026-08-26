import { useState } from "react";
import type { EntityKind } from "../app/entity";
import { SegmentedControl } from "../design-system/atoms";
import { useT } from "../i18n";

// The record-level entry point (B-EP09.x): a SegmentedControl toggling
// between the plain-language change list and the per-field diff timeline —
// two projections of the same audit spine, never fetched simultaneously.
//
// The two panels live in their own files; this one owns the strip and which
// panel it shows. Their hooks and the timeline projection are re-exported
// here because "./history" is the name every record screen already reaches
// for, and a split that renamed the door would touch six screens to say
// nothing.
export {
  RecordHistory,
  type RecordRestore,
  useRecordHistory,
} from "./historyentries";
export {
  changeTimeline,
  FieldHistoryTimeline,
  useFieldHistory,
} from "./historyfields";

import { RecordHistory, type RecordRestore } from "./historyentries";
import { FieldHistoryTimeline } from "./historyfields";

// One vocabulary, whatever record the panel is opened on: `changes` is
// everything that happened in order, each entry carrying what it did; `fields`
// is the same changes grouped by the field they touched. They name the SHAPE
// of the list rather than its subject, which is what keeps a deal, a company
// and a person from describing one panel three ways.
// The pair names the SHAPE of the same history: one row per change, or one
// row per field. Neither is called "Changes" on its own — the timeline that
// embeds this panel already offers a "Changes" filter, and one word meaning
// two things on one screen is how a reader learns to distrust both.
const HISTORY_TABS = ["changes", "fields"] as const;
type HistoryTab = (typeof HISTORY_TABS)[number];

export function RecordHistoryTab({
  kind,
  id,
  currency,
  restore,
}: Readonly<{
  kind: EntityKind;
  id: string;
  // Handed down to the field panel, which needs it to read a minor-unit
  // amount at the scale its currency actually has.
  currency?: string | null;
  // What a change put back needs from the record: the version it pins the
  // write against, and the re-read that follows. Absent on a surface holding
  // neither, where the panel reads the history and offers no verb.
  restore?: RecordRestore;
}>) {
  const t = useT();
  const [tab, setTab] = useState<HistoryTab>("changes");
  const tabLabels: Record<HistoryTab, string> = {
    changes: t("history.tabChanges"),
    fields: t("history.tabFields"),
  };

  return (
    // The strip and the panel are siblings, which is what makes this a stack:
    // switching tabs mounts a new panel while the strip is the same DOM node, so
    // the panel arrives and the control the reader just pressed stays still.
    <div className="arrive-stack">
      <div className="filter-tabs">
        <SegmentedControl
          options={HISTORY_TABS}
          value={tab}
          onChange={setTab}
          labels={tabLabels}
        />
      </div>
      {tab === "changes" ? (
        <RecordHistory
          kind={kind}
          id={id}
          currency={currency}
          restore={restore}
        />
      ) : (
        <FieldHistoryTimeline kind={kind} id={id} currency={currency} />
      )}
    </div>
  );
}
