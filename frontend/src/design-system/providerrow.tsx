// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { type BADGE_TONES, Badge } from "./atoms";
import { Heading } from "./heading";
import "./providerrow.css";

/**
 * The states a connection can be in, as Badge tones. `ai`, `accent` and
 * `discovery` are left out because none of them is a verdict on a connection.
 * The caller supplies the words: Not connected is `default`, Syncing `info`,
 * Connected `success`, Needs attention `warning`, Failed `danger`.
 */
export const PROVIDER_ROW_TONES = [
  "default",
  "info",
  "success",
  "warning",
  "danger",
] as const satisfies readonly (typeof BADGE_TONES)[number][];

export type ProviderRowTone = (typeof PROVIDER_ROW_TONES)[number];

/**
 * One connected account: the vendor's mark, its name with a status, the account
 * it reads, what it brings in, and the one verb that matters now.
 *
 * Reach for SettingRow when the row is a DECISION (a label and the control that
 * answers it); reach for this when the row is a THING somebody connected. The
 * mark column and the status beside the name are what SettingRow cannot draw.
 *
 * Every slot is a node so a screen composes its own facts (chips, a sync time)
 * without the row learning what a mailbox is. Copy arrives translated.
 */
export function ProviderRow({
  mark,
  name,
  headingLevel,
  status,
  account,
  facts,
  action,
  menu,
  children,
}: Readonly<{
  /** A ProviderMark, or a lucide glyph for a source with no brand mark. */
  mark: ReactNode;
  name: string;
  /** Makes the name a heading, for a row that opens a section of its own. */
  headingLevel?: "h3" | "h4";
  status?: Readonly<{ tone: ProviderRowTone; label: string; live?: boolean }>;
  /** The address or account the connection reads, truncated on one line. */
  account?: ReactNode;
  /** One wrapping line of chips and quiet facts such as the last sync. */
  facts?: ReactNode;
  /** The one visible verb: Connect, Reconnect, Set up. */
  action?: ReactNode;
  /** An OverflowMenu holding every other verb, in words. */
  menu?: ReactNode;
  /** Whatever hangs under the row, aligned to its text column. */
  children?: ReactNode;
}>) {
  const hasActions = action !== undefined || menu !== undefined;
  return (
    <div className="provider-row">
      <span className="provider-row-mark" aria-hidden="true">
        {mark}
      </span>
      <div className="provider-row-text">
        <div className="provider-row-head">
          {/* One look for the name whether or not it is a heading, so a list
              that mixes the two still reads as one list. */}
          <Heading size="xsmall" as={headingLevel ?? "span"}>
            {name}
          </Heading>
          {status !== undefined && (
            <Badge tone={status.tone} live={status.live}>
              {status.label}
            </Badge>
          )}
        </div>
        {account !== undefined && (
          <span className="provider-row-account">{account}</span>
        )}
        {facts !== undefined && (
          <div className="provider-row-facts">{facts}</div>
        )}
      </div>
      {hasActions && (
        <div className="provider-row-actions">
          {action}
          {menu}
        </div>
      )}
      {children !== undefined && (
        <div className="provider-row-body">{children}</div>
      )}
    </div>
  );
}

/** Rows of ProviderRow, with the hairline between them owned here, as SettingList does. */
export function ProviderList({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="provider-list">{children}</div>;
}
