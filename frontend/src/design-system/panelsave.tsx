// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { ActionRow } from "./actionrow";
import { Button } from "./atoms";
import "./panelsave.css";

// A card with several fields commits them together, so the verbs exist only
// while there is something to commit. It goes in the `actions` of a `Panel`.
export function PanelSave({
  dirty,
  pending = false,
  onSave,
  onDiscard,
  saveLabel,
  caption,
}: Readonly<{
  /** True while the card's fields differ from what was last saved. */
  dirty: boolean;
  /** The save is in flight. */
  pending?: boolean;
  onSave: () => void;
  onDiscard: () => void;
  /** The verb when "Save" undersells it, such as "Save preferences". */
  saveLabel?: string;
  /** A line before the verbs, such as "Unsaved changes". */
  caption?: string;
}>) {
  const t = useT();
  if (!dirty) return null;
  return (
    <ActionRow
      className="panel-save"
      primary={
        <Button variant="primary" pending={pending} onClick={onSave}>
          {saveLabel ?? t("panelSave.save")}
        </Button>
      }
    >
      {caption && <p className="t-caption panel-save-caption">{caption}</p>}
      {/* A discard that lands while the save runs would race it. */}
      <Button variant="ghost" disabled={pending} onClick={onDiscard}>
        {t("panelSave.discard")}
      </Button>
    </ActionRow>
  );
}
