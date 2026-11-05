// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { contact360Query } from "./contact.queries";
import { stillHeld } from "./employmentcurrency";
import { unwrap } from "./problem";

type Employment = components["schemas"]["Contact360Employment"];
type Page = NonNullable<
  components["schemas"]["Contact360"]["employments"]
>["page"];

export function nextEmploymentCursor(page: Page): string | undefined {
  if (!page.has_more) return undefined;
  if (!page.next_cursor)
    throw new Error("The employer list is incomplete. Try again.");
  return page.next_cursor;
}

export async function readEmploymentPage(contactId: string, cursor: string) {
  const data = unwrap(
    await api.GET("/relationships", {
      params: {
        query: { contact_id: contactId, kind: "employment", cursor, limit: 50 },
      },
    }),
  );
  nextEmploymentCursor(data.page);
  const roles: Employment[] = [];
  for (const row of data.data) {
    if (!row.company_id) continue;
    roles.push({
      relationship_id: row.id,
      company_id: row.company_id,
      role: row.role,
      is_current_primary: row.is_current_primary ?? false,
      employment_status: row.employment_status,
      started_at: row.started_at,
      ended_at: row.ended_at,
      started_precision: row.started_precision,
      ended_precision: row.ended_precision,
      version: row.version,
    });
  }
  return { roles, page: data.page };
}

// A withheld section is unknown; it must never be reported as having no employer.
export async function currentEmployerCompanyIds(
  client: QueryClient,
  contactId: string,
): Promise<string[] | undefined> {
  const view = await client.fetchQuery(contact360Query(contactId));
  if (!view.employments) return undefined;
  const roles = [...view.employments.data];
  let cursor = nextEmploymentCursor(view.employments.page);
  const seen = new Set<string>();
  while (cursor) {
    if (seen.has(cursor))
      throw new Error("The employer list did not finish. Try again.");
    seen.add(cursor);
    const page = await readEmploymentPage(contactId, cursor);
    roles.push(...page.roles);
    cursor = nextEmploymentCursor(page.page);
  }
  return [...new Set(roles.filter(stillHeld).map((role) => role.company_id))];
}
