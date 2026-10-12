// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../src/api/schema";
import { versionSkew } from "./records";
import { type Handler, type MockRequest, page } from "./server";

const owner = {
  name: "owner_id",
  type: "id",
  operators: ["eq", "neq", "in", "exists"],
  custom: false,
  references: "app_user",
};

// A linked field has no `contains`. A picker has to honour that narrowing.
const tag = {
  name: "tag",
  type: "id",
  operators: ["eq", "neq", "in", "exists"],
  custom: false,
  references: "tag",
};

// What each record type may be filtered on. Each operator set is what
// storekit.operatorsByType admits, narrowed by linkOperators on a join. An
// operator the engine refuses would let a spec build a tree the product cannot.
const vocabulary: Record<string, unknown[]> = {
  contact: [owner, tag],
  company: [
    owner,
    {
      name: "industry",
      type: "text",
      operators: ["eq", "neq", "in", "contains", "exists"],
      custom: false,
    },
    // A custom field (#1286), named as the physical column the wire carries.
    // The label lives in /custom-fields, joined on `column_name`.
    {
      name: "cf_fleet_size",
      type: "number",
      operators: ["eq", "neq", "gt", "gte", "lt", "lte", "in", "exists"],
      custom: true,
    },
    {
      name: "lifecycle",
      type: "picklist",
      operators: ["eq", "neq", "in", "exists"],
      custom: false,
      options: ["prospect", "customer", "churned"],
    },
    tag,
  ],
  deal: [
    owner,
    {
      name: "status",
      type: "picklist",
      operators: ["eq", "neq", "in", "exists"],
      custom: false,
      options: ["open", "won", "lost"],
    },
    tag,
  ],
};

export const filterVocabulary: Handler = ({ url, json }) => {
  const resource = url.searchParams.get("resource") ?? "contact";
  return json({ resource, fields: vocabulary[resource] ?? [owner] });
};

type Leaf = { field?: string; op?: string; value?: unknown };
type Predicate = Leaf & { and?: Predicate[]; or?: Predicate[] };

// The root a builder sends is always a group (segmentpredicate.ts). It is
// flattened, so the same filter matches however the reader nested it.
function leaves(node: Predicate | undefined): Leaf[] {
  if (node === undefined) {
    return [];
  }
  if (node.and || node.or) {
    return [...(node.and ?? []), ...(node.or ?? [])].flatMap(leaves);
  }
  return [node];
}

function isAuthoredFilter(resource: string | undefined, filter?: Predicate) {
  const clauses = leaves(filter);
  return (
    resource === "company" &&
    clauses.length === 1 &&
    clauses[0].field === "industry" &&
    clauses[0].op === "eq" &&
    clauses[0].value === "automotive"
  );
}

// Answered from the request body. Only the filter the spec authors gets rows.
// Any other request gets an empty answer, so a screen that sends the wrong
// filter cannot pass on another filter's count.
export const filterPreview: Handler = ({ route, json }) => {
  const asked: { resource?: string; filter?: Predicate } =
    route.request().postDataJSON() ?? {};
  if (!isAuthoredFilter(asked.resource, asked.filter)) {
    return json({
      resource: asked.resource ?? "company",
      match_count: 0,
      columns: [],
      rows: [],
      truncated: false,
    });
  }
  // Two rows against a count of 812, so `truncated` is true and the screen's
  // "showing N of 812" is checked. Both rows match the authored filter.
  return json({
    resource: "company",
    match_count: 812,
    columns: ["id", "name", "industry"],
    rows: [
      { id: "o1", name: "Brandt Automotive", industry: "automotive" },
      { id: "o2", name: "Kessler Fahrzeugbau", industry: "automotive" },
    ],
    truncated: true,
  });
};

// One store answers every read, so the library, a list's rail and an opened
// view agree after a write. Archived views show only on include_archived.
export const listViews: Handler = ({ url, json }, { views }) => {
  const resource = url.searchParams.get("resource");
  const archivedToo = url.searchParams.get("include_archived") === "true";
  return json(
    page(
      views.filter(
        (view) =>
          (archivedToo || !view.archived_at) &&
          (resource === null || view.resource === resource),
      ),
    ),
  );
};

export const createView: Handler = ({ route, json }, state) => {
  const asked: components["schemas"]["CreateSavedViewRequest"] = route
    .request()
    .postDataJSON();
  const created: components["schemas"]["SavedView"] = {
    id: `v-${state.views.length + 1}`,
    owner_id: "u1",
    shared_scope: "private",
    resource: asked.resource,
    name: asked.name,
    query: asked.query,
    version: 1,
  };
  state.views = [...state.views, created];
  return json(created, 201);
};

type SavedView = components["schemas"]["SavedView"];

// Held to If-Match like the deal advance: a missing or stale version fails
// the spec instead of writing over.
function patchView(
  { route, json }: MockRequest,
  state: { views: SavedView[] },
  existing: SavedView,
) {
  if (route.request().headers()["if-match"] !== String(existing.version)) {
    return json(versionSkew, 409);
  }
  const asked: components["schemas"]["UpdateSavedViewRequest"] = route
    .request()
    .postDataJSON();
  const updated = { ...existing, ...asked, version: existing.version + 1 };
  state.views = state.views.map((view) =>
    view.id === existing.id ? updated : view,
  );
  return json(updated);
}

// The contract archives rather than deletes, so a later read still finds it.
function archiveView(state: { views: SavedView[] }, existing: SavedView) {
  const archived = { ...existing, archived_at: "2026-09-13T08:00:00Z" };
  state.views = state.views.map((view) =>
    view.id === existing.id ? archived : view,
  );
  return archived;
}

export const viewById: Handler = (request, state) => {
  const { method, params, json } = request;
  const existing = state.views.find((view) => view.id === params[0]);
  if (!existing || (method !== "GET" && existing.archived_at)) {
    return json({ title: "Not Found", code: "not_found" }, 404);
  }
  if (method === "PATCH") {
    return patchView(request, state, existing);
  }
  if (method === "DELETE") {
    return json(archiveView(state, existing));
  }
  return json(existing);
};
