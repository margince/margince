// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef, useState } from "react";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import {
  ListPopover,
  type ListPopoverOption,
} from "../design-system/listpopover";
import { useT } from "../i18n";
import { isVersionSkewOf, problemMessageOf, throwProblem } from "./common";
import { useUpdateRecord } from "./edit";
import type { Project } from "./projects.form";
import { RosterPartialNote, useRoster, useRosterPartial } from "./roster";

// Hands a project directly to a named colleague. The Owner select beside
// this (projects.form.ts) only ever offers keep-current/Me/Unassign; naming
// anyone else has no path from the project's own screen otherwise. The
// server already takes any workspace member in `owner_id` (updateProject is
// its own bulk transfer's "per-project twin"), so this is transport the
// contract already supports.
//
// Picking IS the act, as on an assignee picker: the write is one pick away
// from being walked back, so a confirm step would only add a press.
export function AssignProjectOwnerAction({
  project,
  disabledReasonId,
}: Readonly<{
  project: Project;
  disabledReasonId?: string;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // The shared roster rather than a search, so the list opens with names.
  const roster = useRoster("user", open);
  const partial = useRosterPartial("user", open);
  // The toast lands after the popover closes, when the roster may be gone.
  const pickedName = useRef("");

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
    // Named off the pick, never a re-read: the write sends only that option's id.
    savedMessage: () =>
      t("project.assignOwnerDone", { name: pickedName.current }),
    onDone: () => setOpen(false),
  });

  const options = roster.data?.flatMap((entry): ListPopoverOption[] =>
    "display_name" in entry
      ? [{ id: entry.id, name: entry.display_name, keywords: [entry.email] }]
      : [],
  );
  const skew = isVersionSkewOf(mutation.error);
  const errorMessage = mutation.isError
    ? skew
      ? t("edit.versionSkew")
      : problemMessageOf(mutation.error, t)
    : undefined;

  return (
    <ListPopover
      label={t("project.assignOwner")}
      title={t("project.assignOwnerTitle")}
      searchLabel={t("project.assignOwnerSearch")}
      reasonId={disabledReasonId}
      open={open}
      onOpenChange={(next) => {
        // Held open while the write is out, so its refusal has somewhere to land.
        if (!next && mutation.isPending) {
          return;
        }
        setOpen(next);
        // A refusal belongs to the attempt it answered, not to the next opening.
        if (!mutation.isPending) {
          mutation.reset();
        }
      }}
      options={roster.isError ? [] : options}
      empty={roster.isError ? problemMessageOf(roster.error, t) : undefined}
      selected={project.owner_id ?? undefined}
      onPick={(option, done) => {
        if (option.id === project.owner_id) {
          done();
          return;
        }
        pickedName.current = option.name;
        mutation.mutate({ values: { owner_id: option.id }, rows: {} });
      }}
      pending={mutation.isPending}
      error={errorMessage}
      footer={<RosterPartialNote partial={partial} />}
    />
  );
}
