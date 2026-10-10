// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Field, TextInput } from "./atoms";
import { ConfirmModal } from "./confirmmodal";

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
  const [sent, setSent] = useState<string | null>(null);
  const [wasOpen, setWasOpen] = useState(open);
  if (wasOpen !== open) {
    setWasOpen(open);
    if (open) {
      setDraft(initial);
      setSent(null);
    }
  }
  const name = draft.trim();
  const refusedName = nameProblem && name === sent ? nameProblem : undefined;
  const ready = name !== "" && name !== initial.trim() && !refusedName;
  const save = () => {
    if (!ready || pending) return;
    setSent(name);
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
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
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
