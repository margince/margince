import type { ReactNode } from "react";
import { FieldRow } from "../design-system/fieldgrid";
import { InlineChoice } from "../design-system/inlinechoice";
import { InlineText } from "../design-system/inlinetext";
import { OffsiteLink } from "../design-system/offsitelink";
import { useT } from "../i18n";
import type { CreateField } from "./create";
import type { useDetailsFieldTarget } from "./pageaside";
import { selectOptions } from "./recordfieldvalues";

// One field of a record's details, read and edited in place: a choice for a
// select, a line of text for anything else.
export function RecordScalarField({
  field,
  valueRef,
  link,
  rendered,
  display,
  mark,
  label,
  values,
  editable,
  canEdit,
  reason,
  editing,
  onEditingChange,
  onDirtyChange,
  onSave,
}: Readonly<{
  field: CreateField;
  valueRef: ReturnType<typeof useDetailsFieldTarget>;
  link?: { href: string; label: string };
  rendered?: ReactNode;
  display?: string;
  mark?: ReactNode;
  label: string;
  values: Record<string, string>;
  editable: boolean;
  canEdit: boolean;
  reason?: string;
  editing: boolean;
  onEditingChange: (editing: boolean) => void;
  onDirtyChange: (dirty: boolean) => void;
  onSave: (value: string) => Promise<void>;
}>) {
  const t = useT();
  return (
    <FieldRow valueRef={valueRef} label={label}>
      {field.type === "select" ? (
        <InlineChoice
          label={label}
          hideLabel
          value={values[field.key] ?? ""}
          options={selectOptions(field, values, t)}
          render={(value) =>
            display && value === values[field.key]
              ? display
              : (selectOptions(field, values, t).find(
                  (option) => option.value === value,
                )?.label ?? value)
          }
          canEdit={canEdit}
          readOnlyReason={reason}
          onEditingChange={onEditingChange}
          onDirtyChange={onDirtyChange}
          onSave={onSave}
        />
      ) : (
        <>
          {!editing && rendered}
          {(editable || !rendered) && (
            <InlineText
              label={label}
              value={values[field.key] ?? ""}
              display={display}
              verb={Boolean(rendered)}
              placeholder={t("field.unset")}
              multiline={field.type === "textarea"}
              type={
                field.type === "number" ||
                field.type === "date" ||
                field.type === "email"
                  ? field.type
                  : "text"
              }
              maxLength={field.maxLength}
              step={field.step ?? (field.type === "number" ? "any" : undefined)}
              canEdit={canEdit}
              readOnlyReason={reason}
              onEditingChange={onEditingChange}
              onDirtyChange={onDirtyChange}
              onSave={onSave}
            />
          )}
        </>
      )}
      {link && !editing && (
        <OffsiteLink href={link.href}>{link.label}</OffsiteLink>
      )}
      {!editing && mark}
    </FieldRow>
  );
}
