// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";
import { type BADGE_TONES, Badge } from "./atoms";
import { Heading } from "./heading";
import "./providerrow.css";

/**
 * The states a connection can be in, as Badge tones. `ai`, `accent`, `info`
 * and `discovery` are left out because blue and violet never mean a status.
 * The caller supplies the words: Not connected is `default`, Syncing `default`
 * with `live`, Connected `success`, Needs attention `warning`, Failed `danger`.
 */
export const PROVIDER_ROW_TONES = [
  "default",
  "success",
  "warning",
  "danger",
] as const satisfies readonly (typeof BADGE_TONES)[number][];

export type ProviderRowTone = (typeof PROVIDER_ROW_TONES)[number];

type ProviderHeadingLevel = "h3" | "h4";

// Unset outside a ProviderList, so a lone row stays out of the outline.
const ListHeadingLevel = createContext<ProviderHeadingLevel | undefined>(
  undefined,
);

/**
 * One connected account. It shows the vendor's mark, its name with a status,
 * the account it reads, what it brings in, and the one verb that matters now.
 *
 * Use SettingRow for a decision (a label and the control that answers it).
 * Use this for a thing somebody connected. SettingRow cannot draw the mark
 * column or the status beside the name.
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
  reason,
  action,
  menu,
  children,
}: Readonly<{
  /** A ProviderMark, or a lucide glyph for a source with no brand mark. */
  mark: ReactNode;
  name: string;
  /** Makes the name a heading; inside a ProviderList it is one already. */
  headingLevel?: ProviderHeadingLevel;
  status?: Readonly<{ tone: ProviderRowTone; label: string; live?: boolean }>;
  /** The address or account the connection reads, truncated on one line. */
  account?: ReactNode;
  /** One wrapping line of chips and quiet facts such as the last sync. */
  facts?: ReactNode;
  /**
   * Why the row needs someone, in one caption line under the facts. The only
   * place an unreachable account explains itself; its verb is `action`.
   */
  reason?: ReactNode;
  /** The one visible verb: Connect, Reconnect, Set up. */
  action?: ReactNode;
  /** An OverflowMenu holding every other verb, in words. */
  menu?: ReactNode;
  /** Whatever hangs under the row, aligned to its text column. */
  children?: ReactNode;
}>) {
  const listLevel = useContext(ListHeadingLevel);
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
          <Heading size="xsmall" as={headingLevel ?? listLevel ?? "span"}>
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
        {reason !== undefined && (
          <p className="provider-row-reason t-caption">{reason}</p>
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

/**
 * Rows of ProviderRow, with the hairline between them owned here, as SettingList
 * does. Each name is an `h3`, the level under a panel's title, unless the list
 * sits one level deeper and says `h4`; a row's own `headingLevel` wins.
 */
export function ProviderList({
  headingLevel = "h3",
  children,
}: Readonly<{ headingLevel?: ProviderHeadingLevel; children: ReactNode }>) {
  return (
    <ListHeadingLevel.Provider value={headingLevel}>
      <div className="provider-list">{children}</div>
    </ListHeadingLevel.Provider>
  );
}
