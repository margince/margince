// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, type ReactNode, useEffect, useState } from "react";
import { useLocale } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { blankRequired, refusalSentences } from "../lib/problem";
import { Field, TextInput } from "./atoms";
import { ConfirmModal } from "./confirmmodal";
import { useSinglePress } from "./presslatch";
import { useFieldFaults } from "./savepage";

/** A refused name write, split the way `NameDialog` draws it. */
export function nameRefusal(
  error: unknown,
  t: (key: MessageKey) => string,
  duplicate: MessageKey,
  // The code of the 409 that means the name is taken; "conflict" by default.
  code?: string,
): { problem: string | null; nameProblem: string | null } {
  const refused = refusalSentences(
    error,
    t,
    {},
    { conflict: { field: NAME_FIELD, sentence: t(duplicate), code } },
  );
  return {
    problem: refused.rest,
    nameProblem: refused.byField.get(NAME_FIELD) ?? null,
  };
}

const NAME_FIELD = "name";

// A refused name stays on its field only while the field still holds it, so
// an edit clears the refusal without a second round trip.
export function useRefusedName(
  name: string,
  nameProblem: string | null | undefined,
) {
  const [sent, setSent] = useState<string | null>(null);
  return {
    refusedName: nameProblem && name === sent ? nameProblem : undefined,
    markSent: () => setSent(name),
    forget: () => setSent(null),
  };
}

// The draft starts from `initial` on every opening, so a dialog reopened after a
// cancel never offers a name the reader already walked away from.
export function NameDialog({
  open,
  onClose,
  title,
  label,
  initial = "",
  placeholder,
  hint,
  confirmLabel,
  pending = false,
  problem,
  nameProblem,
  onSave,
  onDirtyChange,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  label: string;
  /** The name a rename starts from; saving it unchanged only closes. */
  initial?: string;
  placeholder?: string;
  /** A line under the field, such as the raw key a rename keeps (`KeyCode`). */
  hint?: ReactNode;
  confirmLabel: string;
  pending?: boolean;
  /** The refused write, already in the reader's words. */
  problem?: string | null;
  /** The refused name itself (a duplicate), drawn on the field until it changes. */
  nameProblem?: string | null;
  /** The trimmed name. The caller closes the dialog once the write lands. */
  onSave: (name: string) => void;
  onDirtyChange?: (dirty: boolean) => void;
}>) {
  const { locale } = useLocale();
  const [draft, setDraft] = useState(initial);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const name = draft.trim();
  const { refusedName, markSent, forget } = useRefusedName(name, nameProblem);
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setDraft(initial);
      forget();
      faults.show(new Map());
    }
  }
  const unsaved = name !== initial.trim();
  const dirty = open && (pending || unsaved);
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  const singlePress = useSinglePress(pending);
  // An empty name says so on the field. A refused one is already on it. An
  // unchanged one has nothing to store, so Save then only closes.
  const save = () => {
    const blank = blankRequired(locale, { [NAME_FIELD]: [label, draft] });
    if (pending || faults.show(blank)) return;
    if (refusedName) {
      faults.show(new Map([[NAME_FIELD, refusedName]]));
      return;
    }
    if (!unsaved) {
      onClose();
      return;
    }
    markSent();
    onSave(name);
  };
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={title}
      intent="form"
      confirmLabel={confirmLabel}
      bodyRef={faultsRoot}
      pending={pending}
      error={problem}
      unsaved={unsaved}
      onConfirm={save}
    >
      {/* A form of one text box, so Enter saves it. */}
      <form
        onSubmit={singlePress((event: FormEvent) => {
          event.preventDefault();
          save();
        })}
      >
        <Field
          label={label}
          labelHidden={label === title}
          hint={hint}
          error={refusedName ?? faults.of(NAME_FIELD)}
        >
          {(control) => (
            <TextInput
              {...control}
              value={draft}
              placeholder={placeholder}
              onChange={(event) => {
                setDraft(event.target.value);
                faults.edited(NAME_FIELD);
              }}
            />
          )}
        </Field>
      </form>
    </ConfirmModal>
  );
}
