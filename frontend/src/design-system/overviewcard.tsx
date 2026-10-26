// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Panel } from "./panel";
import "./overviewcard.css";

/**
 * One card under a record's How it stands: a title with one link at its
 * end, then rows parted by hairlines. No band under the title, no count and
 * no chevron; the link names the tab that holds the whole list. It is a
 * `Panel` with a flat head. `empty` stands in for the rows when there are
 * none, with `emptyAction` as its verb. `foot` is one closing line about the
 * whole card, in warning ink with `footTone="warn"`.
 */
export function OverviewCard({
  title,
  action,
  empty,
  emptyAction,
  foot,
  footTone,
  children,
  className,
}: Readonly<{
  title: ReactNode;
  action?: ReactNode;
  empty?: ReactNode;
  emptyAction?: ReactNode;
  foot?: ReactNode;
  footTone?: "warn";
  children?: ReactNode;
  className?: string;
}>) {
  const hasBody =
    children !== undefined && children !== null && children !== false;
  return (
    <Panel flatHead title={title} titleAction={action} className={className}>
      <div className="overviewcard-body">
        {hasBody ? (
          children
        ) : (
          <div className="overviewcard-empty">
            {empty && <p className="overviewcard-empty-words">{empty}</p>}
            {emptyAction}
          </div>
        )}
        {foot && (
          <p
            className={
              footTone === "warn"
                ? "overviewcard-foot overviewcard-foot-warn"
                : "overviewcard-foot"
            }
          >
            {foot}
          </p>
        )}
      </div>
    </Panel>
  );
}

/** The rows of an `OverviewCard`, as one list. */
export function OverviewList({ children }: Readonly<{ children: ReactNode }>) {
  return <ul className="overviewlist">{children}</ul>;
}

/**
 * One row: an optional `lead` (a tick box), the `title` over its `sub`, and
 * a `right` slot for a `Badge` or a short value, or a pill beside one verb.
 * `figure` draws the value in ink with tabular digits, for an amount;
 * otherwise it is quiet.
 */
export function OverviewRow({
  lead,
  title,
  sub,
  right,
  figure,
}: Readonly<{
  lead?: ReactNode;
  title: ReactNode;
  sub?: ReactNode;
  right?: ReactNode;
  figure?: boolean;
}>) {
  return (
    <li className="overviewrow">
      {lead && <span className="overviewrow-lead">{lead}</span>}
      <span className="overviewrow-body">
        <span className="overviewrow-title">{title}</span>
        {sub && <span className="overviewrow-sub">{sub}</span>}
      </span>
      {right && (
        <span
          className={
            figure
              ? "overviewrow-right overviewrow-figure"
              : "overviewrow-right"
          }
        >
          {right}
        </span>
      )}
    </li>
  );
}

/** A card whose body is one paragraph (what the deal is, what they asked). */
export function OverviewText({ children }: Readonly<{ children: ReactNode }>) {
  return <p className="overviewtext">{children}</p>;
}
