// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { SegmentedControl } from "../design-system/atoms";
import { ReadyStack } from "../features/brief/brief.feed";
import type { WorklistItem } from "../features/worklist/worklist.queries";
import { useT } from "../i18n";

const SCOPES = ["here", "all"] as const;
type Scope = (typeof SCOPES)[number];

/**
 * The reader's work in the panel: one card in hand, answered here the way Home
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
  const [chosen, setChosen] = useState<Scope | null>(null);
  // A page with rows of its own opens on them. One whose rows are the whole
  // day has nothing to choose between.
  const splits = here.length > 0 && here.length < rows.length;
  const scope: Scope = splits ? (chosen ?? "here") : "all";
  return (
    <div className="arsect arwork">
      {splits && (
        <SegmentedControl
          options={SCOPES}
          value={scope}
          onChange={setChosen}
          label={t("agent.ready.scope")}
          labels={{
            here: t("agent.ready.scope.here"),
            all: t("agent.ready.scope.all"),
          }}
          counts={{ here: here.length, all: rows.length }}
        />
      )}
      {/* Keyed by scope: the card in hand belongs to the list it was taken from. */}
      <ReadyStack key={scope} rows={scope === "here" ? here : rows} solo />
    </div>
  );
}
