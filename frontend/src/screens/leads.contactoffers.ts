// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A lead's identity, offered from the contacts the CRM already holds. A seller
// writing a lead for somebody they know types the start of the name or the
// address and picks them, rather than retyping a person and risking a typo that
// splits them in two.
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import type { CreateField } from "./create";
import type { NameOffer, NameOffers } from "./create.offered";

type Contact = components["schemas"]["Contact"];

/** The contact fields a lead takes, under the lead's own field names. */
export function leadFillsFrom(contact: Contact): Record<string, string> {
  const linkedin = contact.social?.linkedin;
  return {
    full_name: contact.full_name,
    email: contact.primary_email ?? "",
    title: contact.title ?? "",
    company_name: contact.employer?.company_name ?? "",
    linkedin_url: typeof linkedin === "string" ? linkedin : "",
  };
}

/** The contacts matching what was typed, at most ten. */
export async function searchContacts(query: string): Promise<Contact[]> {
  const { data, error } = await api.GET("/contacts", {
    params: { query: { q: query, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data;
}

/**
 * Contacts offered for one box: by name in the name box, by address in the
 * address box. Each offer carries the rest of the contact as fills, and its id
 * under `contact_id`, so the server fills what the form still lacks.
 */
function contactOffers(shown: "full_name" | "email"): NameOffers {
  return {
    pickedKey: "contact_id",
    pickedHint: "lead.create.fromContact",
    search: async (query) => {
      const offers: NameOffer[] = [];
      for (const contact of await searchContacts(query)) {
        const fills = leadFillsFrom(contact);
        if (!fills[shown]) continue;
        offers.push({
          value: contact.id,
          label: fills[shown],
          hint: shown === "email" ? fills.full_name : fills.email,
          fills,
        });
      }
      return offers;
    },
  };
}

export const leadIdentityFields: CreateField[] = [
  {
    key: "full_name",
    label: "create.fullName",
    required: true,
    offers: contactOffers("full_name"),
  },
  {
    key: "email",
    label: "create.email",
    type: "email",
    offers: contactOffers("email"),
  },
  { key: "linkedin_url", label: "create.linkedinUrl" },
  { key: "title", label: "create.contactTitle" },
  { key: "company_name", label: "create.companyName" },
];
