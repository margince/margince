// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button } from "../design-system/atoms";
import { ReadyStack } from "../features/brief/brief.feed";
import type { WorklistItem } from "../features/worklist/worklist.queries";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";

/**
 * The reader's work in the panel: the next task, answered here the way Home
 * answers it. Where the open page has rows of its own they come first, and the
 * whole day is one press away.
 */
export function ReadyWork({
  rows,
  here,
}: Readonly<{
  rows: readonly WorklistItem[];
  /** The rows about the open page; empty where the page has none. */
  here: readonly WorklistItem[];
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [whole, setWhole] = useState(false);
  // A page with rows of its own opens on them. One whose rows are the whole
  // day has nothing to choose between.
  const splits = here.length > 0 && here.length < rows.length;
  const narrowed = splits && !whole;
  return (
    <div className="arwork">
      {/* Keyed by scope: the card in hand belongs to the list it was taken from. */}
      <ReadyStack
        key={narrowed ? "here" : "all"}
        rows={narrowed ? here : rows}
        solo
      />
      {splits && (
        <div className="arscope">
          <Button variant="link" onClick={() => setWhole(narrowed)}>
            {narrowed
              ? plural("agent.ready.showAll", rows.length, {
                  count: formatNumber(rows.length, locale),
                })
              : t("agent.ready.showHere")}
          </Button>
        </div>
      )}
    </div>
  );
}
