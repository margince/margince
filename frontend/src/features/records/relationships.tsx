// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { CreateField } from "../../components/create";
import {
  Button,
  Field,
  Modal,
  SearchField,
  TextInput,
} from "../../design-system/atoms";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { Heading } from "../../design-system/heading";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isOption } from "../../lib/options";
import { unwrap } from "../../lib/problem";
import {
  BILLING_ROLES,
  DEAL_ROLES,
  dealRoleLabel,
  PROJECT_ROLES,
  type RoleVocabulary,
  roleFromTyped,
} from "../../lib/recordlabels";
import {
  type Candidate,
  searchByEntity,
} from "../../lib/relationshipcandidates";
import { KIND_LABELS } from "../../lib/relationshipkinds";
import { SEARCH_HIT_KIND_KEY } from "../../lib/searchkinds";
import "./candidatepicker.css";
import "../../design-system/querystates.css";
import "./relationships.css";
import { ErrorLine } from "../../design-system/errorline";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import {
  type EdgeOption,
  edgeOptions,
  endpointBody,
  invalidateAfterEdge,
  noKindFor,
  scopeCopy,
  scopeQuery,
} from "./relationships.edges";

// The Relationships tab (P-5): the one surface a contact/company 360 renders
// its relationship edges through (employment, deal stakeholder, partner-of,
// referred-by, co-sell-with). There is no GET /relationships/{id} in the
// contract — every row is hydrated straight off the list read, so edit and
// remove both act on the row already in hand rather than a re-fetch.

export {
  counterpartyRef,
  dateRange,
  type EdgeOption,
  edgeOptions,
  endpointBody,
  fetchRelationships,
  invalidateAfterEdge,
  scopeCopy,
  scopeQueryKey,
} from "./relationships.edges";

export type Relationship = components["schemas"]["Relationship"];

export type CreateRelationshipRequest =
  components["schemas"]["CreateRelationshipRequest"];

// Which 360 this tab is rendered from — fixes which side of the edge is
// "this record" and which is the picked "other side".
//
// A deal is a scope of its own, so its stakeholders are added from the deal
// page. `GET /relationships` filters on deal_id for this reading.
export type RelationshipScope =
  | { contact_id: string }
  | { company_id: string }
  | { deal_id: string };

// What this FORM may create, which is narrower than what it may show: the
// contract's create body omits project_company, so the type follows it and a
// picker offering the kind would not compile.
export type CreatableRelationshipKind = CreateRelationshipRequest["kind"];

// A role these kinds hold is shown in the reader's words and sent as its key.
// Any other edge's role is free text, such as a job title.
const ROLE_VOCABULARY: Partial<Record<Relationship["kind"], RoleVocabulary>> = {
  deal_stakeholder: DEAL_ROLES,
  project_stakeholder: PROJECT_ROLES,
  billing_contact: BILLING_ROLES,
};

export function roleShown(
  rel: Pick<Relationship, "kind" | "role">,
  t: (key: MessageKey) => string,
): string {
  if (!rel.role) {
    return "";
  }
  const vocabulary = ROLE_VOCABULARY[rel.kind];
  return vocabulary ? vocabulary.label(rel.role, t) : rel.role;
}

/** The role a form sends for `typed`; `offered` holds the role it opened with. */
export function roleSent(
  kind: Relationship["kind"],
  typed: string,
  t: (key: MessageKey) => string,
  offered: readonly string[] = [],
): string {
  const vocabulary = ROLE_VOCABULARY[kind];
  return vocabulary
    ? roleFromTyped(typed, t, vocabulary, offered)
    : typed.trim();
}

const SEARCH_DEBOUNCE_MS = 250;

// The "add relationship" affordance: kind + role + start date, plus the
// other-side target picker (mirrors merge.tsx's debounced search-and-pick —
// the source of the edge is fixed by scope, so there is no "exclude self"
// filtering here).
export function AddRelationshipAction({
  scope,
  refusedReasonId,
  only,
  variant,
  seatRole,
  describedBy,
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
  // The scope, picker, write and invalidation stay those of the relationships tab.
  only?: { kind: CreatableRelationshipKind; label: MessageKey };
  // "link" when the verb is a card's one link rather than a button.
  variant?: "link";
  // The role key the form opens with, for a verb offered for one missing seat.
  seatRole?: string;
  // The id of the words that say which seat this verb fills.
  describedBy?: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const headingId = useId();
  const all = edgeOptions(scope);
  const options = only
    ? all.filter((option) => option.kind === only.kind)
    : all;
  const defaultOption = options[0] ?? noKindFor(only?.kind);
  const scopeWords = scopeCopy(scope);
  const copy = only
    ? { ...scopeWords, add: only.label, singleKind: true }
    : scopeWords;
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState<CreatableRelationshipKind>(
    defaultOption.kind,
  );
  // Until the reader types, the offered seat shows in their words and is sent
  // as its key, the spelling the coverage rules count.
  const [typedRole, setTypedRole] = useState<string | null>(null);
  const seatLabel = seatRole ? dealRoleLabel(seatRole, t) : "";
  const role = typedRole ?? seatLabel;
  const sentRole =
    typedRole === null
      ? (seatRole ?? "")
      : roleSent(kind, typedRole, t, seatRole ? [seatRole] : []);
  const [startedAt, setStartedAt] = useState<ISODate | "">("");
  const [term, setTerm] = useState("");
  const [candidates, setCandidates] = useState<Candidate[]>([]);
  const [target, setTarget] = useState<Candidate | null>(null);
  // The caught failure itself, not a sentence about it: the effect below
  // runs debounced and must not depend on the translator, which is a new
  // function every render. It is turned into copy where it is rendered.
  const [searchFailure, setSearchFailure] = useState<unknown>(null);
  // Kind fixes the picked endpoint; a stale kind can't outlive its scope
  // because the tab remounts per record, so the first option is always valid.
  const endpoint = options.find((o) => o.kind === kind) ?? defaultOption;
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
      return unwrap(await api.POST("/relationships", { body }));
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
    setKind(defaultOption.kind);
    setTypedRole(null);
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
        variant={variant}
        reasonId={refusedReasonId}
        aria-describedby={describedBy}
        onClick={() => setOpen(true)}
        data-testid="add-relationship"
      >
        {t(copy.add)}
      </Button>
      <Modal open={open} onClose={close} labelledBy={headingId} intent="form">
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t(copy.add)}
        </Heading>
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
          {/* The counterparty comes before Role and Started: a reader picks
              WHAT they are linking to before describing the edge, and the
              caption names the kind being searched rather than a generic
              "other side", reusing the same singular each search result
              screen already carries for its own kind. */}
          <p className="t-caption">{t(SEARCH_HIT_KIND_KEY[entity])}</p>
          <SearchField
            placeholder={t("merge.searchPlaceholder")}
            aria-label={t("merge.searchPlaceholder")}
            value={term}
            onChange={(event) => {
              setTerm(event.target.value);
              setTarget(null);
            }}
          />
          <ErrorLine error={searchFailure} />
          <ul className="relationships-candidates">
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
          <Field label={t("rel.role")}>
            {(control) => (
              <TextInput
                {...control}
                value={role}
                onChange={(event) => setTypedRole(event.target.value)}
              />
            )}
          </Field>
          <Field label={t("rel.startedAt")}>
            {(control) => (
              <DateInput
                {...control}
                value={startedAt}
                onChange={(event) => setStartedAt(event.target.value)}
              />
            )}
          </Field>
          {target && (
            <p>
              {t("rel.addConfirm", {
                target: target.name,
                kind: t(KIND_LABELS[kind]),
              })}
            </p>
          )}
          <ErrorLine error={mutation.error} />
        </div>
        <div className="actions">
          <Button onClick={close} disabled={mutation.isPending}>
            {t("create.cancel")}
          </Button>
          <Button
            variant="primary"
            disabled={!target || mutation.isPending}
            onClick={(event) => {
              if (target && !refuseUnreadableField(event.currentTarget)) {
                mutation.mutate({
                  target,
                  kind,
                  field: endpoint.field,
                  role: sentRole,
                  startedAt,
                });
              }
            }}
            data-testid="add-relationship-submit"
          >
            {t("create.save")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

export const relationshipEditFields: CreateField[] = [
  { key: "role", label: "rel.role" },
  { key: "started_at", label: "rel.startedAt", type: "date" },
  { key: "ended_at", label: "rel.endedAt", type: "date" },
];

// The backend applies update fields via coalesce($n, col), so null keeps the
// stored value and an emptied field stays as it was. `orNull` keeps "" off the wire.
export function orNull(value: unknown): string | null {
  const text = typeof value === "string" ? value.trim() : "";
  return text.length > 0 ? text : null;
}
