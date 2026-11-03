import { type ReactNode, useState } from "react";
import { useT } from "../i18n";
import { Textarea, TextInput } from "./atoms";
import { DateInput, isoOrBlank } from "./dateinput";
import { EditableValue } from "./editablevalue";
import { type FieldEditControl, FieldEditDialog } from "./fieldeditdialog";
import "./inlinechoice.css";

// The field for a reader who cannot edit it. `placeholder` is written for
// someone about to press it ("Add legal name"), so it never stands in here:
// a suggested value does, muted, and failing that `field.unset`.
function ReadOnlyText({
  value,
  display,
  suggested,
  readOnlyReason,
}: Readonly<{
  value: string;
  display?: string;
  suggested?: string;
  readOnlyReason?: string;
}>) {
  const t = useT();
  return (
    <span
      className={
        !value && suggested ? "inlinetext inlinetext-suggested" : "inlinetext"
      }
      title={readOnlyReason}
    >
      {(value && display) || value || suggested || t("field.unset")}
    </span>
  );
}

type TextKind = "text" | "email" | "number" | "date";

function InlineTextControl({
  field,
  multiline,
  type,
  step,
  maxLength,
  suggested,
}: Readonly<{
  field: FieldEditControl;
  multiline?: boolean;
  type?: TextKind;
  step?: string;
  maxLength?: number;
  suggested?: string;
}>) {
  const { control, draft, setDraft, saving } = field;
  if (multiline) {
    return (
      <Textarea
        {...control}
        rows={4}
        value={draft}
        maxLength={maxLength}
        placeholder={suggested}
        readOnly={saving}
        aria-busy={saving || undefined}
        onChange={(event) => setDraft(event.target.value)}
      />
    );
  }
  if (type === "date") {
    return (
      <DateInput
        {...control}
        value={isoOrBlank(draft)}
        readOnly={saving}
        onChange={(event) => setDraft(event.target.value)}
      />
    );
  }
  return (
    <TextInput
      {...control}
      type={type}
      step={step}
      value={draft}
      maxLength={maxLength}
      placeholder={suggested}
      // Not `disabled`: that drops the field from the tab order mid-save.
      readOnly={saving}
      aria-busy={saving || undefined}
      onChange={(event) => setDraft(event.target.value)}
    />
  );
}

const asTyped = (draft: string) => draft;
const asLine = (draft: string) => draft.trim();

export function InlineText({
  label,
  value,
  display,
  link,
  placeholder,
  suggested,
  maxLength,
  multiline,
  type,
  step,
  frame,
  onEditingChange,
  onDirtyChange,
  canEdit,
  readOnlyReason,
  onSave,
}: Readonly<{
  label: string;
  value: string;
  // The resting reading of `value` when it is not the text the editor edits:
  // a currency in its own code over the bare major-unit amount.
  display?: string;
  // The value drawn as something to follow (an address, a profile). It keeps
  // its own action; the pill's padding and the bubble open the editor.
  link?: ReactNode;
  placeholder: string;
  // A value the record carries elsewhere that stands in for this field until
  // one is written here (a contact's title, for the role at their current
  // employer). Shown muted at rest and as the editor's placeholder, since
  // "Unset" would deny a fact the record does hold.
  suggested?: string;
  maxLength?: number;
  type?: TextKind;
  step?: string;
  // Holds the resting value in the element its text belongs to, such as a
  // record's h1.
  frame?: (resting: ReactNode) => ReactNode;
  // A paragraph: Enter is a newline, Cmd/Ctrl+Enter saves, and the text is
  // stored as typed rather than trimmed.
  multiline?: boolean;
  // Fires as the editor opens and closes, so a caller can pin the record
  // version the edit started from.
  onEditingChange?: (editing: boolean) => void;
  onDirtyChange?: (dirty: boolean) => void;
  canEdit: boolean;
  readOnlyReason?: string;
  // Resolves once stored; a throw is the refusal, read by `problemMessageOf`.
  onSave: (next: string) => Promise<void>;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const held = frame ?? ((resting: ReactNode) => resting);
  const editing = (next: boolean) => {
    setOpen(next);
    onEditingChange?.(next);
  };
  const linked = Boolean(link);
  const reading = linked
    ? link
    : (value && display) ||
      value ||
      (suggested && <span className="inlinetext-suggested">{suggested}</span>);
  return (
    <>
      {held(
        canEdit ? (
          <EditableValue
            editLabel={t("inlineChoice.change", { field: label })}
            placeholder={linked ? undefined : placeholder}
            link={linked}
            onEdit={() => editing(true)}
          >
            {reading}
          </EditableValue>
        ) : linked ? (
          link
        ) : (
          <ReadOnlyText
            value={value}
            display={display}
            suggested={suggested}
            readOnlyReason={readOnlyReason}
          />
        ),
      )}
      <FieldEditDialog
        open={open}
        label={label}
        value={value}
        settle={multiline ? asTyped : asLine}
        onClose={() => editing(false)}
        onSave={onSave}
        onDirtyChange={onDirtyChange}
      >
        {(field) => (
          <InlineTextControl
            field={field}
            multiline={multiline}
            type={type}
            step={step}
            maxLength={maxLength}
            suggested={suggested}
          />
        )}
      </FieldEditDialog>
    </>
  );
}
