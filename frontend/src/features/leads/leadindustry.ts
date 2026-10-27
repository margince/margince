// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";

type Lead = components["schemas"]["Lead"];

// The rule the server's `leadIndustrySQL` applies to the industry filter: the
// linked company's industry when it has one, else the lead's own.
export function leadIndustry(
  lead: Lead,
): Readonly<{ code: string; from: "company" | "lead" }> | null {
  const company = lead.company_facts?.industry;
  if (company) {
    return { code: company, from: "company" };
  }
  return lead.industry ? { code: lead.industry, from: "lead" } : null;
}
