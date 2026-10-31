import type { useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { MessageKey } from "../../i18n/en";
import { DEAL_COVERAGE_KEY } from "../../lib/activitykeys";
import type { EntityKind } from "../../lib/entity";
import { unwrap } from "../../lib/problem";
import type { RelationshipEntity } from "../../lib/relationshipcandidates";
import { KIND_LABELS } from "../../lib/relationshipkinds";
import type {
  CreatableRelationshipKind,
  CreateRelationshipRequest,
  Relationship,
  RelationshipScope,
} from "./relationships";

export function scopeQuery(scope: RelationshipScope): {
  contact_id?: string;
  company_id?: string;
  deal_id?: string;
} {
  if ("contact_id" in scope) {
    return { contact_id: scope.contact_id };
  }
  if ("deal_id" in scope) {
    return { deal_id: scope.deal_id };
  }
  return { company_id: scope.company_id };
}

export function scopeQueryKey(
  scope: RelationshipScope,
): [string, string, string] {
  if ("contact_id" in scope) {
    return ["relationships", "contact", scope.contact_id];
  }
  if ("deal_id" in scope) {
    return ["relationships", "deal", scope.deal_id];
  }
  return ["relationships", "company", scope.company_id];
}

// The words this panel uses on the record it is rendered from, and whether that
// record anchors a single kind.
//
// A deal anchors only deal_stakeholder, so it says "stakeholder" where a
// contact's page says "relationship". The generic word sends a reader looking
// for a control the deal page does not have. A Kind picker with one option, or a
// Kind column repeating one badge, asks a question with a single answer.
export function scopeCopy(scope: RelationshipScope): {
  title: MessageKey;
  add: MessageKey;
  empty: MessageKey;
  singleKind: boolean;
} {
  if ("deal_id" in scope) {
    return {
      title: "rel.dealStakeholders",
      add: "rel.addStakeholder",
      empty: "rel.dealStakeholdersEmpty",
      singleKind: true,
    };
  }
  return {
    title: "tab.relationships",
    add: "rel.add",
    empty: "rel.empty",
    singleKind: false,
  };
}

export async function fetchRelationships(
  scope: RelationshipScope,
): Promise<Relationship[]> {
  const data = unwrap(
    await api.GET("/relationships", {
      params: { query: scopeQuery(scope) },
    }),
  );
  return data.data;
}

// The other side of an edge from this scope's point of view, as a typed record
// reference EntityRef can hydrate into a name and backlink. The far end follows
// both the edge shape (migration 0007 rel_*_shape) and the scope. A contact's
// 360 sees its employment (→company) and deal_stakeholder (→deal) edges. A
// company's 360 sees employment (→contact) and the company↔company edges. The
// company filter matches a company↔company edge on either end, so the far
// company is whichever id is not this scope's own.
export function counterpartyRef(
  rel: Relationship,
  scope: RelationshipScope,
): { kind: EntityKind; id: string } | null {
  // From a deal, every edge is a contact: deal_stakeholder is the only kind the
  // deal_id filter can return (rel_*_shape, migration 0007).
  if ("deal_id" in scope) {
    return rel.contact_id ? { kind: "contact", id: rel.contact_id } : null;
  }
  if ("contact_id" in scope) {
    // works_with names a contact on either column, and the contact filter
    // matches either end. The far contact is whichever id is not this scope's
    // own, the same rule the company↔company branch below keeps.
    if (rel.kind === "works_with") {
      const farContact = [rel.contact_id, rel.counterparty_contact_id].find(
        (contactId) => contactId != null && contactId !== scope.contact_id,
      );
      return farContact ? { kind: "contact", id: farContact } : null;
    }
    if (rel.deal_id) {
      return { kind: "deal", id: rel.deal_id };
    }
    return rel.company_id ? { kind: "company", id: rel.company_id } : null;
  }
  if (rel.contact_id) {
    return { kind: "contact", id: rel.contact_id };
  }
  const far = [rel.counterparty_company_id, rel.company_id].find(
    (companyId) => companyId != null && companyId !== scope.company_id,
  );
  return far ? { kind: "company", id: far } : null;
}

// The word an edge is shown under, from this scope's end of it. A referred_by
// edge runs from the company referred to the partner that referred it, so on
// the partner's own page it reads the other way round.
export function kindLabelFor(
  rel: Relationship,
  scope: RelationshipScope,
): MessageKey {
  if (
    rel.kind === "referred_by" &&
    "company_id" in scope &&
    rel.counterparty_company_id === scope.company_id
  ) {
    return "rel.kind.referred";
  }
  return KIND_LABELS[rel.kind];
}

export function dateRange(
  rel: Relationship,
  t: (key: MessageKey) => string,
): string {
  const end = rel.ended_at ?? t("rel.current");
  return rel.started_at ? `${rel.started_at} – ${end}` : end;
}

// A creatable edge from this scope: the kind, which entity fills the picked
// endpoint, and which request field carries its id. The rel_*_shape CHECKs
// (migration 0007) fix all three by (scope, kind). The anchor endpoint comes
// from scope (scopeQuery); this describes the rest.
export type EdgeOption = {
  kind: CreatableRelationshipKind;
  entity: RelationshipEntity;
  field: "company_id" | "contact_id" | "counterparty_company_id" | "deal_id";
};

// Only the kinds a scope can anchor are offered, since the rest would earn an
// endpoint-shape 422. A contact anchors employment (→company) and
// deal_stakeholder (→deal). A company anchors employment (→contact) and the
// three company↔company kinds (→counterparty company).
export function edgeOptions(
  scope: RelationshipScope,
): [EdgeOption, ...EdgeOption[]] {
  // A deal anchors its stakeholders and nothing else. Employment is a fact
  // about a contact and a company, and the company↔company kinds name no deal.
  if ("deal_id" in scope) {
    return [
      { kind: "deal_stakeholder", entity: "contact", field: "contact_id" },
    ];
  }
  if ("contact_id" in scope) {
    return [
      { kind: "employment", entity: "company", field: "company_id" },
      { kind: "deal_stakeholder", entity: "deal", field: "deal_id" },
    ];
  }
  return [
    { kind: "employment", entity: "contact", field: "contact_id" },
    {
      kind: "partner_of",
      entity: "company",
      field: "counterparty_company_id",
    },
    {
      kind: "referred_by",
      entity: "company",
      field: "counterparty_company_id",
    },
    {
      kind: "co_sell_with",
      entity: "company",
      field: "counterparty_company_id",
    },
  ];
}

export function noKindFor(kind: string | undefined): never {
  throw new Error(`no ${kind ?? "relationship"} kind for this scope`);
}

// Typed carrier for the picked endpoint id. A computed-key spread would
// widen to an index signature the request type won't accept.
export function endpointBody(
  field: EdgeOption["field"],
  id: string,
): Partial<CreateRelationshipRequest> {
  switch (field) {
    case "company_id":
      return { company_id: id };
    case "contact_id":
      return { contact_id: id };
    case "counterparty_company_id":
      return { counterparty_company_id: id };
    case "deal_id":
      return { deal_id: id };
  }
}

/**
 * Every cached read a relationship write staleifies, invalidated in one place.
 *
 * `["relationships"]` is this panel's own list. A deal_stakeholder edge also
 * feeds the deal's coverage. The rail's seats, the committee map and the risk
 * chips all read `GET /deals/{id}/coverage`, and the map sits above this panel.
 * Without the coverage drop, a seated champion shows in the table while the map
 * still says the champion is missing.
 *
 * It drops the whole coverage prefix, not one deal's key. A stakeholder can be
 * seated from the contact's page, where the changed deal is the picked target
 * rather than the scope. A page that knows only "some deal moved" cannot name
 * which key to drop.
 */
export function invalidateAfterEdge(
  queryClient: ReturnType<typeof useQueryClient>,
  touchesADeal: boolean,
) {
  queryClient.invalidateQueries({ queryKey: ["relationships"] });
  if (touchesADeal) {
    queryClient.invalidateQueries({ queryKey: DEAL_COVERAGE_KEY });
  }
}
