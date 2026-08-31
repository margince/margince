// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Pencil, Plus, X } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, TextInput } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { EvidenceMark } from "../design-system/evidencemark";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody } from "../design-system/panel";
import { Select, type SelectOption } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryStates, throwProblem } from "./common";
import { isTechnicalFact } from "./companytechnical";
import { derivedSource } from "./evidencesource";
import { EvidenceVerdict, factClaim } from "./evidenceverdict";
import { type FactGroup, factFieldLabelKey, listFacts } from "./factview";
import "./company360.css";

type OrganizationFact = components["schemas"]["OrganizationFact"];
type FactCategory = OrganizationFact["category"];
type FactField = OrganizationFact["field"];

const FACT_CATEGORY_LABELS: Record<FactCategory, MessageKey> = {
  company: "org.factCategory.company",
  offering: "org.factCategory.offering",
  market: "org.factCategory.market",
  signal: "org.factCategory.signal",
};

type FactSuspectReason = NonNullable<OrganizationFact["suspect_reason"]>;

const FACT_SUSPECT_LABELS: Record<FactSuspectReason, MessageKey> = {
  phone_shaped_location: "co.factSuspect.phoneShapedLocation",
  not_a_phone: "co.factSuspect.notAPhone",
  not_a_year: "co.factSuspect.notAYear",
  not_an_email: "co.factSuspect.notAnEmail",
  not_a_size: "co.factSuspect.notASize",
};

// What a person may state, category by category. Taken from the contract's own
// enum rather than respelled, so a field added upstream appears here and a
// field removed there fails the build rather than offering the reader a choice
// the server refuses.
//
// The technical fields are absent on purpose: they are read from DNS and
// certificates rather than from anything a person knows, TechnicalProfileCard
// owns them, and a hand-stated "hosting provider" would contradict the lookup
// the next site read runs.
const STATEABLE: Readonly<Record<FactCategory, readonly FactField[]>> = {
  company: [
    "founded_year",
    "employee_range",
    "phone",
    "contact_email",
    "location",
  ],
  offering: ["service", "product", "capability"],
  market: ["served_industry", "company_size", "geography", "language"],
  signal: ["certification", "partner", "named_customer", "quantified_outcome"],
};

export function factsKey(orgId: string) {
  return ["org-facts", orgId] as const;
}

// How many rows of a category are shown before the reader asks for the rest. A
// real account returns ninety-odd facts, and rendering them all made this card
// taller than the page beside it — at which point nobody reads any of it.
const FACT_PREVIEW = 5;

/**
 * CompanyFactsPanel: what a machine read about this company, and what a person
 * states about it, in one list a person can add to and take from.
 *
 * EVERY STORED ROW IS DRAWN, which is the difference from the card this
 * replaces. That card collapsed duplicate spellings of one offering into the
 * best of them, which is right for reading and wrong the moment a row can be
 * REMOVED: the collapse drops the losers entirely, so a reader who deletes the
 * winner watches a row they have never seen take its place — having deleted
 * something and apparently changed nothing.
 */
export function CompanyFactsPanel({
  orgId,
  canEdit,
  reasonId,
  onOpenHistory,
}: Readonly<{
  orgId: string;
  canEdit: boolean;
  // The one sentence saying why this reader may not write, already on the page.
  // Every refused control points at it rather than carrying its own copy.
  reasonId?: string;
  onOpenHistory?: () => void;
}>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const [adding, setAdding] = useState(false);
  const factsQuery = useQuery({
    queryKey: factsKey(orgId),
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations/{id}/facts", {
        params: { path: { id: orgId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data ?? [];
    },
  });

  // The technical fields are excluded because the technical card claims them.
  // Both read the same endpoint, so without this every mail provider and
  // operated service renders twice on one tab, and a reader correcting one copy
  // watches the other disagree.
  const facts = (factsQuery.data ?? []).filter(
    (fact) => !isTechnicalFact(fact),
  );

  return (
    <Panel
      title={t("co.facts.title")}
      titleAction={
        <Button
          small
          onClick={() => setAdding((was) => !was)}
          reasonId={canEdit ? undefined : reasonId}
          unavailable={!canEdit}
        >
          {t("co.facts.add")}
        </Button>
      }
    >
      <PanelBody>
        {adding && (
          <AddFactForm orgId={orgId} onDone={() => setAdding(false)} />
        )}
        <QueryStates query={factsQuery}>
          {facts.length === 0 ? (
            <p className="t-caption">{t("co.facts.empty")}</p>
          ) : (
            listFacts(facts, t, locale).map((group) => (
              <FactCategoryBlock
                key={group.category}
                orgId={orgId}
                group={group}
                canEdit={canEdit}
                onOpenHistory={onOpenHistory}
              />
            ))
          )}
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}

function FactCategoryBlock({
  orgId,
  group,
  canEdit,
  onOpenHistory,
}: Readonly<{
  orgId: string;
  group: FactGroup;
  canEdit: boolean;
  onOpenHistory?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [expanded, setExpanded] = useState(false);
  const hidden = group.facts.length - FACT_PREVIEW;
  const shown = expanded ? group.facts : group.facts.slice(0, FACT_PREVIEW);
  return (
    <div className="co-facts-group">
      <div className="t-label co-facts-heading">
        {t(FACT_CATEGORY_LABELS[group.category])}
      </div>
      {shown.map((fact) => (
        <FactRow
          key={`${fact.field}:${fact.value_key}`}
          orgId={orgId}
          fact={fact}
          canEdit={canEdit}
          onOpenHistory={onOpenHistory}
        />
      ))}
      {hidden > 0 && (
        <Button small onClick={() => setExpanded(!expanded)}>
          {expanded
            ? t("co.facts.showLess")
            : t("co.facts.showAll", {
                count: formatNumber(group.facts.length, locale),
              })}
        </Button>
      )}
    </div>
  );
}

function FactRow({
  orgId,
  fact,
  canEdit,
  onOpenHistory,
}: Readonly<{
  orgId: string;
  fact: OrganizationFact;
  canEdit: boolean;
  onOpenHistory?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [removing, setRemoving] = useState(false);
  return (
    <div className="co-field">
      <span className="t-label">{t(factFieldLabelKey(fact.field))}</span>
      <div>
        <EvidenceMark
          value={fact.value}
          source={derivedSource(fact, locale, recordZone)}
          onOpenHistory={onOpenHistory}
        />
        {/* The value contradicts its own field — a phone number filed as a
            location, a register number filed as a headcount. The fact is still
            shown with its evidence: hiding it would be a worse answer than
            flagging it, and the reader is the one who can tell. */}
        {fact.suspect_reason && (
          <span className="co-fact-suspect">
            <Badge tone="warn">
              {t(FACT_SUSPECT_LABELS[fact.suspect_reason])}
            </Badge>
          </span>
        )}
        <EvidenceVerdict
          orgId={orgId}
          claim={factClaim(orgId, fact)}
          canEdit={canEdit}
        />
        {/* Removal is the verb correction cannot spell. A correction says "this
            value is wrong"; this says "this is not a fact about this company",
            which is the honest answer to a customer who left or a phone number
            read off the wrong page. */}
        {canEdit && (
          <IconAction
            label={t("co.facts.remove", {
              value: fact.value,
            })}
            icon={<X aria-hidden />}
            small
            onClick={() => setRemoving(true)}
          />
        )}
      </div>
      {removing && (
        <RemoveFactConfirm
          orgId={orgId}
          fact={fact}
          onClose={() => setRemoving(false)}
        />
      )}
    </div>
  );
}

function RemoveFactConfirm({
  orgId,
  fact,
  onClose,
}: Readonly<{
  orgId: string;
  fact: OrganizationFact;
  onClose: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const remove = useMutation({
    // The fact travels as a variable rather than through this closure: the
    // click belongs to the committed render, so what it passes cannot be older
    // than the row that carried it.
    mutationFn: async (doomed: OrganizationFact) => {
      const { error } = await api.DELETE(
        "/organizations/{id}/facts/{factKey}",
        {
          params: {
            path: {
              id: orgId,
              factKey: `${doomed.field}:${doomed.value_key}`,
            },
            ...ifMatch(requireVersion(doomed.version)),
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await settleFacts(queryClient, orgId);
      onClose();
    },
  });
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("co.facts.removeTitle")}
      confirmLabel={t("co.facts.removeConfirm")}
      confirmVariant="danger"
      pending={remove.isPending}
      error={remove.error ? problemMessageOf(remove.error, t) : undefined}
      onConfirm={() => remove.mutate(fact)}
    >
      <p>
        {t("co.facts.removeAsk", {
          field: t(factFieldLabelKey(fact.field)),
          value: fact.value,
        })}
      </p>
    </ConfirmModal>
  );
}

function AddFactForm({
  orgId,
  onDone,
}: Readonly<{ orgId: string; onDone: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const fieldId = useId();
  const valueId = useId();
  const [field, setField] = useState<string>("");
  const [value, setValue] = useState("");

  const options: SelectOption[] = Object.entries(STATEABLE).flatMap(
    ([category, fields]) =>
      fields.map((one) => ({
        value: `${category}:${one}`,
        label: t(factFieldLabelKey(one)),
      })),
  );

  const add = useMutation({
    mutationFn: async (stated: { field: string; value: string }) => {
      const [category, name] = stated.field.split(":");
      const { error } = await api.POST("/organizations/{id}/facts", {
        params: { path: { id: orgId } },
        body: {
          category: category as FactCategory,
          field: name,
          value: stated.value.trim(),
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: async () => {
      await settleFacts(queryClient, orgId);
      setField("");
      setValue("");
      onDone();
    },
  });

  return (
    <div className="co-facts-add">
      <label className="sr-only" htmlFor={fieldId}>
        {t("co.facts.addField")}
      </label>
      <Select
        id={fieldId}
        options={options}
        value={field}
        onChange={setField}
        placeholder={t("co.facts.addField")}
      />
      <label className="sr-only" htmlFor={valueId}>
        {t("co.facts.addValue")}
      </label>
      <TextInput
        id={valueId}
        value={value}
        placeholder={t("co.facts.addValue")}
        onChange={(event) => setValue(event.target.value)}
      />
      <IconAction
        label={t("co.facts.addSave")}
        icon={<Check aria-hidden />}
        variant="primary"
        small
        pending={add.isPending}
        // Both halves are required: a fact with no field names nothing, and one
        // with no value states nothing.
        reason={field && value.trim() ? undefined : t("co.facts.addIncomplete")}
        onClick={() => add.mutate({ field, value })}
      />
      <IconAction
        label={t("co.facts.addCancel")}
        icon={<X aria-hidden />}
        small
        onClick={onDone}
      />
      {add.error && (
        <span role="alert" className="form-error">
          {problemMessageOf(add.error, t)}
        </span>
      )}
    </div>
  );
}

// Everything that describes this account's facts, refreshed together. The keys
// are the ones the CONSUMERS register: React Query matches segments exactly, so
// a near-miss spelling invalidates nothing and the page goes on showing the row
// a reader just removed.
async function settleFacts(
  queryClient: ReturnType<typeof useQueryClient>,
  orgId: string,
) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: factsKey(orgId) }),
    queryClient.invalidateQueries({ queryKey: ["organization", orgId] }),
    queryClient.invalidateQueries({ queryKey: ["organization360", orgId] }),
    queryClient.invalidateQueries({ queryKey: ["organizations"] }),
  ]);
}

// Pencil is imported for the correction affordance EvidenceVerdict draws; the
// import keeps the icon set for this panel in one place.
export const FACT_EDIT_ICON = Pencil;
export const FACT_ADD_ICON = Plus;
