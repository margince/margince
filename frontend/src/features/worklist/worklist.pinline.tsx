// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { useToast } from "../../design-system/toast";
import { useTooltip } from "../../design-system/tooltip";
import { useT } from "../../i18n";
import { isPinned } from "./worklist.pinmark";
import { usePinRow, type WorklistItem } from "./worklist.queries";

// The reader's own override: this row leads their day, whatever the ranking
// chose. It reads the row's own `pinned` reason to know which way to go. A
// local flag would disagree with the page once the reader pinned elsewhere.
// RowActs offers no pin on a batch row. Its id is minted by the fold and will
// not exist under that key on the next read.
export function PinLine({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const toast = useToast();
  const pin = usePinRow();
  const pinned = isPinned(item);
  // What the word cannot say: whose order it changes and for how long. Per
  // state, so Unpin never describes itself as keeping the row on top.
  const { ref, trigger, tip } = useTooltip<HTMLButtonElement>(
    t(pinned ? "worklist.verb.unpinHint" : "worklist.verb.pinHint"),
  );
  return (
    // A verb rather than a switch, so the menu closes on it. The row shows
    // the pin beside its name (`PinnedMark`).
    <Button
      variant="ghost"
      ref={ref}
      {...trigger}
      pending={pin.isPending}
      onClick={() =>
        pin.mutate(
          { source: item.source, rowId: item.id, pinned },
          {
            // A refused write otherwise leaves the row where it was with
            // nothing said, so the reader is told nothing and sees nothing.
            onError: () =>
              toast.show(
                t(
                  pinned
                    ? "worklist.verb.unpinFailed"
                    : "worklist.verb.pinFailed",
                ),
                { tone: "danger" },
              ),
          },
        )
      }
    >
      {t(pinned ? "worklist.verb.unpin" : "worklist.verb.pin")}
      {tip}
    </Button>
  );
}
