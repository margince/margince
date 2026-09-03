// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { useT } from "../i18n";
import { isVersionSkewOf, problemMessageOf, throwProblem } from "./common";
import { useUpdateRecord } from "./edit";
import type { Project } from "./projects.form";

// Hands a project directly to a named colleague. The Owner select beside
// this (projects.form.ts) only ever offers keep-current/Me/Unassign; naming
// anyone else has no path from the project's own screen otherwise. The
// server already takes any workspace member in `owner_id` (updateProject is
// its own bulk transfer's "per-project twin"), so this is transport the
// contract already supports, reached through the standing RecordPicker
// search→pick pattern rather than a new one.

// `q` is a server-side filter (not a client-walked page like a deal search),
// so one page covers every ordinary search — but the endpoint still caps a
// page at 50 by default. Asking for the contract's own maximum instead means
// a common name in a large workspace does not quietly lose matches past the
// default.
export async function searchColleagues(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/users", {
    params: { query: { q, limit: 200 } },
  });
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

  const skew = isVersionSkewOf(mutation.error);
  const errorMessage = mutation.isError
    ? skew
      ? t("edit.versionSkew")
      : problemMessageOf(mutation.error, t)
    : null;

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
      <ConfirmModal
        open={open}
        onClose={() => {
          setOpen(false);
          setPicked(null);
          // A prior failure's error is this action's own transient state, not
          // a fact about the project — closing the dialog on it, however, is
          // not the same as it being addressed. Reset so reopening starts
          // clean rather than showing a refusal from an attempt nobody has
          // repeated yet.
          mutation.reset();
        }}
        title={t("project.assignOwnerTitle")}
        // The verb, not "Confirm": the last control before a write says which
        // write it is, and `deals.confirm` names no outcome at all.
        confirmLabel={t("project.assignOwnerConfirm")}
        confirmReason={
          picked ? undefined : t("project.assignOwnerNoneSelected")
        }
        onConfirm={() =>
          picked &&
          mutation.mutate({ values: { owner_id: picked.id }, rows: {} })
        }
        pending={mutation.isPending}
        error={errorMessage}
      >
        <RecordPicker
          label={t("project.assignOwnerSearch")}
          searchTargets={searchColleagues}
          selected={picked}
          onPick={setPicked}
          disabled={mutation.isPending}
        />
      </ConfirmModal>
    </>
  );
}
