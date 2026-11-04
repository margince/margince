import { Checkbox, Field, TextInput } from "../design-system/atoms";
import { DateInput, isoOrBlank } from "../design-system/dateinput";
import { clockOrBlank, TimeInput } from "../design-system/timeinput";
import { dueInstant } from "../format/calendarday";
import { formatTimeOfDay } from "../format/format";
import { useDateTimePreferences } from "../format/preferences";
import { formatClockTime, timePattern } from "../format/timepattern";
import { wallClockTime } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { TaskDraft } from "../lib/activitybody";
import { useRecordZone } from "../lib/recordzone";
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
              ? { subject: "", body: "", day: "", time: "", assigneeId: "" }
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
          <TaskTimeField
            day={value.day}
            value={value.time}
            onChange={(time) => patch({ time })}
          />
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

/**
 * A task's optional callback time, beside its due date. It waits for a day,
 * because a time with no day names nothing. Cleared, the task is due at the
 * end of its day.
 *
 * A time the record's clock skips that day (02:30 when the clocks go forward)
 * is due at the first minute after the gap. The field says so under the box,
 * so the rep sees the time the task will carry rather than finding it later.
 */
export function TaskTimeField({
  day,
  value,
  onChange,
  label,
}: Readonly<{
  day: string;
  value: string;
  onChange: (time: string) => void;
  label?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const pattern = timePattern(locale, useDateTimePreferences().timeFormat);
  const lands = landedTime(day, value, zone);
  return (
    <Field
      label={label ?? t("log.dueTime")}
      hint={
        lands &&
        t("log.dueTimeSkipped", {
          time: formatClockTime(
            clockOrBlank(value) || "00:00",
            locale,
            pattern,
          ),
          actual: formatTimeOfDay(lands, locale, zone),
        })
      }
    >
      {(control) => (
        <TimeInput
          {...control}
          value={clockOrBlank(value)}
          disabled={day === ""}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}

/**
 * The instant a picked time lands on when the record's clock does not read
 * that time on that day, or undefined when it does.
 */
function landedTime(
  day: string,
  time: string,
  zone: string,
): string | undefined {
  if (day === "" || time === "") {
    return undefined;
  }
  let at: string;
  try {
    at = dueInstant(day, zone, time);
  } catch {
    return undefined;
  }
  return wallClockTime(Date.parse(at), zone) === time ? undefined : at;
}
