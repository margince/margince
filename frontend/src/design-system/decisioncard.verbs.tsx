// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pencil, RotateCcwClock } from "lucide-react";
import { type RefObject, useId, useRef } from "react";
import { ActionRow } from "./actionrow";
import { Button, OverflowMenu } from "./atoms";
import type { DecisionCardLabels, DecisionCompactWords } from "./decisionhead";
import { IconAction } from "./iconaction";

/** A rejection the caller holds back before sending it, with the way to take it back. */
export type DecisionHeldRejection = Readonly<{
  label: string;
  undo: string;
  onUndo: () => void;
}>;

type FocusNext = RefObject<"undo" | "reject" | null>;

function takesFocus(focusNext: FocusNext, control: "undo" | "reject") {
  return (node: HTMLButtonElement | null) => {
    if (node && focusNext.current === control) {
      focusNext.current = null;
      node.focus();
    }
  };
}

// The verbs, or the held rejection in their place. One component across the
// swap, so the focus it asked for survives it.
export function DecisionAnswers({
  decided,
  held,
  answering,
  onReject,
  ...verbs
}: Readonly<
  Omit<Parameters<typeof DecisionVerbs>[0], "focusNext"> & {
    decided: boolean;
    held?: DecisionHeldRejection;
    answering: boolean;
  }
>) {
  const focusNext = useRef<"undo" | "reject" | null>(null);
  if (decided) {
    return null;
  }
  if (held) {
    return (
      <HeldRejectionBar
        held={held}
        focusNext={focusNext}
        onUndo={() => {
          focusNext.current = "reject";
          held.onUndo();
        }}
      />
    );
  }
  if (!answering) {
    return null;
  }
  return (
    <DecisionVerbs
      {...verbs}
      focusNext={focusNext}
      onReject={
        onReject &&
        (() => {
          focusNext.current = "undo";
          onReject();
        })
      }
    />
  );
}

// Only Accept goes busy while a verdict is out; drawing Reject busy would
// claim a rejection nobody sent.
function DecisionVerbs({
  labels,
  compact,
  pending,
  focusNext,
  onAccept,
  onEdit,
  onReject,
  onSkip,
}: Readonly<{
  labels: DecisionCardLabels;
  /** The dense row's words; absent draws Later and Edit as glyphs. */
  compact?: DecisionCompactWords;
  pending?: boolean;
  focusNext: FocusNext;
  onAccept?: () => void;
  onEdit?: () => void;
  onReject?: () => void;
  onSkip?: () => void;
}>) {
  if (!onAccept && !onEdit && !onReject && !onSkip) {
    return null;
  }
  const answers = (onReject || onAccept) && (
    <>
      {onReject && (
        <Button
          variant="danger"
          ref={takesFocus(focusNext, "reject")}
          disabled={pending}
          onClick={onReject}
        >
          {labels.reject}
        </Button>
      )}
      {onAccept && (
        <Button variant="primary" pending={pending} onClick={onAccept}>
          {labels.accept}
        </Button>
      )}
    </>
  );
  const later = onSkip && labels.skip;
  if (compact) {
    return (
      <ActionRow className="dcard-verbs" primary={answers}>
        {(later || onEdit) && (
          <OverflowMenu label={compact.more}>
            {later && (
              <Button disabled={pending} onClick={onSkip}>
                {later}
              </Button>
            )}
            {onEdit && (
              <Button disabled={pending} onClick={onEdit}>
                {labels.edit}
              </Button>
            )}
          </OverflowMenu>
        )}
      </ActionRow>
    );
  }
  return (
    <ActionRow className="dcard-verbs" primary={answers}>
      {later && (
        <IconAction
          label={later}
          icon={<RotateCcwClock aria-hidden />}
          disabled={pending}
          onClick={onSkip}
        />
      )}
      {onEdit && (
        <IconAction
          label={labels.edit}
          icon={<Pencil aria-hidden />}
          disabled={pending}
          onClick={onEdit}
        />
      )}
    </ActionRow>
  );
}

function HeldRejectionBar({
  held,
  focusNext,
  onUndo,
}: Readonly<{
  held: DecisionHeldRejection;
  focusNext: FocusNext;
  onUndo: () => void;
}>) {
  const saidId = useId();
  return (
    <ActionRow
      className="dcard-verbs"
      primary={
        <>
          <output id={saidId}>{held.label}</output>
          <Button
            ref={takesFocus(focusNext, "undo")}
            aria-describedby={saidId}
            onClick={onUndo}
          >
            {held.undo}
          </Button>
        </>
      }
    />
  );
}
