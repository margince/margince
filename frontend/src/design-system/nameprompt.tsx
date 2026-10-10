// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { Button } from "./atoms";
import { NameDialog } from "./namedialog";

// A `NameDialog` behind its own trigger, for a surface whose only verb opens it.
export function NamePrompt({
  trigger,
  icon,
  title,
  label,
  confirmLabel,
  pending = false,
  problem,
  onSave,
}: Readonly<{
  /** The button that opens it, already translated. */
  trigger: string;
  /** A lucide glyph naming the write, ahead of the trigger's words. */
  icon?: ReactNode;
  title: string;
  label: string;
  confirmLabel: string;
  pending?: boolean;
  /** The failure, already read into the reader's language. */
  problem?: string;
  /** Run `done` once the write has landed; a refused write keeps the dialog open. */
  onSave: (name: string, done: () => void) => void;
}>) {
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        {icon}
        {trigger}
      </Button>
      <NameDialog
        open={open}
        onClose={close}
        title={title}
        label={label}
        confirmLabel={confirmLabel}
        pending={pending}
        problem={problem}
        onSave={(name) => onSave(name, close)}
      />
    </>
  );
}
