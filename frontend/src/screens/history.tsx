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
export { RecordHistory, useRecordHistory } from "./historyentries";
export {
  changeTimeline,
  FieldHistoryTimeline,
  useFieldHistory,
} from "./historyfields";

import { RecordHistory } from "./historyentries";
import { FieldHistoryTimeline } from "./historyfields";

const HISTORY_TABS = ["changes", "fields"] as const;
type HistoryTab = (typeof HISTORY_TABS)[number];

export function RecordHistoryTab({
  kind,
  id,
}: Readonly<{ kind: EntityKind; id: string }>) {
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
        <RecordHistory kind={kind} id={id} />
      ) : (
        <FieldHistoryTimeline kind={kind} id={id} />
      )}
    </div>
  );
}
