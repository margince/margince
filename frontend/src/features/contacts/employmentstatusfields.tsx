import { Field, TextInput } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";

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
}: Readonly<{
  status: EmploymentStatusChoice;
  onStatusChange: (status: "current" | "former") => void;
  end: string;
  onEndChange: (end: string) => void;
  disabled?: boolean;
  statusPlaceholder?: string;
}>) {
  const t = useT();
  return (
    <>
      <Field label={t("employment.statusLabel")}>
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
      <Field label={t("employment.end")} hint={t("employment.dateHint")}>
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
