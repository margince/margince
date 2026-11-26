// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The other side of a relationship, found through its record list.
//
// One file because it is one question asked three ways: a company, a contact or
// a deal, each reached through its own list endpoint and each narrowed the way
// that endpoint narrows. The dialog that uses them cares only which entity the
// chosen edge points at.

import { api } from "../api/client";
import type { components } from "../api/schema";
import type { EntityKind } from "./entity";
import { unwrap } from "./problem";

export type Candidate = {
  id: string;
  name: string;
  companyId?: string;
  detail?: string;
  dealStatus?: string;
};

// include_anchor: recording that a contact works at the company running the CRM
// is an ordinary, frequent fact. The list hides the own company by default
// because it answers "which companies are we selling to"; this question is a
// different one, so it opts back in (ADR-0082).
async function searchCompanyCandidates(q: string): Promise<Candidate[]> {
  const data = unwrap(
    await api.GET("/companies", {
      params: { query: { q, limit: 10, include_anchor: true } },
    }),
  );
  return data.data.map((company) => ({
    id: company.id,
    name: company.display_name,
  }));
}

async function searchContactCandidates(q: string): Promise<Candidate[]> {
  const data = unwrap(
    await api.GET("/contacts", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data.map((contact) => ({
    id: contact.id,
    name: contact.full_name,
  }));
}

async function searchDealCandidates(
  q: string,
  companyIds: readonly string[],
): Promise<Candidate[]> {
  const query = q.trim();
  const scopes: (string | undefined)[] = [...new Set(companyIds)];
  if (query || scopes.length === 0) scopes.push(undefined);
  const statuses: components["schemas"]["Deal"]["status"][] = query
    ? ["open", "won", "lost"]
    : ["open"];
  const pages = await Promise.all(
    scopes.flatMap((companyId) =>
      statuses.map(async (status) =>
        unwrap(
          await api.GET("/deals", {
            params: {
              query: {
                q: query || undefined,
                company_id: companyId,
                status,
                limit: 20,
              },
            },
          }),
        ),
      ),
    ),
  );
  const seen = new Set<string>();
  return pages
    .flatMap((page) => page.data)
    .filter((deal) => {
      if (seen.has(deal.id)) return false;
      seen.add(deal.id);
      return true;
    })
    .sort(
      (a, b) =>
        Number(a.status === "won" || a.status === "lost") -
        Number(b.status === "won" || b.status === "lost"),
    )
    .map((deal) => ({
      id: deal.id,
      name: deal.name,
      companyId: deal.company_id ?? undefined,
      dealStatus: deal.status,
    }));
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
  companyIds: readonly string[] = [],
): Promise<Candidate[]> {
  switch (entity) {
    case "company":
      return searchCompanyCandidates(query);
    case "contact":
      return searchContactCandidates(query);
    case "deal":
      return searchDealCandidates(query, companyIds);
    default:
      // Relationship edges only ever anchor contact/company/deal (see
      // edgeOptions below) — `lead`/`user`/`team` are EntityRefKind additions
      // for record refs elsewhere (EntityRef), not creatable relationship
      // endpoints, so this branch is unreachable for any real EdgeOption but
      // still needs to satisfy the now-widened union's exhaustiveness check.
      return Promise.resolve([]);
  }
}
