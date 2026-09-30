// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Create task" over a selection: say what has to be done, by when and by whom,
// and the bulk dialog previews filing that task under every selected record
// before anything is written.

import { useState } from "react";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DateInput, type ISODate, isISODate } from "../design-system/dateinput";
import { Select } from "../design-system/select";
import { endOfDayInZone, viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import { useRoster } from "./entityref";

type BulkTask = components["schemas"]["BulkTask"];

export function TaskVerb({
  disabled,
  onReady,
}: Readonly<{
  disabled: boolean;
  /** The task, once the reader has said what it is. */
  onReady: (task: BulkTask) => void;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [subject, setSubject] = useState("");
  const [due, setDue] = useState<ISODate | "">("");
  const [assigneeId, setAssigneeId] = useState("");
  const roster = useRoster("user", open);
  const start = () => {
    setSubject("");
    setDue("");
    setAssigneeId("");
    setOpen(true);
  };
  return (
    <>
      <Button disabled={disabled} onClick={start}>
        {t("bulk.createTask")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("bulk.taskTitle")}
        confirmLabel={t("bulk.taskNext")}
        confirmDisabled={subject.trim() === ""}
        onConfirm={() => {
          setOpen(false);
          onReady({
            subject: subject.trim(),
            due_at: due === "" ? undefined : endOfDayInZone(due, viewerZone()),
            assignee_id: assigneeId === "" ? undefined : assigneeId,
          });
        }}
      >
        <Field label={t("bulk.taskSubject")} required>
          {(control) => (
            <TextInput
              {...control}
              value={subject}
              maxLength={500}
              onChange={(event) => setSubject(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("bulk.taskDue")}>
          {(control) => (
            <DateInput
              {...control}
              value={due}
              onChange={(event) =>
                setDue(isISODate(event.target.value) ? event.target.value : "")
              }
            />
          )}
        </Field>
        <Field label={t("bulk.taskAssignee")}>
          {(control) => (
            <Select
              {...control}
              value={assigneeId}
              placeholder={t("bulk.taskAssigneeMe")}
              onChange={setAssigneeId}
              options={(roster.data ?? []).map((entry) => ({
                value: entry.id,
                label: "display_name" in entry ? entry.display_name : entry.id,
              }))}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}
