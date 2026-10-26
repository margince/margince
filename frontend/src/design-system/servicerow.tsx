// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { type BADGE_TONES, Badge } from "./atoms";
import "./servicerow.css";

/**
 * A service's status as a Badge tone: Connected `success`, Not connected or
 * Syncing `default`, Needs attention `warning`, Failed `danger`. Blue is left
 * out because it never means a status.
 */
export const SERVICE_ROW_TONES = [
  "default",
  "success",
  "warning",
  "danger",
] as const satisfies readonly (typeof BADGE_TONES)[number][];

export type ServiceRowTone = (typeof SERVICE_ROW_TONES)[number];

/**
 * One thing an account brings in or asks to read (new mail, past email, the
 * calendar). It has a glyph, a name, one caption, then at most one status and
 * one verb. A ProviderRow is the account; these are the lines under it.
 *
 * A row that needs attention says so once, with the amber status plus
 * `reason` and never a second badge. Copy arrives translated.
 */
export function ServiceRow({
  icon: Icon,
  name,
  caption,
  status,
  reason,
  action,
  testId,
  children,
}: Readonly<{
  icon: LucideIcon;
  name: string;
  /** What the service is for, and a quiet fact such as the last sync. */
  caption?: ReactNode;
  status?: Readonly<{ tone: ServiceRowTone; label: string; live?: boolean }>;
  /** One line that says why the status is not Connected. */
  reason?: ReactNode;
  /** The one visible verb: Connect, Reconnect, Import. */
  action?: ReactNode;
  testId?: string;
  /** What the service reports beyond its caption: progress, a link onward. */
  children?: ReactNode;
}>) {
  const hasSide = status !== undefined || action !== undefined;
  return (
    <li className="service-row" data-testid={testId}>
      <span className="service-row-icon" aria-hidden="true">
        <Icon />
      </span>
      <div className="service-row-main">
        <span className="service-row-name">{name}</span>
        {caption !== undefined && <span className="t-caption">{caption}</span>}
        {reason !== undefined && (
          <span className="t-caption service-row-reason">{reason}</span>
        )}
        {children}
      </div>
      {hasSide && (
        <div className="service-row-side">
          {status !== undefined && (
            <Badge tone={status.tone} live={status.live}>
              {status.label}
            </Badge>
          )}
          {action}
        </div>
      )}
    </li>
  );
}

/** Rows of ServiceRow in one framed list, the hairline between rows owned here. */
export function ServiceList({
  label,
  children,
}: Readonly<{
  /** The list's accessible name, such as what one account brings in. */
  label: string;
  children: ReactNode;
}>) {
  return (
    <ul className="service-list" aria-label={label}>
      {children}
    </ul>
  );
}
