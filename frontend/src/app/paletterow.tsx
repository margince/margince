// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  ChevronRight,
  CornerDownLeft,
  FileText,
  type LucideIcon,
  Mail,
  MessageSquare,
  Package,
  Plus,
  Search,
  Settings,
  Sparkles,
  Tag,
} from "lucide-react";
import { createElement } from "react";
import { RECORD_CHIP_ICONS } from "../components/recordlinks";
import { Avatar, Badge, BusyMark } from "../design-system/atoms";
import { EmailReference } from "../design-system/emailreference";
import { Eyebrow } from "../design-system/eyebrow";
import { StatusBadge } from "../design-system/statusbadge";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { SearchHitGroup } from "../lib/searchkinds";
import type { Command } from "./palette";

const TYPE_KEY: Record<Command["type"], MessageKey> = {
  screen: "palette.typeScreen",
  setting: "palette.typeSetting",
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
      {!command.recent &&
        command.group &&
        command.group !== previous?.group && (
          <Eyebrow as="h2" size="xxsmall" className="palette-group">
            {command.group}
          </Eyebrow>
        )}
      <button
        type="button"
        disabled={command.pending}
        aria-busy={command.pending || undefined}
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
        <RowGlyph command={command} />
        <span className="palette-text">
          {command.cite ? (
            <EmailReference
              stacked
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
        </span>
        {command.status && (
          <StatusBadge
            kind={command.status.kind}
            label={command.status.label}
          />
        )}
        {command.aside && (
          <span className="palette-aside t-caption">{command.aside}</span>
        )}
        {command.id === "ask-ai" && <Badge tone="ai">{t("palette.ask")}</Badge>}
        {!command.group && <Badge>{t(TYPE_KEY[command.type])}</Badge>}
        <ChevronRight aria-hidden className="palette-chevron" />
      </button>
    </>
  );
}

// The glyph of each kind a hit can be, beside the record chips' own.
const KIND_ICON: Partial<Record<SearchHitGroup, LucideIcon>> = {
  ...RECORD_CHIP_ICONS,
  product: Package,
  offer_template: FileText,
  email: Mail,
  activity: MessageSquare,
  tag: Tag,
};

function commandIcon(command: Command): LucideIcon {
  if (command.icon) return command.icon;
  if (command.kind) return KIND_ICON[command.kind] ?? CornerDownLeft;
  if (command.recent?.kind === "query" || command.id === "search:all") {
    return Search;
  }
  if (command.type === "setting") return Settings;
  return command.id.startsWith("action:new-") ? Plus : CornerDownLeft;
}

// A record's mark where it has one, else its kind's glyph in a tile. Not
// hidden: the row's name holds what the mark prints (WCAG 2.5.3).
function RowGlyph({ command }: Readonly<{ command: Command }>) {
  if (command.pending) return <BusyMark />;
  if (command.mark) {
    return (
      <span className="palette-mark">
        <Avatar
          name={command.mark.name}
          identity={command.mark.identity}
          src={command.mark.logo}
        />
      </span>
    );
  }
  if (command.id === "ask-ai") {
    return (
      <span className="palette-tile palette-tile-ai">
        <Sparkles aria-hidden />
      </span>
    );
  }
  return (
    <span className="palette-tile">
      {createElement(commandIcon(command), { "aria-hidden": true })}
    </span>
  );
}
