// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHO CAN BE THE OTHER SIDE OF AN EDGE, found by typing.
//
// One file because it is one question asked three ways: a company, a contact or
// a deal, each reached through its own list endpoint and each narrowed the way
// that endpoint narrows. The dialog that uses them cares only which entity the
// chosen edge points at.

import { api } from "../api/client";
import type { EntityKind } from "../app/entity";
import { throwProblem } from "./common";

export type Candidate = { id: string; name: string };

// include_anchor: recording that a contact works at the company running the CRM
// is an ordinary, frequent fact. The list hides the own company by default
// because it answers "which companies are we selling to"; this question is a
// different one, so it opts back in (ADR-0082).
async function searchCompanyCandidates(q: string): Promise<Candidate[]> {
  const { data, error } = await api.GET("/companies", {
    params: { query: { q, limit: 10, include_anchor: true } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((company) => ({
    id: company.id,
    name: company.display_name,
  }));
}

async function searchContactCandidates(q: string): Promise<Candidate[]> {
  const { data, error } = await api.GET("/contacts", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((contact) => ({ id: contact.id, name: contact.full_name }));
}

// /deals has no free-text `q` in the contract (only structured filters), so
// the stakeholder picker fetches a recent page and matches the typed term
// against the deal name client-side. Deals past that page aren't reached — an
// accepted PoC limit, scoped to the manual deal_stakeholder edge.
const DEAL_PICKER_PAGE = 50;

async function searchDealCandidates(q: string): Promise<Candidate[]> {
  const { data, error } = await api.GET("/deals", {
    params: { query: { limit: DEAL_PICKER_PAGE } },
  });
  if (error) {
    throwProblem(error);
  }
  const needle = q.toLowerCase();
  return data.data
    .filter((deal) => deal.name.toLowerCase().includes(needle))
    .slice(0, 10)
    .map((deal) => ({ id: deal.id, name: deal.name }));
}

// The entity kinds this tab can ever pick as a relationship's other side —
// company/contact/deal, which is what the rel_*_shape CHECKs admit. A lead has
// no relationship edges (it is promoted into a contact first) and a project
// seats its stakeholders through its own endpoint, so
// this narrows EntityKind rather than switching on a kind the module can
// never produce.
export type RelationshipEntity = Exclude<EntityKind, "lead" | "project">;

export function searchByEntity(
  entity: RelationshipEntity,
  query: string,
): Promise<Candidate[]> {
  switch (entity) {
    case "company":
      return searchCompanyCandidates(query);
    case "contact":
      return searchContactCandidates(query);
    case "deal":
      return searchDealCandidates(query);
    default:
      // Relationship edges only ever anchor contact/company/deal (see
      // edgeOptions below) — `lead`/`user`/`team` are EntityRefKind additions
      // for record refs elsewhere (EntityRef), not creatable relationship
      // endpoints, so this branch is unreachable for any real EdgeOption but
      // still needs to satisfy the now-widened union's exhaustiveness check.
      return Promise.resolve([]);
  }
}
