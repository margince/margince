// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Pencil, Plus, X } from "lucide-react";
import { type ReactNode, useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { IconAction } from "../design-system/iconaction";
import { Panel, PanelBody, PanelGroupHead } from "../design-system/panel";
import { Select, type SelectOption } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { QueryStates, throwProblem } from "./common";
import { FactRow } from "./companyfactrow";
import { isTechnicalFact } from "./companytechnical";
import {
  type FactGroup,
  factCategoryLabelKey,
  factFieldLabelKey,
  listFacts,
} from "./factview";
import "./company360.css";
import { SurfaceState } from "../design-system/surfacestate";

type CompanyFact = components["schemas"]["CompanyFact"];
type FactCategory = CompanyFact["category"];
type FactField = CompanyFact["field"];

// What a contact may state, category by category. Taken from the contract's own
// enum rather than respelled, so a field added upstream appears here and a
// field removed there fails the build rather than offering the reader a choice
// the server refuses.
//
// The technical fields are absent on purpose: they are read from DNS and
// certificates rather than from anything a contact knows, TechnicalProfilePanel
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

export function factsKey(companyId: string) {
  return ["company-facts", companyId] as const;
}

// How many rows of a category are shown before the reader asks for the rest. A
// real account returns ninety-odd facts, and rendering them all made this card
// taller than the page beside it — at which point nobody reads any of it.
//
// THE CAP LIFTS WHEN THE READER CAN REMOVE A ROW, and that is not a preference.
// A truncated list has the same defect the collapse had: delete the fifth row
// and the sixth takes its place, so the reader has removed something and
// apparently changed nothing. Reading is served by hiding the tail; removing is
// not, and a surface that offers the verb has to show what the verb acts on.
const FACT_PREVIEW = 5;

/**
 * CompanyFactsPanel: what a machine read about this company, and what a contact
 * states about it, in one list a contact can add to and take from.
 *
 * EVERY STORED ROW IS DRAWN, which is the difference from the card this
 * replaces. That card collapsed duplicate spellings of one offering into the
 * best of them, which is right for reading and wrong the moment a row can be
 * REMOVED: the collapse drops the losers entirely, so a reader who deletes the
 * winner watches a row they have never seen take its place — having deleted
 * something and apparently changed nothing.
 */
export function CompanyFactsPanel({
  companyId,
  canEdit,
  reasonId,
  onOpenHistory,
}: Readonly<{
  companyId: string;
  canEdit: boolean;
  // The one sentence saying why this reader may not write, already on the page.
  // Every refused control points at it rather than carrying its own copy.
  reasonId?: string;
  onOpenHistory?: () => void;
}>): ReactNode {
  const t = useT();
  const { locale } = useLocale();
  const [adding, setAdding] = useState(false);
  // The record page is not remounted when the reader opens a different account
  // — CompanyScreen takes the id as a prop — so an open add form would carry
  // its draft across, and the save would write what was typed about one company
  // onto another. Resetting on the id is what makes the draft belong to the
  // account it was typed about.
  const [openFor, setOpenFor] = useState(companyId);
  if (openFor !== companyId) {
    setOpenFor(companyId);
    setAdding(false);
  }
  const factsQuery = useQuery({
    queryKey: factsKey(companyId),
    queryFn: async () => {
      const { data, error } = await api.GET("/companies/{id}/facts", {
        params: { path: { id: companyId } },
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
          onClick={() => setAdding((was) => !was)}
          reasonId={canEdit ? undefined : reasonId}
          unavailable={!canEdit}
        >
          {t("co.facts.add")}
        </Button>
      }
    >
      {adding && (
        <PanelBody>
          <AddFactForm
            companyId={companyId}
            canEdit={canEdit}
            onDone={() => setAdding(false)}
          />
        </PanelBody>
      )}
      <QueryStates query={factsQuery} pendingLabel={t("co.facts.title")}>
        {facts.length === 0 ? (
          <PanelBody>
            <SurfaceState
              state="empty"
              emptyLabel={t("co.facts.empty")}
              loadingLabel={t("co.facts.title")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        ) : (
          listFacts(facts, t, locale).map((group) => (
            <FactCategoryBlock
              key={group.category}
              companyId={companyId}
              group={group}
              canEdit={canEdit}
              onOpenHistory={onOpenHistory}
            />
          ))
        )}
      </QueryStates>
    </Panel>
  );
}

function FactCategoryBlock({
  companyId,
  group,
  canEdit,
  onOpenHistory,
}: Readonly<{
  companyId: string;
  group: FactGroup;
  canEdit: boolean;
  onOpenHistory?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [expanded, setExpanded] = useState(false);
  const capped = !canEdit;
  const hidden = capped ? group.facts.length - FACT_PREVIEW : 0;
  const shown =
    expanded || !capped ? group.facts : group.facts.slice(0, FACT_PREVIEW);
  return (
    <>
      <PanelGroupHead
        title={t(factCategoryLabelKey(group.category))}
        level="h3"
      />
      {shown.map((fact, index) => (
        <FactRow
          key={`${fact.field}:${fact.value_key}`}
          companyId={companyId}
          fact={fact}
          // A run of facts under one field shares the field's name once: the
          // first of the run prints it, and the rows that follow print only
          // what changed, which is the value. A reader scanning four
          // Locations is not asked to read the word "Location" four times.
          label={
            index === 0 || shown[index - 1].field !== fact.field
              ? t(factFieldLabelKey(fact.field))
              : undefined
          }
          canEdit={canEdit}
          onOpenHistory={onOpenHistory}
        />
      ))}
      {hidden > 0 && (
        <PanelBody>
          <Button onClick={() => setExpanded(!expanded)}>
            {expanded
              ? t("co.facts.showLess")
              : t("co.facts.showAll", {
                  count: formatNumber(group.facts.length, locale),
                })}
          </Button>
        </PanelBody>
      )}
    </>
  );
}

function AddFactForm({
  companyId,
  canEdit,
  onDone,
}: Readonly<{ companyId: string; canEdit: boolean; onDone: () => void }>) {
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
      const { error } = await api.POST("/companies/{id}/facts", {
        params: { path: { id: companyId } },
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
      await settleFacts(queryClient, companyId);
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
        pending={add.isPending}
        // Authority first, then completeness: a reader who may not write this
        // record is not helped by being told their form is incomplete.
        reason={refusal(canEdit, field, value, t)}
        onClick={() => add.mutate({ field, value })}
      />
      <IconAction
        label={t("co.facts.addCancel")}
        icon={<X aria-hidden />}
        onClick={onDone}
      />
      <ErrorLine error={add.error} />
    </div>
  );
}

// Why the save is refused, or nothing. Authority first: a reader who may not
// write this record is not helped by being told their form is incomplete.
function refusal(
  canEdit: boolean,
  field: string,
  value: string,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (!canEdit) {
    return t("record.notYoursToChange");
  }
  return field && value.trim() ? undefined : t("co.facts.addIncomplete");
}

// Everything that describes this account's facts, refreshed together. The keys
// are the ones the CONSUMERS register: React Query matches segments exactly, so
// a near-miss spelling invalidates nothing and the page goes on showing the row
// a reader just removed. Exported for companyfactrow.tsx's own removal
// mutation, which settles the same set.
export async function settleFacts(
  queryClient: ReturnType<typeof useQueryClient>,
  companyId: string,
) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: factsKey(companyId) }),
    queryClient.invalidateQueries({ queryKey: ["company", companyId] }),
    queryClient.invalidateQueries({ queryKey: ["company360", companyId] }),
    queryClient.invalidateQueries({ queryKey: ["companies"] }),
  ]);
}

// Pencil is imported for the correction affordance EvidenceVerdict draws; the
// import keeps the icon set for this panel in one place.
export const FACT_EDIT_ICON = Pencil;
export const FACT_ADD_ICON = Plus;
