// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { RecordTeam } from "../../components/recordteam";
import { useT } from "../../i18n";
import { ProjectHistory } from "./project360.chronology";
import { ProjectCompanies } from "./projectcompanies";
import { ProjectDocuments } from "./projectdocuments";
import { ProjectDrawer } from "./projectdrawer";
import { type Project360, StakeholdersCard } from "./projectsections";

// The drawers the page opens; "tasks" is the history narrowed to tasks.
export type ProjectDrawerKind = "history" | "tasks" | "documents" | "contacts";

// The history drawer's title, kept past closing so it does not change while
// the drawer animates out.
function useHistoryDrawerTitle(drawer: ProjectDrawerKind | null): string {
  const t = useT();
  const [kind, setKind] = useState<"history" | "tasks">("history");
  if ((drawer === "history" || drawer === "tasks") && drawer !== kind) {
    setKind(drawer);
  }
  return kind === "tasks"
    ? t("project.commitments.all")
    : t("record.fullHistory");
}

/** The project page's reading drawers, one open at a time. */
export function ProjectDrawers({
  view,
  drawer,
  readOnly,
  onClose,
}: Readonly<{
  view: Project360;
  drawer: ProjectDrawerKind | null;
  readOnly: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const historyTitle = useHistoryDrawerTitle(drawer);
  const project = view.project;
  return (
    <>
      <ProjectDrawer
        open={drawer === "history" || drawer === "tasks"}
        title={historyTitle}
        onClose={onClose}
      >
        <ProjectHistory
          view={view}
          kind={drawer === "tasks" ? "task" : undefined}
        />
      </ProjectDrawer>
      <ProjectDrawer
        open={drawer === "documents"}
        title={t("project.documents.title")}
        onClose={onClose}
      >
        <ProjectDocuments view={view} readOnly={readOnly} />
      </ProjectDrawer>
      <ProjectDrawer
        open={drawer === "contacts"}
        title={t("project.more.contacts")}
        onClose={onClose}
      >
        <div className="project-drawer-stack">
          <ProjectCompanies
            projectId={project.id}
            companies={project.companies}
            readOnly={readOnly}
          />
          <RecordTeam
            recordType="project"
            recordId={project.id}
            readOnly={readOnly}
          />
          <StakeholdersCard
            view={view}
            projectId={project.id}
            readOnly={readOnly}
          />
        </div>
      </ProjectDrawer>
    </>
  );
}
