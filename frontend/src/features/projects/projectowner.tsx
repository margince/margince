// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { OwnerPick } from "../../components/ownerpick";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import type { Project } from "./projects.form";

// This pick and the edit form both write `owner_id` through updateProject, so
// the server asks either the same assignee question.
export function AssignProjectOwnerAction({
  project,
  disabledReasonId,
}: Readonly<{
  project: Project;
  disabledReasonId?: string;
}>) {
  const t = useT();
  return (
    <OwnerPick
      record={project}
      assign={async (ownerId, opened) =>
        unwrap(
          await api.PATCH("/projects/{id}", {
            params: {
              path: { id: opened.id },
              ...ifMatch(requireVersion(opened.version)),
            },
            body: { owner_id: ownerId },
          }),
        )
      }
      invalidate="projects"
      recordKey="project"
      label={t("project.assignOwner")}
      reasonId={disabledReasonId}
    />
  );
}
