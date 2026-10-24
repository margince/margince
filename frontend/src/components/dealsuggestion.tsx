// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A Deal Scout suggestion, wherever it is shown — the pipeline board, the
// Worklist and the company page — and the two answers a rep gives it.
//
// It is drawn as STAGED: the indigo edge and the agent's mark say a machine
// proposed this and nobody has agreed yet. Accepting opens a real deal, which
// the board then draws as an ordinary card; the suggestion never counts in a
// column, a total or a forecast.

import { type ReactNode, useId, useState } from "react";
import { Button, Field, Modal, TextInput } from "../design-system/atoms";
import { DateInput, isoOrBlank } from "../design-system/dateinput";
import { ErrorLine } from "../design-system/errorline";
import { refuseUnreadableField } from "../design-system/formvalidity";
import { Heading } from "../design-system/heading";
import { MoneyInput } from "../design-system/moneyinput";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { Row } from "../design-system/stack";
import { useToast } from "../design-system/toast";
import {
  type ConfidenceLevel,
  ConfidenceMeter,
  ProvenanceTag,
  StagingCard,
} from "../design-system/trust";
import { formatDate, formatMoney } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { useRecordZone } from "../lib/recordzone";
import { routeHash } from "../lib/router";
import { useRecordOwnerOptions } from "./assigneepicker";
import { DealSuggestionAnswerDialog } from "./dealsuggestion.answer";
import {
  type AcceptDealSuggestionBody,
  type DealSuggestion,
  type SuggestionFilter,
  useAcceptDealSuggestion,
  useDealSuggestions,
} from "./dealsuggestions.queries";
import { EntityRef } from "./entityref";
import "./dealsuggestion.css";
import { currencyOptions } from "../format/currencychoices";
import { useLeadingCurrencies } from "../lib/leadingcurrencies";
import { usePipelines } from "../lib/pipelines.queries";

function confidenceLevel(confidence: number): ConfidenceLevel {
  if (confidence >= 0.8) {
    return "high";
  }
  return confidence >= 0.65 ? "med" : "low";
}

type Evidence = DealSuggestion["evidence"][number];

function evidenceLine(evidence: Evidence, t: ReturnType<typeof useT>): string {
  switch (evidence.kind) {
    case "meeting":
      return t("dealSuggestion.evidence.meeting", { title: evidence.title });
    case "signal":
      return t("dealSuggestion.evidence.signal", { title: evidence.title });
    default:
      return t("dealSuggestion.evidence.attachment", { title: evidence.title });
  }
}

/**
 * One suggestion with its evidence and its two answers. `compact` is the board's
 * ghost card: the name, the amount and the answers, and no evidence list — the
 * column is narrow and the Worklist and company page carry the whole reading.
 */
export function DealSuggestionCard({
  suggestion,
  compact = false,
}: Readonly<{ suggestion: DealSuggestion; compact?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  return (
    <StagingCard>
      <div className="dealsuggestion" data-testid="deal-suggestion">
        <Row gap="2" wrap={false}>
          <ProvenanceTag
            provenance={{ kind: "agent", agent: t("dealSuggestion.agent") }}
          />
          <ConfidenceMeter level={confidenceLevel(suggestion.confidence)} />
        </Row>
        <p>
          {compact ? (
            // On the board this card is the suggestion's only trace. It leads to
            // the company it is about, as the Worklist row does.
            <a
              className="staged-value"
              href={routeHash({
                screen: "companies",
                id: suggestion.company_id,
              })}
            >
              {suggestedName(suggestion, t)}
            </a>
          ) : (
            <span className="staged-value">{suggestedName(suggestion, t)}</span>
          )}
        </p>
        {suggestion.amount_minor != null && suggestion.currency && (
          <p className="dealsuggestion-amount t-caption">
            {formatMoney(suggestion.amount_minor, suggestion.currency, locale)}
          </p>
        )}
        {!compact && (
          <p className="t-caption">
            {t("dealSuggestion.accountOwner")}:{" "}
            {suggestion.owner_id ? (
              <EntityRef kind="user" id={suggestion.owner_id} />
            ) : (
              t("dealSuggestion.unassignedOwner")
            )}
          </p>
        )}
        {!compact && (
          <ul className="dealsuggestion-evidence t-caption">
            {suggestion.evidence.map((evidence) => (
              <li
                key={
                  evidence.activity_id ??
                  evidence.signal_id ??
                  evidence.attachment_id
                }
              >
                <span>{evidenceLine(evidence, t)}</span>
                {" — "}
                {formatDate(evidence.occurred_at, locale, zone)}
              </li>
            ))}
          </ul>
        )}
        <SuggestionAnswers suggestion={suggestion} />
      </div>
    </StagingCard>
  );
}

export function SuggestionAnswers({
  suggestion,
}: Readonly<{ suggestion: DealSuggestion }>) {
  const t = useT();
  const mayCreate = useCanWrite("deal", "create");
  const mayLink = useCanWrite("deal", "update");
  const [action, setAction] = useState<"create" | "link" | "dismiss" | null>(
    null,
  );
  if (suggestion.state !== "open" || (!mayCreate && !mayLink)) return null;
  return (
    <div className="dealsuggestion-answers">
      {mayCreate && (
        <Button
          variant="ai"
          onClick={() => setAction("create")}
          data-testid="deal-suggestion-accept"
        >
          {t("dealSuggestion.open")}
        </Button>
      )}
      {mayLink && (
        <Button onClick={() => setAction("link")}>
          {t("dealSuggestion.linkExisting")}
        </Button>
      )}
      {mayCreate && (
        <Button
          variant="ghost"
          onClick={() => setAction("dismiss")}
          data-testid="deal-suggestion-dismiss"
        >
          {t("dealSuggestion.dismiss")}
        </Button>
      )}
      {action === "create" && (
        <AcceptSuggestionDialog
          suggestion={suggestion}
          onClose={() => setAction(null)}
        />
      )}
      {(action === "link" || action === "dismiss") && (
        <DealSuggestionAnswerDialog
          suggestion={suggestion}
          action={action}
          onClose={() => setAction(null)}
        />
      )}
    </div>
  );
}

type Draft = {
  name: string;
  amountMinor: number;
  currency: string;
  stageId: string;
  ownerId: string;
  closeDate: string;
};

/**
 * The name a suggested deal is offered under: the company's, and the kind of
 * evidence that leads, worded here. The server stores only the kind, so no
 * text read out of a message reaches a suggestion.
 */
export function suggestedName(
  suggestion: DealSuggestion,
  t: ReturnType<typeof useT>,
): string {
  return t("dealSuggestion.name", {
    company: suggestion.company_name,
    hint: t(`dealSuggestion.hint.${suggestion.name_hint}` as const),
  });
}

function draftOf(
  suggestion: DealSuggestion,
  name: string,
  defaultCurrency: string,
): Draft {
  return {
    name,
    amountMinor: suggestion.amount_minor ?? 0,
    currency: suggestion.currency ?? defaultCurrency,
    stageId: suggestion.stage_id,
    ownerId: "",
    closeDate: suggestion.close_date ?? "",
  };
}

/**
 * What the request carries: the name, and whatever else the reader changed or
 * set. An omitted field keeps the suggestion's own value on the server, and the
 * amount travels with its currency or not at all — so an amount the reader
 * emptied is sent as `no_amount`, or the server would keep the one it proposed.
 */
export function acceptBody(
  suggestion: DealSuggestion,
  draft: Draft,
): AcceptDealSuggestionBody {
  const body: AcceptDealSuggestionBody = {};
  const name = draft.name.trim();
  if (name) {
    body.name = name;
  }
  if (draft.amountMinor > 0) {
    body.amount_minor = draft.amountMinor;
    body.currency = draft.currency;
  } else if (suggestion.amount_minor != null) {
    body.no_amount = true;
  }
  if (draft.stageId && draft.stageId !== suggestion.stage_id) {
    body.stage_id = draft.stageId;
  }
  if (draft.ownerId) {
    body.owner_id = draft.ownerId;
  }
  if (draft.closeDate) {
    body.close_date = draft.closeDate;
  }
  return body;
}

export function AcceptSuggestionDialog({
  suggestion,
  onClose,
}: Readonly<{ suggestion: DealSuggestion; onClose: () => void }>) {
  const t = useT();
  const toast = useToast();
  const headingId = useId();
  // ONE key per dialog, held across retries: a response lost after the deal
  // was opened replays that acceptance rather than being refused as a second.
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const leading = useLeadingCurrencies();
  const [draft, setDraft] = useState(() =>
    draftOf(suggestion, suggestedName(suggestion, t), leading[0]),
  );
  const accept = useAcceptDealSuggestion();
  const owners = useRecordOwnerOptions();
  const pipelines = usePipelines();
  const stages = (
    pipelines.data?.find((pipeline) => pipeline.id === suggestion.pipeline_id)
      ?.stages ?? []
  ).filter((stage) => stage.semantic === "open");
  const set = (patch: Partial<Draft>) =>
    setDraft((current) => ({ ...current, ...patch }));
  const submit = () =>
    accept.mutate(
      {
        id: suggestion.id,
        idempotencyKey,
        body: acceptBody(suggestion, draft),
      },
      {
        onSuccess: (out) => {
          toast.show(
            t(
              out.unlinked_activity_ids.length > 0
                ? "dealSuggestion.acceptedUnlinked"
                : "dealSuggestion.accepted",
              { name: draft.name.trim() || suggestion.company_name },
            ),
          );
          onClose();
        },
      },
    );
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("dealSuggestion.acceptTitle", { company: suggestion.company_name })}
      </Heading>
      <div className="form-stack">
        <Field label={t("dealSuggestion.field.name")} required>
          {(control) => (
            <TextInput
              {...control}
              value={draft.name}
              onChange={(event) => set({ name: event.target.value })}
              data-testid="deal-suggestion-name"
            />
          )}
        </Field>
        <div className="form-row dealsuggestion-money">
          <Field label={t("dealSuggestion.field.amount")}>
            {(control) => (
              <MoneyInput
                {...control}
                valueMinor={draft.amountMinor}
                currency={draft.currency}
                blankWhenZero
                onChangeMinor={(amountMinor) => set({ amountMinor })}
                data-testid="deal-suggestion-amount"
              />
            )}
          </Field>
          <Field label={t("dealSuggestion.field.currency")}>
            {(control) => (
              <Select
                {...control}
                options={currencyOptions(leading, draft.currency)}
                value={draft.currency}
                onChange={(currency) => set({ currency })}
              />
            )}
          </Field>
        </div>
        <Field label={t("dealSuggestion.field.stage")}>
          {(control) => (
            <Select
              {...control}
              options={stages.map((stage) => ({
                value: stage.id,
                label: stage.name,
              }))}
              value={draft.stageId}
              onChange={(stageId) => set({ stageId })}
            />
          )}
        </Field>
        <Field label={t("dealSuggestion.field.owner")}>
          {(control) => (
            <Select
              {...control}
              options={[
                { value: "", label: t("dealSuggestion.ownerMe") },
                ...owners,
              ]}
              value={draft.ownerId}
              onChange={(ownerId) => set({ ownerId })}
            />
          )}
        </Field>
        <Field
          label={t("dealSuggestion.field.closeDate")}
          hint={t("dealSuggestion.closeDateHint")}
        >
          {(control) => (
            <DateInput
              {...control}
              value={isoOrBlank(draft.closeDate)}
              onChange={(event) => set({ closeDate: event.target.value })}
              data-testid="deal-suggestion-close"
            />
          )}
        </Field>
        <ErrorLine error={accept.error} />
      </div>
      <div className="actions">
        <Button onClick={onClose} disabled={accept.isPending}>
          {t("create.cancel")}
        </Button>
        <Button
          variant="primary"
          disabled={!draft.name.trim() || accept.isPending}
          onClick={(event) => {
            if (!refuseUnreadableField(event.currentTarget)) {
              submit();
            }
          }}
          data-testid="deal-suggestion-confirm"
        >
          {t("dealSuggestion.confirm")}
        </Button>
      </div>
    </Modal>
  );
}

/**
 * The company page's block: this account's open suggestion, beside its signals.
 * Absent when there is none, because "the scout has nothing to say" is not
 * news worth a panel. Plain rather than tinted: the staging card inside already
 * carries the agent's mark, and a second indigo panel would be a second lead.
 */
export function CompanySuggestions({
  companyId,
}: Readonly<{ companyId: string }>) {
  const t = useT();
  const query = useDealSuggestions({ company_id: companyId });
  const suggestions = query.data ?? [];
  if (suggestions.length === 0) {
    return null;
  }
  return (
    <Panel title={t("dealSuggestion.companyTitle")}>
      <PanelBody>
        {suggestions.map((suggestion) => (
          <DealSuggestionCard key={suggestion.id} suggestion={suggestion} />
        ))}
      </PanelBody>
    </Panel>
  );
}

/**
 * The board's ghost cards: for the pipeline on screen, a function drawing each
 * column's open suggestions, keyed by the stage each would open in.
 *
 * Drawn BESIDE a column's cards through its extras, never among them: the
 * column's count and totals come from its deals alone, and a suggestion is not
 * one. Nothing is read until a pipeline is chosen.
 *
 * The board's owner dials narrow the suggestions as they narrow its deals.
 */
export function useSuggestionGhosts(
  pipelineId: string | undefined,
  owner: Omit<SuggestionFilter, "company_id" | "pipeline_id">,
): (column: { stage: string }) => ReactNode {
  const query = useDealSuggestions(
    { pipeline_id: pipelineId, ...owner },
    Boolean(pipelineId),
  );
  const byStage = new Map<string, DealSuggestion[]>();
  for (const suggestion of query.data ?? []) {
    byStage.set(suggestion.stage_id, [
      ...(byStage.get(suggestion.stage_id) ?? []),
      suggestion,
    ]);
  }
  return (column) =>
    (byStage.get(column.stage) ?? []).map((suggestion) => (
      <DealSuggestionCard key={suggestion.id} suggestion={suggestion} compact />
    ));
}
