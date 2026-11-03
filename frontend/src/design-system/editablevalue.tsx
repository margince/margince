// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pencil } from "lucide-react";
import { type MouseEvent, type ReactNode, useId, useRef } from "react";
import "./editablevalue.css";

// A press on one of these inside a link value keeps its own action.
const PRESSABLE =
  "a[href], button, input, select, textarea, summary, [tabindex]";

export type EditableValueProps = Readonly<{
  /** The value as the record reads it. */
  children?: ReactNode;
  /** The verb, translated ("Change Industry"): the bubble's name. */
  editLabel: string;
  /** Opens the editor. Absent, the value is read-only. */
  onEdit?: () => void;
  editable?: boolean;
  /** Shown in the pill while `children` is empty ("Add industry"). */
  placeholder?: ReactNode;
  /**
   * The value is itself a link (an address, a number, a profile).
   * The link keeps its action; the padding and the bubble open the editor.
   */
  link?: boolean;
  testId?: string;
}>;

function isBlank(node: ReactNode): boolean {
  return node === undefined || node === null || node === false || node === "";
}

export function EditableValue({
  children,
  editLabel,
  onEdit,
  editable = true,
  placeholder,
  link = false,
  testId,
}: EditableValueProps) {
  const verbId = useId();
  const faceId = useId();
  const edit = useRef<HTMLButtonElement>(null);
  if (!onEdit || !editable) {
    return <span data-testid={testId}>{children}</span>;
  }
  const face = (
    <span id={faceId} className="editable-value-face">
      {isBlank(children) ? (
        <span className="editable-value-placeholder">{placeholder}</span>
      ) : (
        children
      )}
    </span>
  );
  const bubble = (
    <span className="editable-value-bubble" aria-hidden="true">
      <Pencil />
    </span>
  );
  if (!link) {
    return (
      <button
        type="button"
        className="editable-value"
        // The verb and the value both name it, so the words a reader sees
        // are words a voice command can say.
        aria-labelledby={`${verbId} ${faceId}`}
        aria-haspopup="dialog"
        data-testid={testId}
        onClick={onEdit}
      >
        <span id={verbId} className="sr-only">
          {editLabel}
        </span>
        {face}
        {bubble}
      </button>
    );
  }
  const pressPadding = (event: MouseEvent<HTMLElement>) => {
    const hit =
      event.target instanceof Element ? event.target.closest(PRESSABLE) : null;
    if (hit && event.currentTarget.contains(hit)) return;
    // The dialog gives focus back to its opener, so the opener must hold it.
    edit.current?.focus();
    onEdit();
  };
  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: a pointer shortcut to the edit button inside, which is the keyboard's way in
    // biome-ignore lint/a11y/useKeyWithClickEvents: the edit button inside is the keyboard's way in; a key here would land on the link first
    <span
      className="editable-value editable-value-link"
      data-testid={testId}
      onClick={pressPadding}
    >
      {face}
      <button
        ref={edit}
        type="button"
        className="editable-value-edit"
        aria-label={editLabel}
        aria-haspopup="dialog"
        onClick={onEdit}
      >
        {bubble}
      </button>
    </span>
  );
}
