// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { Button, Modal } from "../design-system/atoms";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { useUpdateRecord } from "./edit";
import type { Project } from "./projects.form";

// Hands a project directly to a named colleague. The Owner select beside
// this (projects.form.ts) only ever offers keep-current/Me/Unassign; naming
// anyone else has no path from the project's own screen otherwise. The
// server already takes any workspace member in `owner_id` (updateProject is
// its own bulk transfer's "per-project twin"), so this is transport the
// contract already supports, reached through the standing RecordPicker
// search→pick pattern rather than a new one.

export async function searchColleagues(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/users", { params: { query: { q } } });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((user) => ({ id: user.id, name: user.display_name }));
}

export function AssignProjectOwnerAction({
  project,
  disabledReasonId,
}: Readonly<{
  project: Project;
  disabledReasonId?: string;
}>) {
  const t = useT();
  const titleId = useId();
  const [open, setOpen] = useState(false);
  const [picked, setPicked] = useState<RecordPickerCandidate | null>(null);

  const mutation = useUpdateRecord<Project>({
    update: async (values) => {
      const { data, error } = await api.PATCH("/projects/{id}", {
        params: {
          path: { id: project.id },
          ...ifMatch(requireVersion(project.version)),
        },
        body: { owner_id: values.owner_id as string },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    invalidate: "projects",
    recordKey: "project",
    recordId: project.id,
    savedMessage: t("project.assignOwnerDone", { name: picked?.name ?? "" }),
    onDone: () => {
      setOpen(false);
      setPicked(null);
    },
  });

  return (
    <>
      <Button
        reasonId={disabledReasonId}
        small
        data-testid="assign-project-owner"
        onClick={() => setOpen(true)}
      >
        {t("project.assignOwner")}
      </Button>
      <Modal
        open={open}
        onClose={() => {
          setOpen(false);
          setPicked(null);
        }}
        labelledBy={titleId}
      >
        <h2 id={titleId} className="t-h2">
          {t("project.assignOwnerTitle")}
        </h2>
        <RecordPicker
          label={t("project.assignOwnerSearch")}
          searchTargets={searchColleagues}
          selected={picked}
          onPick={setPicked}
          disabled={mutation.isPending}
        />
        <div className="actions">
          <Button
            onClick={() => {
              setOpen(false);
              setPicked(null);
            }}
          >
            {t("deals.cancel")}
          </Button>
          <Button
            variant="primary"
            reason={picked ? undefined : t("project.assignOwnerNoneSelected")}
            pending={mutation.isPending}
            onClick={() =>
              picked &&
              mutation.mutate({ values: { owner_id: picked.id }, rows: {} })
            }
          >
            {t("deals.confirm")}
          </Button>
        </div>
      </Modal>
    </>
  );
}
