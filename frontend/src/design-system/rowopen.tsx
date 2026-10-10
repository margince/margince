// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronRight } from "lucide-react";
import { useId } from "react";
import { IconAction } from "./iconaction";

// The keyboard path to what a press on the row opens. A refusal rides the tip
// and the description, not a visible line that would widen the row.
export function RowOpen({
  label,
  refusal,
  onOpen,
}: Readonly<{ label: string; refusal?: string; onOpen?: () => void }>) {
  const reasonId = useId();
  return (
    <>
      <IconAction
        variant="ghost"
        icon={<ChevronRight aria-hidden />}
        label={label}
        hint={refusal}
        reasonId={refusal === undefined ? undefined : reasonId}
        onClick={onOpen}
      />
      {refusal !== undefined && (
        <span id={reasonId} className="sr-only">
          {refusal}
        </span>
      )}
    </>
  );
}
