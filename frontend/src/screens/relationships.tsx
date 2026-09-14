// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useCanWrite } from "../app/capability";
import type { EntityKind } from "../app/entity";
import { isOption } from "../app/options";
import {
  Badge,
  Button,
  DataTable,
  EmptyState,
  Field,
  Modal,
  SearchField,
  TextInput,
} from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { DEAL_COVERAGE_KEY } from "./activitykeys";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import type { CreateField } from "./create";
import { EditAction } from "./edit";
import { EntityRef } from "./entityref";
import {
  type Candidate,
  type RelationshipEntity,
  searchByEntity,
} from "./relationshipcandidates";
import { KIND_LABELS } from "./relationshipkinds";
import "./candidatepicker.css";

// The Relationships tab (P-5): the one surface a contact/company 360 renders
// its relationship edges through (employment, deal stakeholder, partner-of,
// referred-by, co-sell-with). There is no GET /relationships/{id} in the
// contract — every row is hydrated straight off the list read, so edit and
// remove both act on the row already in hand rather than a re-fetch.

type Relationship = components["schemas"]["Relationship"];
type CreateRelationshipRequest =
  components["schemas"]["CreateRelationshipRequest"];

// Which 360 this tab is rendered from — fixes which side of the edge is
// "this record" and which is the picked "other side".
//
// A deal is a scope of its own, not a counterparty of somebody else's: a
// deal_stakeholder edge was creatable only from the CONTACT's side, so adding a
// champion to a deal meant knowing which contact to open first, and a deal
// nobody had thought to name from a contact page had no stakeholder surface at
// all. `GET /relationships` filters on deal_id for exactly this reading.
export type RelationshipScope =
  | { contact_id: string }
  | { company_id: string }
  | { deal_id: string };

// What this FORM may create, which is narrower than what it may show: the
// contract's create body omits project_company, so the type follows it and a
// picker offering the kind would not compile.
type CreatableRelationshipKind = CreateRelationshipRequest["kind"];

const SEARCH_DEBOUNCE_MS = 250;

function scopeQuery(scope: RelationshipScope): {
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

function scopeQueryKey(scope: RelationshipScope): [string, string, string] {
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
// contact's page says "relationship": the generic word sends a reader looking for
// a control the deal page does not have, and a Kind picker holding one option —
// or a Kind column repeating one badge down every row — asks a question with a
// single answer.
function scopeCopy(scope: RelationshipScope): {
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

async function fetchRelationships(
  scope: RelationshipScope,
): Promise<Relationship[]> {
  const { data, error } = await api.GET("/relationships", {
    params: { query: scopeQuery(scope) },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data;
}

// The other side of an edge from this scope's point of view, as a typed
// record reference EntityRef can hydrate into a name + backlink. The far end
// follows the edge shape (migration 0007 rel_*_shape) AND the scope: a
// contact's 360 sees its employment (→company) and deal_stakeholder (→deal) edges;
// a company's 360 sees employment (→contact) and the company↔company edges. Critically,
// the company list filter matches a company↔company edge on EITHER end
// (company_id OR counterparty_company_id), so the far company is whichever id is
// not this scope's own — never the record itself.
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
    // works_with names a contact on either column, and the contact filter now
    // matches either end — the far contact is whichever id is not this
    // scope's own, the same rule the company↔company branch below keeps.
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

function dateRange(rel: Relationship, t: (key: MessageKey) => string): string {
  const end = rel.ended_at ?? t("rel.current");
  return rel.started_at ? `${rel.started_at} – ${end}` : end;
}

// A creatable edge from this scope: the kind, which entity fills the picked
// endpoint, and which request field carries its id — all fixed by (scope,
// kind) per the rel_*_shape CHECKs (migration 0007). The anchor endpoint
// comes from scope (scopeQuery); this describes the rest.
export type EdgeOption = {
  kind: CreatableRelationshipKind;
  entity: RelationshipEntity;
  field: "company_id" | "contact_id" | "counterparty_company_id" | "deal_id";
};

// Only the kinds a scope can actually anchor are offered — a contact anchors
// employment (→company) and deal_stakeholder (→deal); a company anchors employment
// (→contact) and the three company↔company kinds (→counterparty company). Offering the
// rest would only earn an endpoint-shape 422.
export function edgeOptions(scope: RelationshipScope): EdgeOption[] {
  // A deal anchors its stakeholders and nothing else — employment is a fact
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

// Typed carrier for the picked endpoint id — a computed-key spread would
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
 * feeds the deal's coverage — the rail's seats, the committee map and the risk
 * chips all read `GET /deals/{id}/coverage`, and the map sits directly above
 * this panel on the deal page. Without this, seating a champion filled the table
 * while the map one panel up still said the champion was missing.
 *
 * The whole coverage prefix rather than one deal's: a stakeholder can be seated
 * from the CONTACT's page too, where the deal being changed is the picked target
 * rather than the scope, and a page that knows only "some deal moved" cannot
 * name which key to drop.
 */
function invalidateAfterEdge(
  queryClient: ReturnType<typeof useQueryClient>,
  touchesADeal: boolean,
) {
  queryClient.invalidateQueries({ queryKey: ["relationships"] });
  if (touchesADeal) {
    queryClient.invalidateQueries({ queryKey: DEAL_COVERAGE_KEY });
  }
}

// The "add relationship" affordance: kind + role + start date, plus the
// other-side target picker (mirrors merge.tsx's debounced search-and-pick —
// the source of the edge is fixed by scope, so there is no "exclude self"
// filtering here).
export function AddRelationshipAction({
  scope,
  refusedReasonId,
  only,
}: Readonly<{
  scope: RelationshipScope;
  // The id of the anchor page's sentence about why its record takes no
  // changes. An edge is written through the anchor's own write gate on the
  // server, so a deal this caller cannot write takes no stakeholder from them.
  refusedReasonId?: string;
  // ONE edge and the word for it, when the surface offering this verb is about
  // that edge alone. A contact's Deals tab is exactly that: the reader is there
  // to seat this contact on a deal, and a kind selector offering "employment"
  // beside it would ask them to answer a question the tab already answered.
  //
  // The scope is unchanged — it is still this contact — so the picker, the write
  // and the invalidation are the ones the relationships tab already uses. What
  // narrows is what this surface offers and what it calls it.
  only?: { kind: CreatableRelationshipKind; label: MessageKey };
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const headingId = useId();
  const all = edgeOptions(scope);
  const options = only
    ? all.filter((option) => option.kind === only.kind)
    : all;
  const scopeWords = scopeCopy(scope);
  const copy = only
    ? { ...scopeWords, add: only.label, singleKind: true }
    : scopeWords;
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<CreatableRelationshipKind>(options[0].kind);
  const [role, setRole] = useState("");
  const [startedAt, setStartedAt] = useState("");
  const [term, setTerm] = useState("");
  const [candidates, setCandidates] = useState<Candidate[]>([]);
  const [target, setTarget] = useState<Candidate | null>(null);
  // The caught failure itself, not a sentence about it: the effect below
  // runs debounced and must not depend on the translator, which is a new
  // function every render. It is turned into copy where it is rendered.
  const [searchFailure, setSearchFailure] = useState<unknown>(null);
  // Kind fixes the picked endpoint; a stale kind can't outlive its scope
  // because the tab remounts per record, so the first option is always valid.
  const endpoint = options.find((o) => o.kind === kind) ?? options[0];
  const entity = endpoint.entity;

  useEffect(() => {
    if (!open) {
      return;
    }
    const query = term.trim();
    if (!query) {
      setCandidates([]);
      setSearchFailure(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(async () => {
      try {
        const results = await searchByEntity(entity, query);
        if (!cancelled) {
          setCandidates(results);
          setSearchFailure(null);
        }
      } catch (error) {
        if (!cancelled) {
          setCandidates([]);
          setSearchFailure(error);
        }
      }
    }, SEARCH_DEBOUNCE_MS);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [open, term, entity]);

  // EVERY answer the form holds arrives as the mutation's variable, not through
  // this closure: react-query re-arms a mutation's options in a passive effect,
  // so a submit landing between the commit that enables the button and that
  // effect runs the previous render's function. The target alone used to travel,
  // which left the kind, the role and the start date one render behind — a write
  // stating choices nobody had made yet.
  const mutation = useMutation({
    mutationFn: async (chosen: {
      target: Candidate;
      kind: CreatableRelationshipKind;
      field: EdgeOption["field"];
      role: string;
      startedAt: string;
    }) => {
      const body: CreateRelationshipRequest = {
        kind: chosen.kind,
        role: chosen.role.trim() || undefined,
        started_at: chosen.startedAt || undefined,
        source: "manual",
        // Not sent at all. This form offers every relationship kind and no
        // primary control, so `false` here was a literal rather than anybody's
        // decision — and for an employment it silently blocked the server's own
        // rule that a contact's only current job is their current primary one.
        ...scopeQuery(scope),
        ...endpointBody(chosen.field, chosen.target.id),
      };
      const { data, error } = await api.POST("/relationships", { body });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (created) => {
      invalidateAfterEdge(queryClient, created.deal_id != null);
      close();
    },
  });

  // Switching kind can switch the target entity (company→deal→contact), so any
  // pending pick and search results from the old entity must clear.
  function selectKind(next: CreatableRelationshipKind) {
    setKind(next);
    setTerm("");
    setCandidates([]);
    setTarget(null);
    setSearchFailure(null);
  }

  function close() {
    setOpen(false);
    setKind(options[0].kind);
    setRole("");
    setStartedAt("");
    setTerm("");
    setCandidates([]);
    setTarget(null);
    setSearchFailure(null);
    mutation.reset();
  }

  return (
    <>
      <Button
        small
        reasonId={refusedReasonId}
        onClick={() => setOpen(true)}
        data-testid="add-relationship"
      >
        {t(copy.add)}
      </Button>
      <Modal open={open} onClose={close} labelledBy={headingId}>
        <h2 id={headingId} className="t-h2" style={{ marginBottom: 12 }}>
          {t(copy.add)}
        </h2>
        <div className="form-stack">
          {!copy.singleKind && (
            <Field label={t("rel.kind")}>
              {(control) => (
                <Select
                  {...control}
                  value={kind}
                  onChange={(value) => {
                    const kinds = options.map((o) => o.kind);
                    if (isOption(value, kinds)) selectKind(value);
                  }}
                  options={options.map((option) => ({
                    value: option.kind,
                    label: t(KIND_LABELS[option.kind]),
                  }))}
                />
              )}
            </Field>
          )}
          <Field label={t("rel.role")}>
            {(control) => (
              <TextInput
                {...control}
                value={role}
                onChange={(event) => setRole(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("rel.startedAt")}>
            {(control) => (
              <TextInput
                {...control}
                type="date"
                value={startedAt}
                onChange={(event) => setStartedAt(event.target.value)}
              />
            )}
          </Field>
          <p className="t-caption">{t("rel.pickCounterparty")}</p>
          <SearchField
            placeholder={t("merge.searchPlaceholder")}
            aria-label={t("merge.searchPlaceholder")}
            value={term}
            onChange={(event) => {
              setTerm(event.target.value);
              setTarget(null);
            }}
          />
          {searchFailure ? (
            <p className="t-caption" style={{ color: "var(--dangerText)" }}>
              {problemMessageOf(searchFailure, t)}
            </p>
          ) : null}
          <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {candidates.map((candidate) => (
              <li key={candidate.id}>
                <Button
                  className="candidate-option"
                  aria-pressed={target?.id === candidate.id}
                  onClick={() => setTarget(candidate)}
                >
                  {candidate.name}
                </Button>
              </li>
            ))}
          </ul>
          {target && (
            <p style={{ marginBottom: 4 }}>
              {t("rel.addConfirm", {
                target: target.name,
                kind: t(KIND_LABELS[kind]),
              })}
            </p>
          )}
          {mutation.isError && (
            <p className="t-caption" style={{ color: "var(--dangerText)" }}>
              {problemMessageOf(mutation.error, t)}
            </p>
          )}
          <div
            style={{
              display: "flex",
              gap: "var(--gapActions)",
              justifyContent: "flex-end",
            }}
          >
            <Button small onClick={close} disabled={mutation.isPending}>
              {t("create.cancel")}
            </Button>
            <Button
              small
              variant="primary"
              disabled={!target || mutation.isPending}
              onClick={() =>
                target &&
                mutation.mutate({
                  target,
                  kind,
                  field: endpoint.field,
                  role,
                  startedAt,
                })
              }
              data-testid="add-relationship-submit"
            >
              {t("create.save")}
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}

const relationshipEditFields: CreateField[] = [
  { key: "role", label: "rel.role" },
  { key: "started_at", label: "rel.startedAt", type: "date" },
  { key: "ended_at", label: "rel.endedAt", type: "date" },
];

// UpdateRelationshipRequest fields are nullable, but the backend applies them
// via coalesce($n, col) (backend/internal/modules/contacts/relationship.go):
// null means KEEP the stored value, never clear it. So role, started_at and
// ended_at can be set or changed here while an emptied one stays as it was —
// clearing needs the server to tell omit from explicit-null first. `orNull`
// still keeps an empty string off the wire.
function orNull(value: unknown): string | null {
  const text = typeof value === "string" ? value.trim() : "";
  return text.length > 0 ? text : null;
}

export function RelationshipsTab({
  scope,
  refusedReasonId,
}: Readonly<{
  scope: RelationshipScope;
  // See AddRelationshipAction: the anchor page's one read-only sentence,
  // which every write here points at when the anchor refuses changes.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const headingId = useId();
  const copy = scopeCopy(scope);
  // The object half of each verb's gate, asked as the server asks it
  // (relationship:create on an add, :update on an edit, :delete on a
  // removal). A verb the role holds no grant for is withheld outright — there
  // is no fact about the record to report — where the anchor's refusal above
  // keeps the verb and says why.
  const canCreate = useCanWrite("relationship", "create");
  const canUpdate = useCanWrite("relationship", "update");
  const canDelete = useCanWrite("relationship", "delete");
  const query = useQuery({
    queryKey: scopeQueryKey(scope),
    queryFn: () => fetchRelationships(scope),
  });

  // Two-step confirm, mirroring ArchiveAction (archive.tsx) — Remove is a
  // hard DELETE with no restore path, so it never fires from a single click.
  // The ROW, not its id: what the write invalidates depends on whether the edge
  // names a deal, and an id alone cannot answer that.
  const [removing, setRemoving] = useState<Relationship | null>(null);

  const remove = useMutation({
    mutationFn: async (doomed: Relationship) => {
      const { data, error } = await api.DELETE("/relationships/{id}", {
        params: { path: { id: doomed.id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_removed, doomed) => {
      invalidateAfterEdge(queryClient, doomed.deal_id != null);
      setRemoving(null);
    },
  });

  return (
    <Panel
      title={t(copy.title)}
      titleAction={
        canCreate ? (
          <AddRelationshipAction
            scope={scope}
            refusedReasonId={refusedReasonId}
          />
        ) : undefined
      }
    >
      <PanelBody>
        <QueryGate query={query} pendingLabel={t(copy.title)}>
          {(rows) =>
            rows.length === 0 ? (
              <EmptyState>{t(copy.empty)}</EmptyState>
            ) : (
              <DataTable
                label={t(copy.title)}
                columns={[
                  ...(copy.singleKind
                    ? []
                    : [
                        {
                          key: "kind",
                          header: t("rel.kind"),
                          render: (rel: Relationship) => (
                            <Badge>{t(KIND_LABELS[rel.kind])}</Badge>
                          ),
                        },
                      ]),
                  {
                    key: "role",
                    header: t("rel.role"),
                    render: (rel: Relationship) => rel.role ?? "",
                  },
                  {
                    key: "counterparty",
                    header: t("rel.counterparty"),
                    render: (rel: Relationship) => {
                      const ref = counterpartyRef(rel, scope);
                      return ref ? (
                        <EntityRef kind={ref.kind} id={ref.id} />
                      ) : (
                        <span className="t-mono">—</span>
                      );
                    },
                  },
                  {
                    key: "dates",
                    header: t("rel.dates"),
                    render: (rel: Relationship) => dateRange(rel, t),
                  },
                  {
                    key: "actions",
                    header: "",
                    render: (rel: Relationship) => (
                      <div style={{ display: "flex", gap: "var(--space-2)" }}>
                        {canUpdate && (
                          <EditAction
                            disabledReasonId={refusedReasonId}
                            label={t("record.edit")}
                            savedMessage={t("rel.saveDone")}
                            fields={relationshipEditFields}
                            record={{
                              id: rel.id,
                              version: rel.version,
                              role: rel.role ?? "",
                              started_at: rel.started_at ?? "",
                              ended_at: rel.ended_at ?? "",
                            }}
                            update={async (values, _rows, opened) => {
                              const { data, error } = await api.PATCH(
                                "/relationships/{id}",
                                {
                                  params: {
                                    path: { id: rel.id },
                                    ...ifMatch(requireVersion(opened?.version)),
                                  },
                                  body: {
                                    role: orNull(values.role),
                                    started_at: orNull(values.started_at),
                                    ended_at: orNull(values.ended_at),
                                  },
                                },
                              );
                              if (error) {
                                throwProblem(error);
                              }
                              return data;
                            }}
                            invalidate="relationships"
                            recordKey="relationship"
                          />
                        )}
                        {canDelete && (
                          <Button
                            small
                            variant="danger"
                            reasonId={refusedReasonId}
                            onClick={() => setRemoving(rel)}
                            data-testid="remove-relationship"
                          >
                            {t("rel.remove")}
                          </Button>
                        )}
                      </div>
                    ),
                  },
                ]}
                rows={rows}
                rowKey={(rel) => rel.id}
              />
            )
          }
        </QueryGate>
      </PanelBody>
      <Modal
        open={removing !== null}
        onClose={() => {
          setRemoving(null);
          remove.reset();
        }}
        labelledBy={headingId}
      >
        <h2 id={headingId} className="t-h2" style={{ marginBottom: 12 }}>
          {t("rel.remove")}
        </h2>
        <p style={{ marginBottom: 16 }}>{t("rel.removeConfirm")}</p>
        {remove.isError && (
          <p className="t-caption" style={{ color: "var(--dangerText)" }}>
            {problemMessageOf(remove.error, t)}
          </p>
        )}
        <div
          style={{
            display: "flex",
            gap: "var(--gapActions)",
            justifyContent: "flex-end",
          }}
        >
          <Button
            small
            onClick={() => setRemoving(null)}
            disabled={remove.isPending}
          >
            {t("create.cancel")}
          </Button>
          <Button
            small
            variant="danger"
            onClick={() => {
              if (removing) {
                remove.mutate(removing);
              }
            }}
            disabled={remove.isPending}
            data-testid="remove-relationship-confirm"
          >
            {t("rel.remove")}
          </Button>
        </div>
      </Modal>
    </Panel>
  );
}
