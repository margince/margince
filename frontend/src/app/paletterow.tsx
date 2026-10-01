// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CornerDownLeft, Sparkles } from "lucide-react";
import { Badge } from "../design-system/atoms";
import { EmailReference } from "../design-system/emailreference";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { Command } from "./palette";

const TYPE_KEY: Record<Command["type"], MessageKey> = {
  screen: "palette.typeScreen",
  action: "palette.typeAction",
  record: "palette.typeRecord",
};

/**
 * One destination in the palette list, under the heading of its group when it
 * is the first of a run. The heading sits outside the row: the arrow keys walk
 * rows, and a heading is not somewhere to go.
 */
export function PaletteRow({
  command,
  previous,
  selected,
  onRun,
}: Readonly<{
  command: Command;
  // The row above, which decides whether this one opens a new group.
  previous: Command | undefined;
  selected: boolean;
  onRun: (command: Command) => void;
}>) {
  const t = useT();
  return (
    <>
      {command.group && command.group !== previous?.group && (
        <p className="palette-group t-caption">{command.group}</p>
      )}
      <button
        type="button"
        className={
          selected ? "palette-row t-body selected" : "palette-row t-body"
        }
        onClick={() => onRun(command)}
        ref={(element) => {
          if (selected) {
            element?.scrollIntoView?.({ block: "nearest" });
          }
        }}
      >
        {command.id === "ask-ai" ? (
          <Sparkles aria-hidden />
        ) : (
          <CornerDownLeft aria-hidden />
        )}
        {command.cite ? (
          <EmailReference
            subject={command.cite.subject}
            occurredAt={command.cite.occurredAt}
          />
        ) : (
          <>
            <span className="label">{command.label}</span>
            {command.subtitle && (
              <span className="sub t-caption">{command.subtitle}</span>
            )}
          </>
        )}
        {!command.group && <Badge>{t(TYPE_KEY[command.type])}</Badge>}
      </button>
    </>
  );
}
