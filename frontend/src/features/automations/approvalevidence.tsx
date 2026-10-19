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
  if (approval.kind !== "commitment_task") return null;
  const legacySource = approval.proposed_change?.source_activity_id;
  const source =
    approval.target_entity_type === "activity" && approval.target_entity_id
      ? approval.target_entity_id
      : typeof legacySource === "string"
        ? legacySource
        : null;
  if (!source) {
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
      <SourceEvidence activityId={source} onOpenTranscript={setOpened} />
      <OpenActivityDrawer
        activityId={opened}
        zone={zone}
        onClose={() => setOpened(null)}
      />
    </>
  );
}
