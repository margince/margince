// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useState } from "react";
import type { MessageKey } from "../i18n/en";
import { problemCodeOf, problemMessageOf } from "../screens/common";
import { Field, TextInput } from "./atoms";
import { ConfirmModal } from "./confirmmodal";
import { useSinglePress } from "./presslatch";

/** A refused name write, split the way `NameDialog` draws it. */
export function nameRefusal(
  error: unknown,
  t: (key: MessageKey) => string,
  duplicate: MessageKey,
): { problem: string | null; nameProblem: string | null } {
  if (error === null || error === undefined) {
    return { problem: null, nameProblem: null };
  }
  return problemCodeOf(error) === "conflict"
    ? { problem: null, nameProblem: t(duplicate) }
    : { problem: problemMessageOf(error, t), nameProblem: null };
}

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
  confirmLabel,
  pending = false,
  problem,
  nameProblem,
  onSave,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  label: string;
  /** The name a rename starts from; an unchanged name cannot be saved. */
  initial?: string;
  placeholder?: string;
  confirmLabel: string;
  pending?: boolean;
  /** The refused write, already in the reader's words. */
  problem?: string | null;
  /** The refused name itself (a duplicate), drawn on the field until it changes. */
  nameProblem?: string | null;
  /** The trimmed name. The caller closes the dialog once the write lands. */
  onSave: (name: string) => void;
}>) {
  const [draft, setDraft] = useState(initial);
  const name = draft.trim();
  const { refusedName, markSent, forget } = useRefusedName(name, nameProblem);
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setDraft(initial);
      forget();
    }
  }
  const singlePress = useSinglePress(pending);
  const ready = name !== "" && name !== initial.trim() && !refusedName;
  const save = () => {
    if (!ready || pending) return;
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
      confirmDisabled={!ready}
      pending={pending}
      error={problem}
      onConfirm={save}
    >
      {/* A form of one text box, so Enter saves it. */}
      <form
        onSubmit={singlePress((event: FormEvent) => {
          event.preventDefault();
          save();
        })}
      >
        <Field label={label} error={refusedName}>
          {(control) => (
            <TextInput
              {...control}
              value={draft}
              placeholder={placeholder}
              onChange={(event) => setDraft(event.target.value)}
            />
          )}
        </Field>
      </form>
    </ConfirmModal>
  );
}
