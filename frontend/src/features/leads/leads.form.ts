// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lead Details form: which identity fields it offers, and what a submit
// sends. Status and score have their own domain actions and are not here.
//
// Apart from the screen because leads.tsx is large. This is the part with no
// rendering in it, so it reads and tests on its own.

import type { components } from "../../api/schema";
import type { CreateField, CreateFieldOption } from "../../components/create";
import { salutationChoice } from "../../lib/personsalutation";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";

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
    salutation: salutationChoice(values),
    company_name: field("company_name"),
    industry: field("industry"),
    origin: field("origin"),
    origin_detail: field("origin_detail"),
    candidate_company_key: field("candidate_company_key"),
    project_id: field("project_id"),
    partner_company_id: field("partner_company_id"),
    owner_id: stringField(values.owner_id).trim() || undefined,
  };
}

/** The choices of the lead's select fields, each with its own unset option first. */
export type LeadFieldChoices = Readonly<{
  industries: CreateFieldOption[];
  salutations: CreateFieldOption[];
  origins: CreateFieldOption[];
  partners: CreateFieldOption[];
}>;

// Typed, never offered from the contacts: someone is either a lead or a
// contact, and the server refuses a contact's address.
export const LEAD_IDENTITY_CREATE_FIELDS: readonly CreateField[] = [
  {
    key: "full_name",
    label: "create.fullName",
    required: true,
    maxLength: TEXT_LIMITS.label,
  },
  { key: "email", label: "create.email", type: "email" },
  { key: "linkedin_url", label: "create.linkedinUrl" },
  { key: "title", label: "create.contactTitle", maxLength: TEXT_LIMITS.label },
  {
    key: "company_name",
    label: "create.companyName",
    maxLength: TEXT_LIMITS.label,
  },
];

export function leadEditFields(choices: LeadFieldChoices): CreateField[] {
  return [
    {
      key: "salutation",
      label: "create.salutation",
      type: "select",
      options: choices.salutations,
    },
    {
      key: "full_name",
      label: "create.fullName",
      required: true,
      maxLength: TEXT_LIMITS.label,
    },
    {
      key: "company_name",
      label: "create.companyName",
      maxLength: TEXT_LIMITS.label,
    },
    {
      key: "title",
      label: "create.contactTitle",
      maxLength: TEXT_LIMITS.label,
    },
    {
      key: "industry",
      label: "create.industry",
      type: "select",
      options: choices.industries,
    },
    { key: "email", label: "create.email", type: "email" },
    // Plain text, as a contact's number is. The server normalises to E.164
    // through values.ParsePhone. A `tel` input would let a browser offer a local
    // format the writer then refuses.
    { key: "phone", label: "create.phone" },
    {
      key: "origin",
      label: "lead.origin",
      type: "select",
      options: choices.origins,
    },
    { key: "origin_detail", label: "lead.originDetail" },
    {
      key: "partner_company_id",
      label: "lead.partner",
      type: "select",
      options: choices.partners,
    },
    { key: "linkedin_url", label: "create.linkedinUrl" },
    { key: "candidate_company_key", label: "record.companyRoutingKey" },
  ];
}
