// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type FormEvent,
  type KeyboardEvent,
  type ReactNode,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { useT } from "../i18n";
import { isImeCommit } from "../lib/imecommit";
import { problemMessageOf } from "../lib/problem";
import { Button, Field, type FieldControl } from "./atoms";
import { refuseUnreadableField } from "./formvalidity";
import { RecordFormDialog } from "./recordformdialog";

export const FIELD_COMMIT_KEY = "Enter";

/** A paragraph commits on ⌘/Ctrl+Enter, because Enter there is a newline. */
export function isParagraphCommit(
  event: Pick<KeyboardEvent, "key" | "metaKey" | "ctrlKey">,
): boolean {
  return event.key === FIELD_COMMIT_KEY && (event.metaKey || event.ctrlKey);
}

/** What the dialog hands the one control it holds. */
export type FieldEditControl = Readonly<{
  control: FieldControl;
  draft: string;
  setDraft: (next: string) => void;
  /** A save is out: the control holds the caret and refuses the keystroke. */
  saving: boolean;
}>;

export type FieldEditDialogProps = Readonly<{
  open: boolean;
  /** The field's name: the dialog's title, and the control's label. */
  label: string;
  /** The stored value. Each opening starts the draft from it. */
  value: string;
  hint?: ReactNode;
  onClose: () => void;
  /** Resolves once stored; a throw is the refusal, shown on the field. */
  onSave: (next: string) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
  /** What a save sends and compares with `value`, such as a trimmed line. */
  settle?: (draft: string) => string;
  children: (field: FieldEditControl) => ReactNode;
}>;

const asTyped = (draft: string) => draft;

/**
 * Whether one field's editor is open. A field that turns read-only while it is
 * open closes it, and the caller hears that close like any other.
 */
export function useFieldEditor(
  canEdit: boolean,
  onEditingChange?: (editing: boolean) => void,
) {
  const [open, setOpen] = useState(false);
  const reported = useRef(false);
  if (open && !canEdit) {
    setOpen(false);
  }
  const editing = (next: boolean) => {
    reported.current = next;
    setOpen(next);
    onEditingChange?.(next);
  };
  useEffect(() => {
    if (canEdit || !reported.current) return;
    reported.current = false;
    onEditingChange?.(false);
  }, [canEdit, onEditingChange]);
  return { open: open && canEdit, editing };
}

export function FieldEditDialog({
  open,
  label,
  value,
  hint,
  onClose,
  onSave,
  onDirtyChange,
  settle = asTyped,
  children,
}: FieldEditDialogProps) {
  const t = useT();
  const formId = useId();
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const [openedOn, setOpenedOn] = useState(false);
  if (open !== openedOn) {
    setOpenedOn(open);
    if (open) {
      setDraft(value);
      setFailure(null);
    }
  }
  // Set in the press, so an Enter that lands before `saving` renders is refused.
  const inFlight = useRef(false);
  const dirty = open && (saving || settle(draft) !== value);
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  const save = async () => {
    const next = settle(draft);
    // Storing what is already stored writes an audit row for no change.
    if (next === value) {
      onClose();
      return;
    }
    inFlight.current = true;
    setSaving(true);
    setFailure(null);
    try {
      await onSave(next);
      onClose();
    } catch (err) {
      setFailure(problemMessageOf(err, t));
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  };
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (inFlight.current || refuseUnreadableField(event.currentTarget)) return;
    void save();
  };
  const paragraphCommit = (event: KeyboardEvent<HTMLFormElement>) => {
    if (
      event.target instanceof HTMLTextAreaElement &&
      !isImeCommit(event) &&
      isParagraphCommit(event)
    ) {
      event.preventDefault();
      event.currentTarget.requestSubmit();
    }
  };

  return (
    <RecordFormDialog
      open={open}
      title={label}
      onClose={onClose}
      fieldCount={1}
      unsaved={dirty && !saving}
      closeDisabled={saving}
      form={
        <form
          id={formId}
          className="form-stack"
          onSubmit={submit}
          onKeyDown={paragraphCommit}
        >
          <Field
            label={label}
            labelHidden
            hint={hint}
            error={failure ?? undefined}
          >
            {(control) => children({ control, draft, setDraft, saving })}
          </Field>
        </form>
      }
      actions={
        <>
          <Button type="button" onClick={onClose} disabled={saving}>
            {t("create.cancel")}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            pending={saving}
            busyLabel={t("common.saving")}
          >
            {t("record.save")}
          </Button>
        </>
      }
    />
  );
}
