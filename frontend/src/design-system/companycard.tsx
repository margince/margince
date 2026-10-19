// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./companycard.css";

// One company a record is tied to, as a card: the mark, the name over its
// facts, a line of its own for whatever the record says about the tie, and the
// row's verbs at the far edge.
//
// The contact page's employers and the project page's companies are the same
// question — which companies is this record connected to — so they share the
// one card instead of each drawing a list of its own.
export function CompanyCard({
  logo,
  title,
  badges,
  facts,
  children,
  actions,
}: Readonly<{
  // The slot is kept when absent so a row without a mark still lines its name
  // up with the rows that have one.
  logo?: ReactNode;
  // The link to the company. Its hit area is stretched over the whole card, so
  // the logo and the facts open the same page the name does.
  title: ReactNode;
  // Beside the name: the tie's own status or role, and any provenance mark.
  badges?: ReactNode;
  facts?: ReactNode;
  // The rest of the tie, under the facts. Controls here stay above the stretched
  // link and keep their own press.
  children?: ReactNode;
  // The verbs, revealed on hover or focus; use an `OverflowMenu`.
  actions?: ReactNode;
}>) {
  return (
    <div className="company-card">
      <span className="company-card-logo">{logo}</span>
      <span className="company-card-body">
        <span className="company-card-name">
          {title}
          {badges}
        </span>
        {facts && <span className="company-card-facts t-caption">{facts}</span>}
        {children}
      </span>
      {actions && <span className="company-card-actions">{actions}</span>}
    </div>
  );
}
