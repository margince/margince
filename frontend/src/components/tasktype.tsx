// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  CalendarPlus,
  ClipboardCheck,
  type LucideIcon,
  Reply,
  Send,
  Split,
  SquareCheck,
} from "lucide-react";
import { Field } from "../design-system/atoms";
import { Chip } from "../design-system/readings";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { valueFor } from "../lib/labelfor";
import {
  isTaskType,
  TASK_TYPES,
  type TaskType,
  taskTypeLabel,
} from "../lib/tasktype";

const TASK_TYPE_ICONS: Readonly<Record<TaskType, LucideIcon>> = {
  reply: Reply,
  reach_out: Send,
  meet: CalendarPlus,
  review: ClipboardCheck,
  decide: Split,
  do: SquareCheck,
};

/** The task's type as a chip on its meta line: what kind of work it is. */
export function TaskTypeChip({ type }: Readonly<{ type: TaskType }>) {
  const t = useT();
  return (
    <Chip dense icon={valueFor(TASK_TYPE_ICONS, type, SquareCheck)}>
      {t(taskTypeLabel(type))}
    </Chip>
  );
}

/** The task form's pick of the type, which decides the task's one step. */
export function TaskTypeField({
  active,
  value,
  onChange,
}: Readonly<{
  /** Only a task has a type, so the field is left out for any other kind. */
  active: boolean;
  value: TaskType;
  onChange: (type: TaskType) => void;
}>) {
  const t = useT();
  if (!active) return null;
  return (
    <Field label={t("task.type.label")}>
      {(control) => (
        <Select
          {...control}
          options={TASK_TYPES.map((type) => ({
            value: type,
            label: t(taskTypeLabel(type)),
          }))}
          value={value}
          onChange={(picked) => onChange(isTaskType(picked) ? picked : "do")}
        />
      )}
    </Field>
  );
}
