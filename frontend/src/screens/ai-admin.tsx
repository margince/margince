// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { routeHash } from "../app/router";
import { useUnsavedGuard } from "../app/unsaved";
import {
  Badge,
  Button,
  DataTable,
  Disclosure,
  EmptyState,
  Field,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { Meter } from "../design-system/readings";
import { formatDateTime, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { settingsHref } from "./settingsrouting";

type Budget = components["schemas"]["AiBudgetSnapshot"];
type Change = components["schemas"]["AiBudgetChange"];
type Feature = components["schemas"]["AiFeatureRoute"];
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

function BudgetReading({ budget }: Readonly<{ budget: Budget }>) {
  const t = useT();
  const { locale } = useLocale();
  const number = (value: number) => formatNumber(value, locale);
  const pct = Math.round((budget.spent_tokens / budget.monthly_tokens) * 100);
  return (
    <>
      <p>
        {t("aiAdmin.consumption", {
          spent: number(budget.spent_tokens),
          total: number(budget.monthly_tokens),
          pct: number(pct),
        })}
      </p>
      <Meter
        value={Math.min(100, pct)}
        max={100}
        label={t("aiAdmin.allowance")}
      />
      <p>
        {t("aiAdmin.remaining", { tokens: number(budget.remaining_tokens) })} ·{" "}
        {t("aiAdmin.reset", {
          date: formatDateTime(budget.resets_at, locale, "UTC"),
        })}
      </p>
      <p>
        {budget.source === "company_override"
          ? t("aiAdmin.fixed")
          : t("aiAdmin.formula", {
              users: number(budget.eligible_full_users),
              tokens: number(budget.config.tokens_per_full_user),
            })}
        {budget.eligible_full_users === 0 &&
        budget.source !== "company_override"
          ? ` ${t("aiAdmin.floor")}`
          : ""}
      </p>
      <Callout
        kind="standing"
        tone={budget.band === "normal" ? "info" : "warn"}
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
      <h3>{t("aiAdmin.preview")}</h3>
      <BudgetReading budget={preview.proposed} />
      <p className="t-caption">{t("aiAdmin.previewHint")}</p>
      {canDiagnose && <DeferredWork rows={preview.deferred_work} />}
      {canRoute && (
        <Disclosure summary={t("aiAdmin.features")}>
          <AiFeatureTable rows={preview.features} />
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
      <BudgetReading budget={budget} />
      {save.isSuccess && (
        <Callout kind="outcome" tone="success" title={t("aiAdmin.saved")}>
          {t("aiAdmin.recovery")}
        </Callout>
      )}
      {!editing && canManage && (
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
            <Callout kind="standing" tone="warn" title={t("aiAdmin.stale")}>
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

export function AiFeaturesCard() {
  const t = useT();
  const canSee = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const canRoute = useCan("ai_routing", "read");
  const query = useAiStatus(canSee && canBudget);
  if (!canSee || !canBudget) return <AiFeaturesWithheldPanel />;
  return (
    <Panel title={t("aiAdmin.features")}>
      <PanelBody>
        <p>{t("aiAdmin.prospective")}</p>
        <QueryGate query={query} pendingLabel={t("aiAdmin.features")}>
          {(status) => (
            <>
              {canRoute && <AiFeatureTable rows={status.features} />}
              <DeferredWork rows={status.deferred_work} />
              <a href={routeHash(settingsHref("model-calls"))}>
                {t("aiAdmin.calls")}
              </a>
            </>
          )}
        </QueryGate>
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

export function AiFeatureTable({
  rows,
  onEdit,
}: Readonly<{ rows: Feature[]; onEdit?: (tier: string) => void }>) {
  const t = useT();
  const impact = (row: Feature) => {
    switch (row.impact) {
      case "budget_blocked":
        return t("aiAdmin.impact.blocked");
      case "model_changed":
        return t("aiAdmin.impact.model");
      case "fallback_changed":
        return t("aiAdmin.impact.fallback");
      case "unconfigured":
        return t("aiAdmin.impact.unconfigured");
      default:
        return row.budget_exempt
          ? t("aiAdmin.impact.exempt")
          : t("aiAdmin.impact.same");
    }
  };
  return (
    <DataTable
      label={t("aiAdmin.features")}
      rows={rows}
      rowKey={(row) => row.task}
      columns={[
        {
          key: "activity",
          header: t("aiAdmin.activity"),
          render: (row: Feature) => (
            <span title={row.task}>{row.display_name}</span>
          ),
        },
        {
          key: "model",
          header: t("aiAdmin.model"),
          render: (row: Feature) =>
            row.effective_candidates.length ? (
              <Disclosure
                summary={`${row.effective_candidates[0].provider} · ${row.effective_candidates[0].model}`}
              >
                <ol>
                  {row.effective_candidates.map((candidate) => (
                    <li key={candidate.tier}>
                      {candidate.provider} · {candidate.model} ·{" "}
                      {candidate.processing === "cloud_provider"
                        ? t("aiAdmin.cloud")
                        : t("aiAdmin.endpoint")}
                    </li>
                  ))}
                </ol>
                {onEdit && (
                  <Button small onClick={() => onEdit(row.leading_tier)}>
                    {t("aiAdmin.editBinding")}
                  </Button>
                )}
              </Disclosure>
            ) : (
              "—"
            ),
        },
        {
          key: "impact",
          header: t("aiAdmin.effect"),
          render: (row: Feature) => (
            <Badge
              tone={
                row.impact === "budget_blocked" || row.impact === "unconfigured"
                  ? "warn"
                  : undefined
              }
            >
              {impact(row)}
            </Badge>
          ),
        },
      ]}
    />
  );
}
