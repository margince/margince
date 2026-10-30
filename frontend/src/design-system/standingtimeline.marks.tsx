// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowDownLeft, ArrowUpRight } from "lucide-react";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { type PeekContent, type PeekItem, usePeek } from "./peek";
import type { TimelineMark } from "./standingtimeline";
import type { MarkGroup } from "./standingtimeline.layout";
import { TIMELINE_ICON } from "./timelineentry";

// The marks' own layer, beside the canvas rather than inside it. The canvas is
// drawn for the eye and hidden from assistive technology. A chip is a control
// a reader must be able to reach.
export function MarkLane({
  marks,
  groups,
}: Readonly<{ marks: readonly TimelineMark[]; groups: readonly MarkGroup[] }>) {
  return (
    <div className="standing-timeline-marks">
      {groups.map((group) => (
        <MarkChip
          key={marks[group.members[0] ?? 0]?.key}
          px={group.px}
          members={group.members.flatMap((index) => marks[index] ?? [])}
        />
      ))}
    </div>
  );
}

// The most rows one card lists; the rest are named and left to History.
const CARD_ROWS = 6;

function MarkChip({
  px,
  members,
}: Readonly<{ px: number; members: readonly TimelineMark[] }>) {
  const plural = usePlural();
  const t = useT();
  const { locale } = useLocale();
  const newest = members.at(-1);
  const oldest = members[0];
  const content: PeekContent | undefined =
    newest && oldest
      ? {
          head:
            members.length === 1
              ? newest.what
              : plural("standing.mark.group", members.length, {
                  count: formatNumber(members.length, locale),
                  range:
                    oldest.when === newest.when
                      ? newest.when
                      : t("standing.mark.range", {
                          from: oldest.when,
                          to: newest.when,
                        }),
                }),
          items: [...members].reverse().slice(0, CARD_ROWS).map(markRow),
          foot:
            members.length > CARD_ROWS
              ? plural("standing.mark.more", members.length - CARD_ROWS, {
                  count: formatNumber(members.length - CARD_ROWS, locale),
                })
              : undefined,
        }
      : undefined;
  const {
    ref: peekRef,
    props: peekProps,
    panel: peekPanel,
    open: peekOpen,
  } = usePeek<HTMLButtonElement>({ content, pressOpens: true });
  if (!newest || !content) {
    return null;
  }
  const Icon = TIMELINE_ICON[newest.kind];
  return (
    <>
      <button
        ref={peekRef}
        type="button"
        className="standing-timeline-mark"
        style={{ left: `${px}px` }}
        aria-label={content.head}
        aria-haspopup="true"
        aria-expanded={peekOpen}
        {...peekProps}
      >
        <Icon aria-hidden="true" />
        {members.length === 1 && newest.direction && (
          <span className="standing-timeline-mark-way">
            {newest.direction === "inbound" ? (
              <ArrowDownLeft aria-hidden="true" />
            ) : (
              <ArrowUpRight aria-hidden="true" />
            )}
          </span>
        )}
        {members.length > 1 && (
          <span className="standing-timeline-mark-count">
            {formatNumber(members.length, locale)}
          </span>
        )}
      </button>
      {peekPanel}
    </>
  );
}

function markRow(mark: TimelineMark): PeekItem {
  return {
    key: mark.key,
    title: mark.title,
    sub: [mark.detail, mark.when].filter(Boolean).join(" · "),
    onOpen: mark.onOpen,
  };
}
