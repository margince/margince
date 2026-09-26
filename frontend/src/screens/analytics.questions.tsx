// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import { routeHash, useRoute } from "../app/router";
import { Button, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { FactList } from "../design-system/factlist";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Stack } from "../design-system/stack";
import { SurfaceState } from "../design-system/surfacestate";
import { formatMoneyOrAbsent } from "../format/format";
import { useLocale, useT } from "../i18n";
import { openAnalyticsSection, openSavedQuestion } from "./analytics.address";
import {
  type AnalyticsContext,
  type AnalyticsScope,
  type AnalyticsSelection,
  scopeQuery,
} from "./analytics.context";
import { AnswerTable, QuestionFailure } from "./analytics.questions.answer";
import { QuestionBuilder } from "./analytics.questions.builder";
import {
  draftFromQuery,
  inOptionOrder,
  newDraft,
  type QuestionDraft,
  toQuery,
} from "./analytics.questions.draft";
import { useValueNamer } from "./analytics.questions.names";
import {
  type AnalyticsEntity,
  type AnalyticsQuery,
  analyticsFieldLabel,
  entityLabel,
  filterAmountCurrency,
  measureLabel,
  opLabel,
  takesValue,
} from "./analytics.questions.vocab";
import { QueryGate, throwProblem } from "./common";
import { EntityRef } from "./entityref";

// The population a query names, as one comparable string: the page's picker
// and a question already asked agree only when these do.
function queryScopeKey(query: Pick<AnalyticsQuery, "scope_kind" | "scope_id">) {
  return `${query.scope_kind ?? ""}:${query.scope_id ?? ""}`;
}

async function askQuestion(query: AnalyticsQuery) {
  const { data, error } = await api.POST("/analytics/query", { body: query });
  if (error) {
    throwProblem(error);
  }
  return data;
}

function useAnalyticsSchema() {
  return useQuery({
    queryKey: ["analytics-schema"],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/schema", {});
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * The Questions section: a question nobody wrote a report for, composed from
 * this seat's own vocabulary and answered by the database. A saved one lives
 * at its own address and is answered again for whoever opens it.
 */
export function QuestionsView({
  context,
  selection,
  onSelectScope,
}: Readonly<{
  context: AnalyticsContext;
  selection: AnalyticsSelection;
  onSelectScope: (scope: AnalyticsScope) => void;
}>) {
  const { default_scope: defaultScope, allowed_scopes: allowedScopes } =
    context;
  // An empty code names no currency, the same as an absent one.
  const baseCurrency = context.base_currency || null;
  const route = useRoute();
  const runId = route.screen === "analytics" ? route.id2 : undefined;
  const [draft, setDraft] = useState<QuestionDraft>(() => newDraft(""));
  // Set when an edited question was saved over a population this reader may
  // not measure, so the builder says it will be asked over another one.
  const [scopeNotice, setScopeNotice] = useState<string | null>(null);
  if (runId) {
    return (
      <SavedQuestion
        runId={runId}
        defaultScope={defaultScope}
        allowedScopes={allowedScopes}
        baseCurrency={baseCurrency}
        onEdit={(query) => {
          setDraft(draftFromQuery(query, baseCurrency));
          // The question was asked over a population; editing it keeps that
          // population, and says so when this reader may not measure it
          // rather than quietly re-asking over another.
          const wanted = queryScopeKey(query);
          const known = query.scope_kind
            ? allowedScopes.find(
                (candidate) => queryScopeKey(scopeQuery(candidate)) === wanted,
              )
            : defaultScope;
          onSelectScope(known ?? defaultScope);
          setScopeNotice(known ? null : defaultScope.label);
          openAnalyticsSection("questions");
        }}
      />
    );
  }
  return (
    <AskQuestion
      selection={selection}
      draft={draft}
      onDraft={setDraft}
      baseCurrency={baseCurrency}
      scopeNotice={scopeNotice}
      onAsked={() => setScopeNotice(null)}
    />
  );
}

function AskQuestion({
  selection,
  draft,
  onDraft,
  baseCurrency,
  scopeNotice,
  onAsked,
}: Readonly<{
  selection: AnalyticsSelection;
  draft: QuestionDraft;
  onDraft: (next: QuestionDraft) => void;
  baseCurrency: string | null;
  scopeNotice: string | null;
  onAsked: () => void;
}>) {
  const t = useT();
  const schema = useAnalyticsSchema();
  const ask = useMutation({ mutationFn: askQuestion });
  const save = useMutation({
    mutationFn: (query: AnalyticsQuery) =>
      askQuestion({ ...query, save: true }),
    onSuccess: (answer) => {
      if (answer.run_id) {
        openSavedQuestion(answer.run_id);
      }
    },
  });
  const scope = scopeQuery(selection.scope);
  // A saved question can carry its grouping in any order; it is asked in the
  // order the picker shows it.
  const asAsked = (
    question: QuestionDraft,
    entities: readonly AnalyticsEntity[],
  ) => {
    const entity = entities.find((e) => e.name === question.entity);
    return toQuery(
      entity
        ? { ...question, groupBy: inOptionOrder(question.groupBy, entity) }
        : question,
      scope,
      baseCurrency,
    );
  };
  // An answer is shown only under the population it was asked over. Changing
  // the picker afterwards would otherwise put one population's figures under
  // another's name.
  const asked = ask.variables;
  const current = asked && queryScopeKey(asked) === queryScopeKey(scope);
  // Changed since it was asked: the figures stay, under the question that
  // produced them, and say they are not the question now on screen.
  const stale = (entities: readonly AnalyticsEntity[]) =>
    asked !== undefined &&
    JSON.stringify(asAsked(draft, entities)) !== JSON.stringify(asked);
  return (
    <QueryGate query={schema} pendingLabel={t("analytics.q.loadingSchema")}>
      {(vocabulary) =>
        vocabulary.entities && vocabulary.entities.length > 0 ? (
          <Stack gap="4">
            {scopeNotice && (
              <Callout
                tone="info"
                kind="standing"
                title={t("analytics.q.scopeNotAvailable", {
                  scope: scopeNotice,
                })}
              />
            )}
            <QuestionBuilder
              entities={vocabulary.entities}
              draft={draft}
              baseCurrency={baseCurrency}
              onChange={onDraft}
              onAsk={() => {
                onAsked();
                ask.mutate(asAsked(draft, vocabulary.entities));
              }}
              onSave={() => save.mutate(asAsked(draft, vocabulary.entities))}
              asking={ask.isPending}
              saving={save.isPending}
            />
            {save.isError && <QuestionFailure error={save.error} />}
            {asked && current && (
              <Panel title={t("analytics.q.answerTitle")}>
                <PanelBody>
                  {ask.isPending && (
                    <PendingBody label={t("analytics.q.asking")} />
                  )}
                  {ask.isError && <QuestionFailure error={ask.error} />}
                  {ask.data && stale(vocabulary.entities) && (
                    <Callout
                      tone="warning"
                      kind="standing"
                      title={t("analytics.q.staleTitle")}
                    >
                      {t("analytics.q.staleBody")}
                    </Callout>
                  )}
                  {ask.data && (
                    <AnswerTable
                      query={asked}
                      answer={ask.data}
                      baseCurrency={baseCurrency}
                      source={{ kind: "query", query: asked }}
                    />
                  )}
                </PanelBody>
              </Panel>
            )}
          </Stack>
        ) : (
          // The vocabulary is narrowed by this seat's grants before it is
          // sent, so a seat with nothing in it is a permission, not an
          // empty installation.
          <SurfaceState
            state="withheld"
            emptyLabel={t("common.empty")}
            loadingLabel={t("analytics.q.loadingSchema")}
            detail={{ withheldReason: t("analytics.q.noEntities") }}
          >
            {null}
          </SurfaceState>
        )
      }
    </QueryGate>
  );
}

function scopeLabel(
  query: AnalyticsQuery,
  defaultScope: AnalyticsScope,
  allowedScopes: readonly AnalyticsScope[],
): string | null {
  if (!query.scope_kind) {
    return defaultScope.label;
  }
  const wanted = queryScopeKey(query);
  const known = allowedScopes.find(
    (scope) => queryScopeKey(scopeQuery(scope)) === wanted,
  );
  return known ? known.label : null;
}

/**
 * A saved question, answered again for THIS reader: the stored rows were
 * floored for whoever asked, so what is shown is the question re-asked under
 * the reader's own access, and the page says so.
 */
function SavedQuestion({
  runId,
  defaultScope,
  allowedScopes,
  baseCurrency,
  onEdit,
}: Readonly<{
  runId: string;
  defaultScope: AnalyticsScope;
  allowedScopes: readonly AnalyticsScope[];
  baseCurrency: string | null;
  onEdit: (query: AnalyticsQuery) => void;
}>) {
  const t = useT();
  const run = useQuery({
    queryKey: ["analytics-run", runId],
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/runs/{run_id}", {
        params: { path: { run_id: runId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const link = `${globalThis.location.origin}${globalThis.location.pathname}${routeHash(
    { screen: "analytics", id: "questions", id2: runId },
  )}`;
  const clipboard = useClipboardCopy(link, {
    copy: t("analytics.q.copyLink"),
    copied: t("analytics.q.linkCopied"),
    remedy: t("analytics.q.copyRemedy"),
  });
  if (run.isPending) {
    return <PendingBody label={t("analytics.q.loadingRun")} />;
  }
  if (run.isError) {
    return (
      <Stack gap="3">
        <QuestionFailure error={run.error} />
        <div>
          <Button onClick={() => openAnalyticsSection("questions")}>
            {t("analytics.q.newQuestion")}
          </Button>
        </div>
      </Stack>
    );
  }
  const saved = run.data;
  const population = scopeLabel(saved.query, defaultScope, allowedScopes);
  return (
    <Stack gap="4">
      <Panel
        title={t("analytics.q.savedTitle")}
        actions={
          <>
            <Button onClick={clipboard.copy}>{clipboard.label}</Button>
            <Button onClick={() => onEdit(saved.query)}>
              {t("analytics.q.edit")}
            </Button>
          </>
        }
      >
        <PanelBody>
          <PanelIntro>{t("analytics.q.readerAccess")}</PanelIntro>
          <QuestionSummary
            query={saved.query}
            baseCurrency={baseCurrency}
            population={population}
            askedBy={<EntityRef kind="user" id={saved.asked_by} />}
          />
          {clipboard.notice}
        </PanelBody>
      </Panel>
      <Panel title={t("analytics.q.answerTitle")}>
        <PanelBody>
          <AnswerTable
            query={saved.query}
            answer={saved.answer}
            baseCurrency={baseCurrency}
            source={{ kind: "run", runId }}
          />
        </PanelBody>
      </Panel>
    </Stack>
  );
}

/** The question a saved run asks, read back as facts rather than controls. */
function QuestionSummary({
  query,
  baseCurrency,
  population,
  askedBy,
}: Readonly<{
  query: AnalyticsQuery;
  baseCurrency: string | null;
  population: string | null;
  askedBy: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const filters = query.filters ?? [];
  const namer = useValueNamer(
    query.entity,
    filters.map((f) => f.field),
    undefined,
  );
  const none = t("analytics.q.none");
  // An amount travels in minor units and reads as money.
  const filterValueText = (filter: (typeof filters)[number]) => {
    const currency = filterAmountCurrency(filter.field, filters, baseCurrency);
    return currency && typeof filter.value === "number"
      ? formatMoneyOrAbsent(filter.value, currency, locale)
      : namer(filter.field, filter.value);
  };
  const facts = [
    {
      key: "population",
      term: t("analytics.q.population"),
      value: entityLabel(t, query.entity),
    },
    {
      key: "scope",
      term: t("analytics.q.measuredOver"),
      value: population ?? t("analytics.q.scopeUnknown"),
    },
    {
      key: "group",
      term: t("analytics.q.groupBy"),
      value:
        (query.group_by ?? [])
          .map((field) => analyticsFieldLabel(t, field))
          .join(", ") || none,
    },
    {
      key: "measures",
      term: t("analytics.q.measures"),
      value: query.measures.map((m) => measureLabel(t, m)).join(", "),
    },
    {
      key: "filters",
      term: t("analytics.q.filters"),
      value:
        filters
          .map((f) =>
            [
              analyticsFieldLabel(t, f.field),
              opLabel(t, f.op),
              takesValue(f.op) ? filterValueText(f) : "",
            ]
              .filter((part) => part !== "")
              .join(" "),
          )
          .join("; ") || none,
    },
    { key: "askedBy", term: t("analytics.q.savedBy"), value: askedBy },
  ];
  return <FactList facts={facts} />;
}
