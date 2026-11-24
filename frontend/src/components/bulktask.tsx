// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Create task" over a selection: say what has to be done, by when and by whom,
// and the bulk dialog previews filing that task under every selected record
// before anything is written.

import { useState } from "react";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { DateInput, type ISODate } from "../design-system/dateinput";
import { useFieldFaults } from "../design-system/savepage";
import { Select } from "../design-system/select";
import { dueDayBounds } from "../format/calendarday";
import { useLocale, useT } from "../i18n";
import { taskDueFields } from "../lib/activitybody";
import { blankRequired } from "../lib/problem";
import { useViewerZone } from "../lib/viewerzone";
import { useAssignableUserOptions } from "./assigneepicker";
import { useRosterHint, useRosterPartial } from "./entityref";

type BulkTask = components["schemas"]["BulkTask"];

export function TaskVerb({
  disabled,
  reasonId,
  onReady,
}: Readonly<{
  disabled: boolean;
  /** The bar's sentence saying why filing a task is refused, when it is. */
  reasonId?: string;
  /** The task, once the reader has said what it is. */
  onReady: (task: BulkTask) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const [subject, setSubject] = useState("");
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const subjectLabel = t("bulk.taskSubject");
  const [due, setDue] = useState<ISODate | "">("");
  const [assigneeId, setAssigneeId] = useState("");
  const readerZone = useViewerZone();
  const { options, roster } = useAssignableUserOptions(open);
  const rosterHint = useRosterHint(roster, useRosterPartial("user", open));
  const start = () => {
    setSubject("");
    setDue("");
    setAssigneeId("");
    faults.show(new Map());
    setOpen(true);
  };
  return (
    <>
      <Button disabled={disabled} reasonId={reasonId} onClick={start}>
        {t("bulk.createTask")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("bulk.taskTitle")}
        intent="form"
        confirmLabel={t("bulk.taskNext")}
        bodyRef={faultsRoot}
        onConfirm={() => {
          const blank = blankRequired(locale, {
            subject: [subjectLabel, subject],
          });
          if (faults.show(blank)) return;
          setOpen(false);
          onReady({
            subject: subject.trim(),
            ...(due === "" ? {} : taskDueFields(due, "", readerZone)),
            assignee_id: assigneeId === "" ? undefined : assigneeId,
          });
        }}
      >
        <Field label={subjectLabel} required error={faults.of("subject")}>
          {(control) => (
            <TextInput
              {...control}
              value={subject}
              maxLength={500}
              onChange={(event) => {
                setSubject(event.target.value);
                faults.edited("subject");
              }}
            />
          )}
        </Field>
        <Field label={t("bulk.taskDue")}>
          {(control) => (
            <DateInput
              {...control}
              value={due}
              {...dueDayBounds(readerZone)}
              onChange={(event) => setDue(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("bulk.taskAssignee")} hint={rosterHint}>
          {(control) => (
            <Select
              {...control}
              value={assigneeId}
              placeholder={t("log.assigneeMe")}
              onChange={setAssigneeId}
              options={options}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}
