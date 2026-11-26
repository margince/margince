// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ComponentType } from "react";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { RecordView } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import { problemStatus } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import { ProjectKeyChip } from "./projects";
import { PHASE_LABEL, type Project } from "./projects.form";

/**
 * The records feature's full history with its restore, which the shell passes
 * in because a feature draws no other feature's code.
 */
export type ArchivedHistory = ComponentType<
  Readonly<{ kind: "project"; id: string; version: Project["version"] }>
>;

// Drawn from the row alone, because the server refuses an archived project's
// 360. History stays: it is the one place the archive is undone.
export function ArchivedProjectPage({
  project,
  history: History,
}: Readonly<{ project: Project; history?: ArchivedHistory }>) {
  const t = useT();
  const readerZone = useViewerZone();
  return (
    <RecordView
      name={project.name}
      identity={project.id}
      zone={readerZone}
      pulse={
        <>
          <Badge tone="warning">{t("record.archived")}</Badge>
          <Badge>{t(labelFor(PHASE_LABEL, project.phase))}</Badge>
          {project.key && <ProjectKeyChip projectKey={project.key} dense />}
        </>
      }
      band={<p>{t("project.archivedReadOnly")}</p>}
    >
      {History && (
        <History kind="project" id={project.id} version={project.version} />
      )}
    </RecordView>
  );
}

// A 404 is no such project in this reader's scope; a 422 is an address that
// names no project at all. Neither is helped by a retry.
export function isMissingProject(error: unknown): boolean {
  const status = problemStatus(error);
  return status === 404 || status === 422;
}

export function ProjectNotFound() {
  const t = useT();
  return (
    <EmptyState
      title={t("project.page.notFound")}
      action={
        <Button onClick={() => navigate({ screen: "projects" })}>
          {t("project.page.backToProjects")}
        </Button>
      }
    >
      {t("project.page.notFoundBody")}
    </EmptyState>
  );
}
