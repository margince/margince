// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useViewerZone } from "../../lib/viewerzone";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { type WorklistItem, worklistKey } from "../worklist/worklist.queries";
import { Triage } from "./brief.focus";
import { identity } from "./brief.focuslink";
import { HomeListReturnScope } from "./brief.queue";

/**
 * The day's rows as cards, the one in hand open, with Next. Home's "Needs you"
 * and the agent panel both draw this, so a row reads and answers the same in
 * either place.
 */
export function ReadyStack({
  rows,
  onContext,
}: Readonly<{
  rows: readonly WorklistItem[];
  onContext?: (item: WorklistItem) => void;
}>) {
  const readerZone = useViewerZone();
  const client = useQueryClient();
  const [openEmail, setOpenEmail] = useState<string | null>(null);
  const { at, choose } = useCardChoice(rows);
  const lead = rows[at];
  return (
    <>
      {lead && (
        <HomeListReturnScope>
          <Triage
            rows={rows}
            at={at}
            lead={lead}
            onChoose={choose}
            onOpenEmail={setOpenEmail}
            onContext={onContext}
          />
        </HomeListReturnScope>
      )}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={readerZone}
        onClose={() => setOpenEmail(null)}
        onReplySent={() =>
          void client.invalidateQueries({ queryKey: worklistKey })
        }
      />
    </>
  );
}

// The row in hand, by identity. The queue is re-read and answered rows leave
// it, so an index would hand over the row that moved up. A chosen row that has
// gone falls back to the day's first.
function useCardChoice(rows: readonly WorklistItem[]) {
  const [choice, setChoice] = useState<{ key: string | null; at: number }>({
    key: null,
    at: 0,
  });
  const found = rows.findIndex((item) => identity(item) === choice.key);
  const at =
    found >= 0 ? found : Math.max(0, Math.min(choice.at, rows.length - 1));
  if (at !== choice.at) setChoice({ ...choice, at });
  return {
    at,
    choose: (item: WorklistItem) =>
      setChoice({ key: identity(item), at: Math.max(0, rows.indexOf(item)) }),
  };
}
