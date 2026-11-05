// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useAssignableUserOptions } from "../../components/assigneepicker";
import { EntityRef } from "../../components/entityref";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useCompanyZone } from "../../lib/companyzone";
import { unwrap } from "../../lib/problem";

export type SalesDecision = components["schemas"]["SalesDecision"];
type DecisionInput = components["schemas"]["SalesDecisionInput"];

const DECISIONS_KEY = "sales-decisions";

/** The decisions recorded from sales reviews, newest first, on one deal or all. */
export function useSalesDecisions(dealId?: string) {
  return useQuery({
    queryKey: [DECISIONS_KEY, dealId ?? ""],
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/decisions", {
          params: { query: { deal_id: dealId } },
        }),
      ).data,
    staleTime: 30_000,
  });
}

/**
 * Decisions from the last review: the open ones and the recently closed, each
 * with its owner, deal and review date. Recording an outcome closes one and
 * changes no deal.
 */
export function DecisionsPanel() {
  const t = useT();
  const { locale } = useLocale();
  const zone = useCompanyZone();
  const decisions = useSalesDecisions();
  const canRecord = useCanWrite("activity", "create");
  const canClose = useCanWrite("activity", "update");
  const [recording, setRecording] = useState(false);
  const [closing, setClosing] = useState<SalesDecision | null>(null);
  if (!decisions.isError && !decisions.data?.length && !recording)
    return canRecord ? (
      <div className="actions">
        <Button variant="ghost" onClick={() => setRecording(true)}>
          {t("analytics.decisions.record")}
        </Button>
      </div>
    ) : null;
  return (
    <Panel
      title={t("analytics.decisions.title")}
      titleAction={
        canRecord ? (
          <Button variant="ghost" onClick={() => setRecording(true)}>
            {t("analytics.decisions.record")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <p className="t-caption">{t("analytics.decisions.purpose")}</p>
        <QueryGate
          query={decisions}
          pendingLabel={t("analytics.decisions.title")}
        >
          {(rows) =>
            rows.length === 0 ? (
              <EmptyState>{t("analytics.decisions.none")}</EmptyState>
            ) : (
              <DataTable
                label={t("analytics.decisions.title")}
                rows={rows}
                rowKey={(decision) => decision.id}
                columns={[
                  {
                    key: "decision",
                    header: t("analytics.decisions.decision"),
                    render: (decision) => (
                      <>
                        <span>{decision.decision}</span>
                        {decision.expected_result && (
                          <p className="t-sub">{decision.expected_result}</p>
                        )}
                        {decision.result && (
                          <p className="t-sub">
                            {t("analytics.decisions.resultLine", {
                              result: decision.result,
                            })}
                          </p>
                        )}
                      </>
                    ),
                  },
                  {
                    key: "deal",
                    header: t("analytics.decisions.deal"),
                    render: (decision) => (
                      <EntityRef kind="deal" id={decision.deal_id} />
                    ),
                  },
                  {
                    key: "owner",
                    header: t("analytics.decisions.owner"),
                    render: (decision) => (
                      <EntityRef kind="user" id={decision.owner_id} />
                    ),
                  },
                  {
                    key: "review",
                    header: t("analytics.decisions.reviewOn"),
                    render: (decision) =>
                      decision.review_on
                        ? formatDate(decision.review_on, locale, zone)
                        : "—",
                  },
                  {
                    key: "status",
                    header: t("analytics.decisions.status"),
                    // A task closed elsewhere still owes its outcome, so the
                    // action stays until a result is recorded.
                    render: (decision) => (
                      <>
                        {decision.status === "done" ? (
                          <Badge tone="success">
                            {t("analytics.decisions.done")}
                          </Badge>
                        ) : (
                          <Badge>{t("analytics.decisions.open")}</Badge>
                        )}
                        {canClose && decision.result == null && (
                          <Button
                            variant="link"
                            onClick={() => setClosing(decision)}
                          >
                            {t("analytics.decisions.recordOutcome")}
                          </Button>
                        )}
                      </>
                    ),
                  },
                ]}
              />
            )
          }
        </QueryGate>
      </PanelBody>
      {recording && (
        <RecordDecisionDialog onClose={() => setRecording(false)} />
      )}
      {closing && (
        <OutcomeDialog decision={closing} onClose={() => setClosing(null)} />
      )}
    </Panel>
  );
}

/** The review's entry under More: the same list in a drawer. */
export function ReviewDecisionsAction() {
  const t = useT();
  const titleId = useId();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button variant="ghost" onClick={() => setOpen(true)}>
        {t("analytics.decisions.review")}
      </Button>
      {open && (
        <Modal
          open
          onClose={() => setOpen(false)}
          labelledBy={titleId}
          intent="drawer"
        >
          <Heading size="large" id={titleId} className="modal-title">
            {t("analytics.decisions.review")}
          </Heading>
          <DecisionsPanel />
        </Modal>
      )}
    </>
  );
}

type Draft = Readonly<{
  decision: string;
  dealId: string;
  ownerId: string;
  reviewOn: string;
  expected: string;
}>;

/**
 * Record decision: the decision, its deal, the accountable owner, the review
 * date and the result the owner expects. One confirmation writes one task;
 * a retry under the same key, or the same open decision on the same deal,
 * answers that task again.
 */
export function RecordDecisionDialog({
  initial,
  onClose,
}: Readonly<{
  // What the assessment proposed, for the reader to confirm or change.
  initial?: Partial<Draft>;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const client = useQueryClient();
  const [key] = useState(() => crypto.randomUUID());
  const [draft, setDraft] = useState<Draft>({
    decision: "",
    dealId: "",
    ownerId: "",
    reviewOn: "",
    expected: "",
    ...initial,
  });
  const owners = useAssignableUserOptions();
  const deals = useQuery({
    queryKey: ["decision-deals"],
    queryFn: async () =>
      unwrap(
        await api.GET("/deals", {
          params: { query: { limit: 200, status: "open" } },
        }),
      ).data.map((deal) => ({ value: deal.id, label: deal.name })),
    staleTime: 60_000,
  });
  const record = useMutation({
    mutationFn: async (body: DecisionInput) =>
      unwrap(
        await api.POST("/analytics/decisions", {
          params: { header: { "Idempotency-Key": key } },
          body,
        }),
      ),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: [DECISIONS_KEY] });
      onClose();
    },
  });
  const incomplete =
    !draft.decision.trim() ||
    !draft.dealId ||
    !draft.ownerId ||
    !isoOrBlank(draft.reviewOn);
  const submit = () =>
    record.mutate({
      decision: draft.decision.trim(),
      deal_id: draft.dealId,
      owner_id: draft.ownerId,
      review_on: draft.reviewOn,
      expected_result: draft.expected.trim() || undefined,
    });
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {t("analytics.decisions.record")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (!incomplete) submit();
        }}
      >
        <Field label={t("analytics.decisions.decision")} required>
          {(field) => (
            <TextInput
              {...field}
              value={draft.decision}
              maxLength={500}
              onChange={(event) =>
                setDraft({ ...draft, decision: event.target.value })
              }
            />
          )}
        </Field>
        <Field label={t("analytics.decisions.deal")} required>
          {(field) => (
            <Select
              {...field}
              value={draft.dealId}
              options={[
                { value: "", label: t("analytics.decisions.chooseDeal") },
                ...(deals.data ?? []),
              ]}
              onChange={(value) => setDraft({ ...draft, dealId: value })}
            />
          )}
        </Field>
        <Field label={t("analytics.decisions.owner")} required>
          {(field) => (
            <Select
              {...field}
              value={draft.ownerId}
              options={[
                { value: "", label: t("analytics.decisions.chooseOwner") },
                ...owners.options,
              ]}
              onChange={(value) => setDraft({ ...draft, ownerId: value })}
            />
          )}
        </Field>
        <Field label={t("analytics.decisions.reviewOn")} required>
          {(field) => (
            <DateInput
              {...field}
              value={isoOrBlank(draft.reviewOn)}
              onChange={(event) =>
                setDraft({ ...draft, reviewOn: event.target.value })
              }
            />
          )}
        </Field>
        <Field label={t("analytics.decisions.expected")}>
          {(field) => (
            <Textarea
              {...field}
              value={draft.expected}
              rows={3}
              onChange={(event) =>
                setDraft({ ...draft, expected: event.target.value })
              }
            />
          )}
        </Field>
        <p className="t-caption">{t("analytics.decisions.recordNote")}</p>
        <ErrorLine error={record.error} />
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("reporting.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          disabled={incomplete}
          pending={record.isPending}
        >
          {t("analytics.decisions.confirm")}
        </Button>
      </div>
    </Modal>
  );
}

/** Record outcome: what work was completed. The task closes; the deal stays. */
function OutcomeDialog({
  decision,
  onClose,
}: Readonly<{ decision: SalesDecision; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  const client = useQueryClient();
  const [result, setResult] = useState("");
  const close = useMutation({
    mutationFn: async (text: string) =>
      unwrap(
        await api.POST("/analytics/decisions/{id}/outcome", {
          params: { path: { id: decision.id } },
          body: { result: text },
        }),
      ),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: [DECISIONS_KEY] });
      onClose();
    },
  });
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" id={titleId} className="t-h2 modal-title">
        {t("analytics.decisions.recordOutcome")}
      </Heading>
      <p className="t-caption">{decision.decision}</p>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (result.trim()) close.mutate(result.trim());
        }}
      >
        <Field label={t("analytics.decisions.whatWasDone")} required>
          {(field) => (
            <Textarea
              {...field}
              value={result}
              rows={4}
              onChange={(event) => setResult(event.target.value)}
            />
          )}
        </Field>
        <p className="t-caption">{t("analytics.decisions.outcomeNote")}</p>
        <ErrorLine error={close.error} />
      </form>
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("reporting.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          disabled={!result.trim()}
          pending={close.isPending}
        >
          {t("analytics.decisions.saveOutcome")}
        </Button>
      </div>
    </Modal>
  );
}
