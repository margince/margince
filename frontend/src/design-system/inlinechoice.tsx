import { type ReactNode, useState } from "react";
import { useT } from "../i18n";
import { EditableValue } from "./editablevalue";
import { FieldEditDialog } from "./fieldeditdialog";
import "./inlinechoice.css";
import { Select, type SelectOption } from "./select";

// One value a reader may choose, edited in a dialog that saves on Save.
// A pick that writes at once leaves no way to back out.
export function InlineChoice({
  label,
  hideLabel,
  value,
  options,
  canEdit,
  readOnlyReason,
  hint,
  render,
  onSave,
  onEditingChange,
  onDirtyChange,
  testId,
}: Readonly<{
  // Names the field: the dialog's title and the trigger's verb.
  label: string;
  // Drops the visible "label: " prefix where the layout already prints the
  // field's name (FieldGrid's label column); the accessible name keeps it.
  hideLabel?: boolean;
  // Fires as the editor opens and closes, so a caller can pin the record
  // version the edit started from.
  onEditingChange?: (editing: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
  value: string;
  options: readonly SelectOption[];
  canEdit: boolean;
  readOnlyReason?: string;
  // What the dialog says about its options, such as a roster that failed.
  hint?: string;
  render: (value: string) => ReactNode;
  // Resolves once stored; a throw is the refusal, shown on the field with
  // the reader's choice kept.
  onSave: (next: string) => Promise<void>;
  // Test id for the value at rest.
  testId?: string;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const editing = (next: boolean) => {
    setOpen(next);
    onEditingChange?.(next);
  };
  // An unset choice still says so: a blank read as a value that failed to load.
  const shown = value ? render(value) : t("field.unset");
  return (
    <span>
      {!hideLabel && <>{label}: </>}
      {canEdit ? (
        <EditableValue
          editLabel={t("inlineChoice.change", { field: label })}
          placeholder={t("field.unset")}
          testId={testId}
          onEdit={() => editing(true)}
        >
          {value ? shown : null}
        </EditableValue>
      ) : (
        <span
          className={value ? undefined : "inlinechoice-unset"}
          title={readOnlyReason}
          data-testid={testId}
        >
          {shown}
        </span>
      )}
      <FieldEditDialog
        open={open}
        label={label}
        value={value}
        hint={hint}
        onClose={() => editing(false)}
        onSave={onSave}
        onDirtyChange={onDirtyChange}
      >
        {({ control, draft, setDraft, saving }) => (
          <Select
            {...control}
            value={draft}
            options={options}
            // Disabled stops a second pick over a write in flight; busy says
            // it is working, not refused.
            disabled={saving}
            aria-busy={saving || undefined}
            onChange={setDraft}
          />
        )}
      </FieldEditDialog>
    </span>
  );
}
