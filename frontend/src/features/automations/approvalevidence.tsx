import { useState } from "react";
import type { DecisionApproval } from "../../design-system/decisioncard";
import { SourceEvidence } from "../../design-system/sourceevidence";
import { SurfaceState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";

export function ApprovalEvidence({
  approval,
}: Readonly<{ approval: DecisionApproval }>) {
  const t = useT();
  const zone = useRecordZone();
  const [opened, setOpened] = useState<string | null>(null);
  const legacySource = approval.proposed_change?.source_activity_id;
  const source =
    approval.target_entity_type === "activity" && approval.target_entity_id
      ? approval.target_entity_id
      : typeof legacySource === "string"
        ? legacySource
        : null;
  const sources = approval.preview?.evidence
    ? [
        ...new Set(
          approval.preview.evidence.flatMap((item) =>
            item.source_type === "activity" && item.source_id
              ? [item.source_id]
              : [],
          ),
        ),
      ]
    : approval.kind === "commitment_task" && source
      ? [source]
      : [];
  if (sources.length === 0 && approval.kind !== "commitment_task") return null;
  if (sources.length === 0) {
    return (
      <SurfaceState
        label={t("stat.evidence")}
        state="empty"
        emptyLabel={t("trust.sourceUnknown")}
        loadingLabel={t("tasks.detailLoading")}
      >
        {null}
      </SurfaceState>
    );
  }
  return (
    <>
      {sources.map((id) => (
        <SourceEvidence key={id} activityId={id} onOpenTranscript={setOpened} />
      ))}
      <OpenActivityDrawer
        activityId={opened}
        zone={zone}
        onClose={() => setOpened(null)}
      />
    </>
  );
}
