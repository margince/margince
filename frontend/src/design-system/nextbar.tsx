// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { Badge, Button } from "./atoms";
import { Panel } from "./panel";
import { type PeekContent, usePeek } from "./peek";
import { badgeTone, type ReadingTone } from "./readingtone";
import "./nextbar.css";

/** The other open tasks behind the bar: how many, where the list is, and which
 *  they are. */
export type NextBarMore = Readonly<{
  count: number;
  onOpen: () => void;
  peek?: PeekContent;
}>;

/**
 * The one thing to do next on a record, as a bar at the top of its Overview.
 * It shows what it is, how late, why, and the verbs that settle or defer it. An absent
 * `title` says there is nothing dated next, rather than drawing an empty bar.
 */
export function NextBar({
  title,
  late,
  sub,
  insight,
  action,
  snooze,
  more,
}: Readonly<{
  title?: ReactNode;
  // How late, or what makes it due ("19 days late", "Demo booked").
  late?: Readonly<{ label: string; tone: ReadingTone }>;
  // Why it is next: when it was due, who promised it.
  sub?: ReactNode;
  // A link after the sentence to what the move was read from. The caller
  // gives it the indigo ink only when an agent wrote it.
  insight?: ReactNode;
  // The verb that settles the move, as the caller's own `Button`.
  action?: ReactNode;
  snooze?: Readonly<{ onSnooze: () => void; pending?: boolean }>;
  // The other open tasks, behind a link to the record's task list. `peek`
  // names them on hover and focus, so "+2 tasks" is never a lone number.
  more?: NextBarMore;
}>) {
  const t = useT();
  return (
    <Panel>
      <div className="next-bar-row">
        <span className="next-bar-label">{t("standing.next.label")}</span>
        <span className="next-bar-rule" aria-hidden="true" />
        <div className="next-bar-body">
          {title === undefined ? (
            <span className="next-bar-none">{t("standing.next.none")}</span>
          ) : (
            <>
              <span className="next-bar-title-row">
                <span className="next-bar-title">{title}</span>
                {late && (
                  <Badge tone={badgeTone(late.tone)}>{late.label}</Badge>
                )}
              </span>
              {(sub || insight) && (
                <span className="next-bar-sub">
                  {sub}
                  {sub && insight ? " · " : null}
                  {insight}
                </span>
              )}
            </>
          )}
        </div>
        {(action || snooze || more) && (
          <div className="next-bar-verbs">
            {action}
            {snooze && (
              <Button
                variant="ghost"
                onClick={snooze.onSnooze}
                pending={snooze.pending}
              >
                {t("standing.next.snooze")}
              </Button>
            )}
            {more && more.count > 0 && <MoreTasks more={more} />}
          </div>
        )}
      </div>
    </Panel>
  );
}

function MoreTasks({ more }: Readonly<{ more: NextBarMore }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const { ref, props, panel } = usePeek<HTMLButtonElement>({
    content: more.peek,
  });
  return (
    <>
      <Button ref={ref} variant="link" onClick={more.onOpen} {...props}>
        {plural("standing.next.more", more.count, {
          count: formatNumber(more.count, locale),
        })}
      </Button>
      {panel}
    </>
  );
}
