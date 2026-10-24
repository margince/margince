// The lead Details form: which identity fields it offers, and what a submit
// sends. Status and score have their own domain actions and are not here.
//
// Apart from the screen because leads.tsx is large. This is the part with no
// rendering in it, so it reads and tests on its own.

import type { components } from "../../api/schema";
import type { CreateField } from "../../components/create";

type UpdateLeadRequest = components["schemas"]["UpdateLeadRequest"];

function stringField(value: unknown): string {
  return typeof value === "string" ? value : "";
}

// An absent key is left absent rather than cleared. The form said nothing about
// it, which is not a request to empty it.
export function mapLeadUpdate(
  values: Record<string, unknown>,
): UpdateLeadRequest {
  const field = (key: string) =>
    Object.hasOwn(values, key)
      ? stringField(values[key]).trim() || null
      : undefined;
  return {
    full_name: field("full_name"),
    email: field("email"),
    phone: field("phone"),
    title: field("title"),
    company_name: field("company_name"),
    origin: field("origin"),
    origin_detail: field("origin_detail"),
    candidate_company_key: field("candidate_company_key"),
    project_id: field("project_id"),
    owner_id: stringField(values.owner_id).trim() || undefined,
  };
}

export const leadEditFields: CreateField[] = [
  { key: "full_name", label: "create.fullName", required: true },
  { key: "email", label: "create.email", type: "email" },
  // Plain text, as a contact's number is. The server normalises to E.164
  // through values.ParsePhone. A `tel` input would let a browser offer a local
  // format the writer then refuses.
  { key: "phone", label: "create.phone" },
  { key: "title", label: "create.contactTitle" },
  { key: "company_name", label: "create.companyName" },
];
