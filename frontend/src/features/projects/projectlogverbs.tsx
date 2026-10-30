// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { FileText } from "lucide-react";
import type { ActivityGate } from "../../components/activitygate";
import { LogActivityAction } from "../../components/logactivity.action";
import { Button } from "../../design-system/atoms";
import { VerbWord } from "../../design-system/iconaction";
import { useT } from "../../i18n";

export type LogDrawer = "log" | "task" | null;

/**
 * Log, and the one drawer it shares with the menu's Add task, opened on the
 * kind the verb asked for. A verb a reader may not use still shows, pointing
 * at the sentence that says why, stated here because the menu mounts late.
 */
export function ProjectLogVerbs({
  projectId,
  refusedReasonId,
  activity,
  drawer,
  onDrawer,
}: Readonly<{
  projectId: string;
  refusedReasonId?: string;
  activity: ActivityGate;
  drawer: LogDrawer;
  onDrawer: (next: LogDrawer) => void;
}>) {
  const t = useT();
  return (
    <>
      {!refusedReasonId && activity.refusedId && (
        <p className="t-caption" id={activity.sentenceId}>
          {t("record.logActivityRefused")}
        </p>
      )}
      <Button
        disabled={activity.pending}
        reasonId={refusedReasonId ?? activity.refusedId}
        title={t("log.title")}
        onClick={() => onDrawer("log")}
      >
        <FileText aria-hidden="true" />
        <VerbWord word={t("record.verb.log")} label={t("log.title")} />
      </Button>
      {drawer && (
        <LogActivityAction
          entityType="project"
          entityId={projectId}
          askedKind={drawer === "task" ? "task" : undefined}
          openOnMount
          onClose={() => onDrawer(null)}
        />
      )}
    </>
  );
}
