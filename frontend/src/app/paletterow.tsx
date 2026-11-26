// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  ChevronRight,
  CornerDownLeft,
  DoorOpen,
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
import { createElement, Fragment } from "react";
import { RECORD_CHIP_ICONS } from "../components/recordlinks";
import { Avatar, Badge, BusyMark } from "../design-system/atoms";
import { EmailReference } from "../design-system/emailreference";
import { Heading } from "../design-system/heading";
import { ListOption } from "../design-system/listoption";
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
 * The palette's results as the listbox its search box drives. A run of rows
 * under one heading is a `role="group"` named by it; the arrows walk the rows,
 * and a heading is not somewhere to go. Recents are named by the heading above
 * the list, which carries a button and so cannot sit inside it.
 */
export function PaletteOptions({
  rows,
  selected,
  listboxId,
  recentHeadingId,
  onRun,
}: Readonly<{
  rows: readonly Command[];
  selected: number;
  listboxId: string;
  recentHeadingId: string | undefined;
  onRun: (command: Command) => void;
}>) {
  return (
    <div id={listboxId} role="listbox">
      {paletteRuns(rows).map((run) => {
        const options = run.rows.map((command, offset) => {
          const index = run.start + offset;
          return (
            <PaletteRow
              key={command.id}
              id={optionId(listboxId, index)}
              command={command}
              selected={index === selected}
              onRun={onRun}
            />
          );
        });
        const headingId = `${listboxId}-group-${run.start}`;
        const labelledBy = run.recent ? recentHeadingId : headingId;
        if (labelledBy === undefined || (!run.recent && !run.group)) {
          return <Fragment key={headingId}>{options}</Fragment>;
        }
        return (
          // biome-ignore lint/a11y/useSemanticElements: a listbox owns options and groups of them; a fieldset is a form's group and no listbox child
          <div key={headingId} role="group" aria-labelledby={labelledBy}>
            {!run.recent && (
              <Heading
                as="div"
                size="xxsmall"
                id={headingId}
                className="t-eyebrow palette-group"
              >
                {run.group}
              </Heading>
            )}
            {options}
          </div>
        );
      })}
    </div>
  );
}

function optionId(listboxId: string, index: number): string {
  return `${listboxId}-option-${index}`;
}

/** The DOM id of the highlighted option, which `aria-activedescendant` names. */
export function paletteOptionId(
  listboxId: string,
  rows: readonly Command[],
  selected: number,
): string | undefined {
  return selected >= 0 && selected < rows.length
    ? optionId(listboxId, selected)
    : undefined;
}

type PaletteRun = {
  recent: boolean;
  group: string | undefined;
  start: number;
  rows: Command[];
};

// Consecutive rows under one heading. Recents form one run whatever group they
// name, and an option keeps its index in the whole list.
function paletteRuns(rows: readonly Command[]): readonly PaletteRun[] {
  const runs: PaletteRun[] = [];
  rows.forEach((command, index) => {
    const recent = command.recent !== undefined;
    const group = recent ? undefined : command.group;
    const last = runs.at(-1);
    if (last && last.recent === recent && last.group === group) {
      last.rows.push(command);
    } else {
      runs.push({ recent, group, start: index, rows: [command] });
    }
  });
  return runs;
}

/** One destination in the palette list. */
export function PaletteRow({
  id,
  command,
  selected,
  onRun,
}: Readonly<{
  id: string;
  command: Command;
  selected: boolean;
  onRun: (command: Command) => void;
}>) {
  const t = useT();
  return (
    <ListOption
      id={id}
      selected={selected}
      disabled={command.pending}
      busy={command.pending}
      className={
        selected ? "palette-row t-body selected" : "palette-row t-body"
      }
      onPick={() => onRun(command)}
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
        <StatusBadge kind={command.status.kind} label={command.status.label} />
      )}
      {command.aside && (
        <span className="palette-aside t-caption">{command.aside}</span>
      )}
      {command.id === "ask-ai" && <Badge tone="ai">{t("palette.ask")}</Badge>}
      {!command.group && <Badge>{t(TYPE_KEY[command.type])}</Badge>}
      <ChevronRight aria-hidden className="palette-chevron" />
    </ListOption>
  );
}

// The glyph of each kind a hit can be, beside the record chips' own.
const KIND_ICON: Partial<Record<SearchHitGroup, LucideIcon>> = {
  ...RECORD_CHIP_ICONS,
  deal_room: DoorOpen,
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
