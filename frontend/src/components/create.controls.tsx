import { type ReactNode, useState } from "react";
import {
  Button,
  Field,
  type FieldControl,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { DateInput, isoOrBlank } from "../design-system/dateinput";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import {
  MultiSelect,
  Select,
  type SelectOption,
} from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type CreateField,
  fieldLabel,
  fieldOptions,
  type SubField,
} from "./create.fields";
import {
  joinMultiselectValue,
  splitMultiselectValue,
} from "./create.multiselect";

// The control half of a field row. The wrapping `Field` owns the label, id and
// described-by seam, so the wiring arrives whole as `control`. The picked
// candidate is held here: a searched record is usually on no page the form
// loaded, so `options` alone would name it by its id.
function ReferenceControl({
  field,
  searchTargets,
  control,
  value,
  setValue,
  t,
}: Readonly<{
  field: CreateField;
  searchTargets: NonNullable<CreateField["searchTargets"]>;
  control: FieldControl;
  value: string;
  setValue: (next: string) => void;
  t: (key: MessageKey) => string;
}>) {
  const [picked, setPicked] = useState<RecordPickerCandidate | null>(null);
  // Forgotten once the value moves off it. A reset that later lands on the
  // same id is then named by the current options, not by a stale pick.
  if (picked && picked.id !== value) {
    setPicked(null);
  }
  const selected: RecordPickerCandidate | null = !value
    ? null
    : picked?.id === value
      ? picked
      : {
          id: value,
          name:
            field.options?.find((option) => option.value === value)?.label ??
            value,
        };
  return (
    <>
      <RecordPicker
        id={control.id}
        aria-describedby={control["aria-describedby"]}
        aria-invalid={control["aria-invalid"]}
        searchTargets={searchTargets}
        selected={selected}
        onPick={(candidate) => {
          setPicked(candidate);
          setValue(candidate.id);
        }}
      />
      {value && !field.required && (
        <Button variant="link" onClick={() => setValue("")}>
          {t("field.unset")}
        </Button>
      )}
    </>
  );
}

export function fieldControl(
  field: CreateField | SubField,
  control: FieldControl,
  value: string,
  setValue: (next: string) => void,
  t: (key: MessageKey) => string,
  // The form's current values, for a field whose choices depend on them.
  values: Record<string, string> = {},
): ReactNode {
  if ("searchTargets" in field && field.searchTargets)
    return (
      <ReferenceControl
        field={field}
        searchTargets={field.searchTargets}
        control={control}
        value={value}
        setValue={setValue}
        t={t}
      />
    );
  if (field.type === "select") {
    // Optional fields can be cleared; preserve a caller's existing empty choice.
    const options =
      "optionsFor" in field
        ? fieldOptions(field, values)
        : (field.options ?? []);
    const clearable = options.some((option) => option.value === "");
    const blank: SelectOption[] =
      field.required || clearable
        ? []
        : [{ value: "", label: t("field.unset") }];
    return (
      <Select
        {...control}
        value={value}
        onChange={setValue}
        options={[...blank, ...options]}
        // Nothing to choose from yet: the answer this list depends on has
        // not been given.
        disabled={
          "optionsFor" in field &&
          Boolean(field.optionsFor) &&
          options.length === 0
        }
      />
    );
  }
  if (field.type === "textarea") {
    return (
      <Textarea
        {...control}
        value={value}
        placeholder={field.placeholder}
        rows={3}
        maxLength={field.maxLength}
        onChange={(event) => setValue(event.target.value)}
      />
    );
  }
  if (field.type === "date") {
    return (
      <DateInput
        {...control}
        value={isoOrBlank(value)}
        onChange={(event) => setValue(event.target.value)}
      />
    );
  }
  return (
    <TextInput
      {...control}
      type={field.type ?? "text"}
      // A bare number input steps by 1, so the browser refuses 14.60 before
      // any handler sees it. A money field has to say it takes cents.
      step={field.step}
      min={field.min}
      maxLength={field.maxLength}
      value={value}
      placeholder={field.placeholder}
      onChange={(event) => setValue(event.target.value)}
    />
  );
}

// A multiselect field writes its set back through the scalar string channel
// (`joinMultiselectValue`).
export function MultiselectField({
  field,
  value,
  setValue,
}: Readonly<{
  field: CreateField;
  value: string;
  setValue: (next: string) => void;
}>) {
  const t = useT();
  return (
    <Field
      label={fieldLabel(field, t)}
      required={field.required}
      hint={field.hint}
    >
      {(control) => (
        <MultiSelect
          {...control}
          options={field.options ?? []}
          values={splitMultiselectValue(value, field.multiselectEncoding)}
          onChange={(next) =>
            setValue(joinMultiselectValue(next, field.multiselectEncoding))
          }
          placeholder={t("field.unset")}
        />
      )}
    </Field>
  );
}
