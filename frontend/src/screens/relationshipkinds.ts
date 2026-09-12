import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type RelationshipKind = components["schemas"]["Relationship"]["kind"];

// Every kind the product can SHOW, with the word it shows. Wider than what any
// one form may create — a kind is readable as soon as a row of it can exist,
// and a row can exist from a surface other than the form reading it.
export const KIND_LABELS: Record<RelationshipKind, MessageKey> = {
  employment: "rel.kind.employment",
  deal_stakeholder: "rel.kind.dealStakeholder",
  project_stakeholder: "rel.kind.projectStakeholder",
  // Readable, never creatable through the generic form: a company's place on a
  // project is written through the project's own surface, which holds write
  // authority over the project row and the refusal that keeps its last company
  // on it.
  project_company: "rel.kind.projectCompany",
  partner_of: "rel.kind.partnerOf",
  referred_by: "rel.kind.referredBy",
  co_sell_with: "rel.kind.coSellWith",
  works_with: "rel.kind.worksWith",
  // Readable so an existing edge renders as a word rather than a token. The
  // generic form does not offer it: its role is required and bounded, and that
  // form has a free-text role box. It is written from the billing panel on the
  // company's finance tab, which has the control this one has not.
  billing_contact: "rel.kind.billingContact",
};
