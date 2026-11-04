// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Panel } from "../../design-system/panel";

/** A report section's name, led by the mark of what it counts. */
export function ReportTitle({
  icon: Icon,
  children,
}: Readonly<{ icon: LucideIcon; children: ReactNode }>) {
  return (
    <span className="brief-report-card-title">
      <Icon aria-hidden="true" />
      {children}
    </span>
  );
}

/**
 * One section of the week as a card. Its name and a reading share the head;
 * what it lists sits on the card's own rows.
 */
export function ReportCard({
  icon,
  title,
  aside,
  footer,
  children,
}: Readonly<{
  icon: LucideIcon;
  title: string;
  aside?: ReactNode;
  footer?: ReactNode;
  children: ReactNode;
}>) {
  return (
    <Panel
      flatHead
      title={<ReportTitle icon={icon}>{title}</ReportTitle>}
      titleAction={aside}
      footer={footer}
    >
      {children}
    </Panel>
  );
}
