// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { Button, type ButtonVariant } from "../../design-system/atoms";
import { StageActions } from "../../design-system/onboarding-stage";

/**
 * A step's way onward, on the stage's sticky rail, under one rule for every
 * step: the button always presses. What still stands in the way is said when
 * it is pressed — beside the button, and by the caller at the fields — rather
 * than by a grey button a reader has to work out the reason for.
 *
 * `blockers` are what the step cannot proceed without, by the name the reader
 * knows them by; the press names them and goes nowhere. `held` is the one
 * exception to pressing: a standing refusal from the server that an identical
 * press cannot change, which the caller explains beside its own retry — a
 * button that pressed there would only earn the same refusal again. `note` is
 * advice that does not block, shown regardless. A secondary way (a skip, a
 * "not now") sits before the primary as `children`.
 */
export function WayOnward({
  label,
  pendingLabel,
  pending = false,
  variant = "primary",
  blockers = [],
  held = false,
  stillNeeded,
  note,
  onGo,
  children,
}: Readonly<{
  label: string;
  /** What the button says while the step's write is in flight. */
  pendingLabel?: string;
  pending?: boolean;
  variant?: ButtonVariant;
  blockers?: readonly string[];
  held?: boolean;
  /** Spells the blockers out for the reader once the button was pressed early. */
  stillNeeded: (blockers: readonly string[]) => string;
  note?: ReactNode;
  onGo: () => void;
  children?: ReactNode;
}>) {
  const [attempted, setAttempted] = useState(false);
  const blocked = blockers.length > 0;
  return (
    <StageActions>
      {attempted && blocked ? (
        <p className="ob-stage-note" role="alert">
          {stillNeeded(blockers)}
        </p>
      ) : (
        note
      )}
      {children}
      <Button
        variant={variant}
        pending={pending}
        disabled={held}
        onClick={() => {
          if (blocked) {
            setAttempted(true);
            return;
          }
          onGo();
        }}
      >
        {pending && pendingLabel !== undefined ? pendingLabel : label}
      </Button>
    </StageActions>
  );
}
