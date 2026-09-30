// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { useUnsavedGuard } from "../app/unsaved";
import {
  Button,
  Disclosure,
  EmptyState,
  Field,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { Meter } from "../design-system/readings";
import { formatDateTime, formatNumber } from "../format/format";
import { formatTokens } from "../format/tokens";
import { useLocale, useT } from "../i18n";
import { AiFeatureTable } from "./ai-feature-table";
import { SpendEstimate } from "./ai-settings";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

type Budget = components["schemas"]["AiBudgetSnapshot"];
type Change = components["schemas"]["AiBudgetChange"];
type Deferred = components["schemas"]["AiDeferredWork"];

export function useAiStatus(enabled: boolean) {
  return useQuery({
    queryKey: ["ai-status"],
    enabled,
    refetchInterval: 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/status");
      if (error) throwProblem(error);
      if (!data) throw new Error("AI status is unavailable");
      return data;
    },
  });
}

export function AiBudgetCard() {
  const t = useT();
  const canSee = useCan("ai_budget", "read");
  const canManage = useCanWrite("ai_budget", "update");
  const query = useQuery({
    queryKey: ["ai-budget"],
    enabled: canSee,
    refetchInterval: 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/budget");
      if (error) throwProblem(error);
      if (!data) throw new Error("AI allowance is unavailable");
      return data;
    },
  });
  if (!canSee) return null;
  return (
    <Panel title={t("aiAdmin.allowance")}>
      <PanelBody>
        <p>{t("aiAdmin.pool")}</p>
        <QueryGate query={query} pendingLabel={t("aiAdmin.allowance")}>
          {(budget) => <BudgetBody budget={budget} canManage={canManage} />}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function BudgetReading({
  budget,
  estimate,
}: Readonly<{ budget: Budget; estimate?: ReactNode }>) {
  const t = useT();
  const { locale } = useLocale();
  const number = (value: number) => formatNumber(value, locale);
  const tokens = (value: number) => formatTokens(value, locale);
  const pct = Math.round((budget.spent_tokens / budget.monthly_tokens) * 100);
  return (
    <>
      <p>
        {t("aiAdmin.consumption", {
          spent: tokens(budget.spent_tokens),
          total: tokens(budget.monthly_tokens),
          pct: number(pct),
        })}
      </p>
      <Meter
        value={Math.min(100, pct)}
        max={100}
        label={t("aiAdmin.allowance")}
      />
      <p>
        {t("aiAdmin.remaining", { tokens: tokens(budget.remaining_tokens) })} ·{" "}
        {t("aiAdmin.reset", {
          date: formatDateTime(budget.resets_at, locale, "UTC"),
        })}
      </p>
      {estimate}
      <p>
        {budget.source === "company_override"
          ? t("aiAdmin.fixed")
          : t("aiAdmin.formula", {
              users: number(budget.eligible_full_users),
              tokens: tokens(budget.config.tokens_per_full_user),
            })}
        {budget.eligible_full_users === 0 &&
        budget.source !== "company_override"
          ? ` ${t("aiAdmin.floor")}`
          : ""}
      </p>
      <Callout
        kind="standing"
        tone={budget.band === "normal" ? "info" : "warning"}
        title={
          budget.band === "normal"
            ? t("aiAdmin.normal")
            : budget.band === "degraded"
              ? t("aiAdmin.degraded")
              : t("aiAdmin.queued")
        }
      >
        {t("aiAdmin.policy")}
      </Callout>
    </>
  );
}

function BudgetPreview({
  preview,
}: Readonly<{ preview: components["schemas"]["AiBudgetPreview"] }>) {
  const t = useT();
  const canRoute = useCan("ai_routing", "read");
  const canDiagnose = useCan("ai_diagnostics", "read");
  return (
    <>
      <Heading size="small" as="h3">
        {t("aiAdmin.preview")}
      </Heading>
      <BudgetReading budget={preview.proposed} />
      <p>{t("aiAdmin.previewHint")}</p>
      {canDiagnose && <DeferredWork rows={preview.deferred_work} />}
      {canRoute && (
        <Disclosure summary={t("aiAdmin.features")}>
          <AiFeatureTable rows={preview.features} canTrace={canDiagnose} />
        </Disclosure>
      )}
    </>
  );
}

function validTokenAllowance(value: string) {
  return /^\d+$/.test(value) && Number(value) > 0 && Number(value) <= 1e12;
}

function BudgetBody({
  budget,
  canManage,
}: Readonly<{ budget: Budget; canManage: boolean }>) {
  const t = useT();
  const client = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [perUser, setPerUser] = useState("");
  const [company, setCompany] = useState("");
  const [revision, setRevision] = useState("");
  useUnsavedGuard(editing);
  const preview = useMutation({
    mutationFn: async (change: Change) => {
      const { data, error } = await api.POST("/ai/budget/preview", {
        body: change,
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  const save = useMutation({
    mutationFn: async (change: Change) => {
      const { data, error } = await api.PUT("/ai/budget", { body: change });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: async (data) => {
      client.setQueryData(["ai-budget"], data);
      setEditing(false);
      preview.reset();
      await Promise.all([
        client.invalidateQueries({ queryKey: ["ai-status"] }),
        client.invalidateQueries({ queryKey: ["ai-usage"] }),
      ]);
    },
  });
  const valid =
    validTokenAllowance(perUser) &&
    (company === "" || validTokenAllowance(company));
  const change: Change = {
    config: {
      tokens_per_full_user: Number(perUser),
      company_monthly_tokens: company === "" ? null : Number(company),
    },
    expected_revision: revision,
  };
  const busy = preview.isPending || save.isPending;
  return (
    <>
      <BudgetReading budget={budget} estimate={<SpendEstimate />} />
      {save.isSuccess && (
        <Callout kind="outcome" tone="success" title={t("aiAdmin.saved")}>
          {t("aiAdmin.recovery")}
        </Callout>
      )}
      {!editing && canManage && (
        <div className="card-actions">
          <Button
            onClick={() => {
              setPerUser(String(budget.config.tokens_per_full_user));
              setCompany(
                budget.config.company_monthly_tokens === null
                  ? ""
                  : String(budget.config.company_monthly_tokens),
              );
              setRevision(budget.revision);
              setEditing(true);
              save.reset();
            }}
          >
            {t("aiAdmin.edit")}
          </Button>
        </div>
      )}
      {editing && (
        <>
          <div className="form-row">
            <Field label={t("aiAdmin.perUser")} hint={t("aiAdmin.range")}>
              {(control) => (
                <TextInput
                  {...control}
                  inputMode="numeric"
                  value={perUser}
                  disabled={busy}
                  onChange={(e) => {
                    setPerUser(e.target.value);
                    preview.reset();
                  }}
                />
              )}
            </Field>
            <Field
              label={t("aiAdmin.company")}
              hint={t("aiAdmin.overrideHint")}
            >
              {(control) => (
                <TextInput
                  {...control}
                  inputMode="numeric"
                  value={company}
                  disabled={busy}
                  onChange={(e) => {
                    setCompany(e.target.value);
                    preview.reset();
                  }}
                />
              )}
            </Field>
          </div>
          {revision !== budget.revision && (
            <Callout kind="standing" tone="warning" title={t("aiAdmin.stale")}>
              {t("aiAdmin.staleHelp")}
            </Callout>
          )}
          {(preview.isError || save.isError) && (
            <Callout kind="outcome" tone="danger" title={t("aiAdmin.failed")}>
              {problemMessageOf(preview.error ?? save.error, t)}
            </Callout>
          )}
          {preview.data && <BudgetPreview preview={preview.data} />}
          <div className="card-actions">
            <Button
              disabled={!valid || revision !== budget.revision}
              pending={preview.isPending}
              onClick={() => preview.mutate(change)}
            >
              {t("aiAdmin.preview")}
            </Button>
            <Button
              variant="primary"
              disabled={
                !preview.data ||
                !valid ||
                revision !== budget.revision ||
                preview.isPending
              }
              pending={save.isPending}
              onClick={() => save.mutate(change)}
            >
              {t("aiAdmin.save")}
            </Button>
            <Button
              disabled={busy}
              onClick={() => {
                setEditing(false);
                preview.reset();
                save.reset();
              }}
            >
              {t("aiAdmin.cancel")}
            </Button>
          </div>
        </>
      )}
    </>
  );
}

// The "AI by activity" section, withheld: `/ai/status` requires BOTH
// `ai_diagnostics:read` and `ai_budget:read` server-side (crm.yaml: "Read AI
// administration status"), so there is no partial payload for a reader
// missing either. Shared by every screen that opens this section — the
// routing tab and the usage tab both do — so the withheld state is spelled
// once rather than drifting between two silent `return null`s.
export function AiFeaturesWithheldPanel() {
  const t = useT();
  return (
    <Panel title={t("aiAdmin.features")}>
      <PanelBody>
        <EmptyState>{t("aiAdmin.featuresWithheld")}</EmptyState>
      </PanelBody>
    </Panel>
  );
}

function DeferredWork({ rows }: Readonly<{ rows: Deferred[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const label = (carrier: string) =>
    carrier === "site_read"
      ? t("aiAdmin.website")
      : carrier === "company_scan"
        ? t("aiAdmin.scans")
        : t("aiAdmin.voice");
  return (
    <Disclosure summary={t("aiAdmin.waiting")}>
      <p>{t("aiAdmin.coverage")}</p>
      <ul>
        {rows.map((row) => (
          <li key={row.carrier}>
            {label(row.carrier)}:{" "}
            {row.available && row.count !== undefined
              ? formatNumber(row.count, locale)
              : t("aiAdmin.unavailable")}
          </li>
        ))}
      </ul>
      <p>{t("aiAdmin.recovery")}</p>
    </Disclosure>
  );
}
