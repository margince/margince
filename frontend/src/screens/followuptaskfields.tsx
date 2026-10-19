import { TaskAssigneeField } from "../components/assigneepicker";
import { Checkbox, Field, TextInput } from "../design-system/atoms";
import type { TaskDraft } from "../features/records/activitybody";
import { ActivityBodyField } from "../features/records/activitybodyfield";
import { useT } from "../i18n";

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
              <TextInput
                {...control}
                type="date"
                value={value.day}
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
