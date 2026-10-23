// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pencil, RotateCcwClock } from "lucide-react";
import { type RefObject, useLayoutEffect, useRef } from "react";
import { ActionRow } from "./actionrow";
import { Button, OverflowMenu } from "./atoms";
import type { DecisionCardLabels, DecisionCompactWords } from "./decisionhead";
import { IconAction } from "./iconaction";

/**
 * A rejection the caller holds back before sending it. `onUndo` is absent once
 * it is sent; `onPause` hears whether a pointer or focus is on Undo.
 */
export type DecisionHeldRejection = Readonly<{
  label: string;
  undo: string;
  onUndo?: () => void;
  onPause?: (paused: boolean) => void;
}>;

export type FocusNext = RefObject<"label" | "reject" | null>;

export function takesFocus(
  focusNextRef: FocusNext,
  target: "label" | "reject",
) {
  return (node: HTMLElement | null) => {
    if (node && focusNextRef.current === target) {
      focusNextRef.current = null;
      node.focus();
    }
  };
}

export function DecisionAnswers({
  decided,
  held,
  answering,
  onReject,
  focusNextRef,
  ...verbs
}: Readonly<
  Parameters<typeof DecisionVerbs>[0] & {
    decided: boolean;
    held?: DecisionHeldRejection;
    answering: boolean;
  }
>) {
  if (decided) {
    return null;
  }
  if (held) {
    return <HeldRejectionBar held={held} focusNextRef={focusNextRef} />;
  }
  if (!answering) {
    return null;
  }
  return (
    <DecisionVerbs
      {...verbs}
      focusNextRef={focusNextRef}
      onReject={
        onReject &&
        (() => {
          focusNextRef.current = "label";
          onReject();
        })
      }
    />
  );
}

// Only Accept goes busy: a rejection on its way is drawn by the held bar.
function DecisionVerbs({
  labels,
  compact,
  pending,
  focusNextRef,
  onAccept,
  onEdit,
  onReject,
  onSkip,
}: Readonly<{
  labels: DecisionCardLabels;
  /** The dense row's words; absent draws Later and Edit as glyphs. */
  compact?: DecisionCompactWords;
  pending?: boolean;
  focusNextRef: FocusNext;
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
          ref={takesFocus(focusNextRef, "reject")}
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

// Focus leaving the bar goes to Reject, else the lapse notice, else a decided
// card. A card drawing none of them clears the flag and focuses nothing.
function HeldRejectionBar({
  held,
  focusNextRef,
}: Readonly<{
  held: DecisionHeldRejection;
  focusNextRef: FocusNext;
}>) {
  const label = useRef<HTMLElement | null>(null);
  const over = useRef({ pointer: false, focus: false });
  const report = (change: Readonly<{ pointer?: boolean; focus?: boolean }>) => {
    over.current = { ...over.current, ...change };
    held.onPause?.(over.current.pointer || over.current.focus);
  };
  useLayoutEffect(() => {
    const bar = label.current?.parentElement;
    return () => {
      if (bar?.contains(document.activeElement)) {
        focusNextRef.current = "reject";
      }
    };
  }, [focusNextRef]);
  const { onUndo } = held;
  return (
    <ActionRow
      className="dcard-verbs"
      primary={
        <>
          <span
            tabIndex={-1}
            ref={(node) => {
              label.current = node;
              takesFocus(focusNextRef, "label")(node);
            }}
          >
            {held.label}
          </span>
          {onUndo && (
            <Button
              onPointerEnter={() => report({ pointer: true })}
              onPointerLeave={() => report({ pointer: false })}
              onFocus={() => report({ focus: true })}
              onBlur={() => report({ focus: false })}
              onClick={() => {
                focusNextRef.current = "reject";
                onUndo();
              }}
            >
              {held.undo}
            </Button>
          )}
        </>
      }
    />
  );
}
