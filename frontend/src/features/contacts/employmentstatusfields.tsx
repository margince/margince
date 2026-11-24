import { Field, TextInput } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { type Locale, type Translator, useT } from "../../i18n";
import { fieldCheckSentence } from "../../lib/problem";
import { validDateEntry } from "./employmentpatch";

/** A status the reader picked; empty until one is, for a link imported as unknown. */
export type EmploymentStatusChoice = "current" | "former" | "";

/**
 * Whether the job is still held, and when it ended: the pair every employment
 * dialog asks for. One component, so adding a job and correcting one offer the
 * same two answers in the same words.
 */
export function EmploymentStatusFields({
  status,
  onStatusChange,
  end,
  onEndChange,
  disabled,
  statusPlaceholder,
  statusError,
  endError,
}: Readonly<{
  status: EmploymentStatusChoice;
  onStatusChange: (status: "current" | "former") => void;
  end: string;
  onEndChange: (end: string) => void;
  disabled?: boolean;
  statusPlaceholder?: string;
  statusError?: string;
  endError?: string;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("employment.statusLabel")} error={statusError}>
        {(control) => (
          <Select
            {...control}
            value={status}
            placeholder={statusPlaceholder ?? t("employment.statusChoose")}
            onChange={(value) => {
              if (value === "current" || value === "former")
                onStatusChange(value);
            }}
            options={[
              { value: "current", label: t("employment.status.current") },
              { value: "former", label: t("employment.status.former") },
            ]}
            disabled={disabled}
          />
        )}
      </Field>
      <Field
        label={t("employment.end")}
        hint={t("employment.dateHint")}
        error={endError}
      >
        {(control) => (
          <TextInput
            {...control}
            value={end}
            disabled={disabled}
            onChange={(event) => onEndChange(event.target.value)}
          />
        )}
      </Field>
    </>
  );
}

// A start or end typed in neither shape the hint names. Keyed as the dialogs
// key their fields, so a press says which date to fix.
export function employmentDateFaults(
  locale: Locale,
  t: Translator,
  dates: Readonly<{ start: string; end: string }>,
): Map<string, string> {
  const faults = new Map<string, string>();
  if (!validDateEntry(dates.start)) {
    faults.set(
      "start",
      fieldCheckSentence("invalid_date", locale, t("employment.start")),
    );
  }
  if (!validDateEntry(dates.end)) {
    faults.set(
      "end",
      fieldCheckSentence("invalid_date", locale, t("employment.end")),
    );
  }
  return faults;
}
