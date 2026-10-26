import { Checkbox, Field, TextInput } from "../design-system/atoms";
import { DateInput, isoOrBlank } from "../design-system/dateinput";
import { useT } from "../i18n";
import type { TaskDraft } from "../lib/activitybody";
import { ActivityBodyField } from "./activitybodyfield";
import { TaskAssigneeField } from "./assigneepicker";

export function FollowUpTaskFields({
  value,
  onChange,
}: Readonly<{
  value: TaskDraft | undefined;
  onChange: (value: TaskDraft | undefined) => void;
}>) {
  const t = useT();
  const patch = (change: Partial<TaskDraft>) => {
    if (value) onChange({ ...value, ...change });
  };
  return (
    <>
      <Checkbox
        label={t("log.addFollowUp")}
        checked={value !== undefined}
        onChange={(event) =>
          onChange(
            event.target.checked
              ? { subject: "", body: "", day: "", assigneeId: "" }
              : undefined,
          )
        }
      />
      {value && (
        <>
          <Field label={t("log.followUpSubject")} required>
            {(control) => (
              <TextInput
                {...control}
                value={value.subject}
                onChange={(event) => patch({ subject: event.target.value })}
              />
            )}
          </Field>
          <Field label={t("log.dueAt")}>
            {(control) => (
              <DateInput
                {...control}
                value={isoOrBlank(value.day)}
                onChange={(event) => patch({ day: event.target.value })}
              />
            )}
          </Field>
          <TaskAssigneeField
            active
            value={value.assigneeId}
            onChange={(assigneeId) => patch({ assigneeId })}
          />
          <ActivityBodyField
            label={t("log.body")}
            transcript={false}
            rows={3}
            value={value.body}
            onChange={(body) => patch({ body })}
          />
        </>
      )}
    </>
  );
}
