import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { ProblemError, unwrap } from "../../lib/problem";
import type { Lead } from "./leads.list";

type CreateLeadRequest = components["schemas"]["CreateLeadRequest"];

// Builds the create-lead request body: scalar fields trim to undefined when
// blank (never sent rather than sent empty). Lead email is a single string,
// not a repeatable list, so no rows channel is used here.
export function mapLeadBody(values: Record<string, string>): CreateLeadRequest {
  return {
    full_name: values.full_name?.trim() || undefined,
    email: values.email?.trim() || undefined,
    linkedin_url: values.linkedin_url?.trim() || undefined,
    title: values.title?.trim() || undefined,
    company_name: values.company_name?.trim() || undefined,
    industry: values.industry?.trim() || undefined,
    contact_id: values.contact_id || undefined,
    owner_id:
      values.owner_id === UNASSIGNED_OWNER
        ? undefined
        : values.owner_id?.trim() || undefined,
    status: "new",
    origin: values.origin?.trim() || undefined,
    origin_detail: values.origin_detail?.trim() || undefined,
    partner_company_id: values.partner_company_id || undefined,
  };
}

// The owner picker's "nobody yet" choice. A sentinel rather than the empty
// string because the field is required: the writer chooses the queue rather
// than skip the question. The server reads an omitted owner_id as an
// unassigned lead for routing or a claim to pick up.
export const UNASSIGNED_OWNER = "unassigned";

export async function createLead(
  values: Record<string, string>,
  customFields: Record<string, unknown>,
  t: (key: MessageKey) => string,
): Promise<Lead> {
  const body = mapLeadBody(values);
  const probes = [body.email, body.linkedin_url].filter(
    (value): value is string => Boolean(value),
  );
  for (const probe of probes) {
    const matches = unwrap(
      await api.GET("/leads", {
        params: { query: { q: probe, limit: 10 } },
      }),
      t,
    );
    const normalized = probe.toLowerCase().replace(/\/$/, "");
    const existing = matches.data.find(
      (lead) =>
        lead.email?.toLowerCase() === normalized ||
        lead.linkedin_url?.toLowerCase().replace(/\/$/, "") === normalized,
    );
    if (existing) {
      throw new ProblemError(
        {
          code: "duplicate_lead", // problem-code:client found by a search before the create
          detail: t("lead.duplicateFound"),
          details: { existing_id: existing.id },
        },
        t,
      );
    }
  }
  return unwrap(
    await api.POST("/leads", {
      body: { ...body, ...customFields },
    }),
    t,
  );
}
