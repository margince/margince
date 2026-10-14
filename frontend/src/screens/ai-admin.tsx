// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
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
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryStates } from "../design-system/querystates";
import { Meter } from "../design-system/readings";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDateTime, formatNumber } from "../format/format";
import { formatTokens } from "../format/tokens";
import { useLocale, useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import { problemMessageOf, unwrap } from "../lib/problem";
import { AiFeatureTable } from "./ai-feature-table";
import "./ai-admin.css";

type Budget = components["schemas"]["AiBudgetSnapshot"];
type Change = components["schemas"]["AiBudgetChange"];
type Deferred = components["schemas"]["AiDeferredWork"];

export function useAiStatus(enabled: boolean) {
  return useQuery({
    queryKey: ["ai-status"],
    enabled,
    refetchInterval: 60_000,
    queryFn: async () => {
      const data = unwrap(await api.GET("/ai/status"));
      if (!data) throw new Error("AI status is unavailable");
      return data;
    },
  });
}

export function AiBudgetCard() {
  const t = useT();
  const canSee = useCan("ai_budget", "read");
  const canManage = useCanWrite("ai_budget", "update");
  const [editing, setEditing] = useState(false);
  const [saved, setSaved] = useState(false);
  useUnsavedGuard(editing);
  const query = useQuery({
    queryKey: ["ai-budget"],
    enabled: canSee,
    refetchInterval: 60_000,
    queryFn: async () => {
      const data = unwrap(await api.GET("/ai/budget"));
      if (!data) throw new Error("AI allowance is unavailable");
      return data;
    },
  });
  if (!canSee) return null;
  const budget = query.data;
  return (
    <Panel
      title={t("aiAdmin.allowance")}
      titleAction={
        budget && canManage && !editing ? (
          <Button
            onClick={() => {
              setSaved(false);
              setEditing(true);
            }}
          >
            {t("aiAdmin.edit")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <PanelIntro>{t("aiAdmin.pool")}</PanelIntro>
        {budget === undefined && (
          <QueryStates query={query} pendingLabel={t("aiAdmin.allowance")}>
            {null}
          </QueryStates>
        )}
      </PanelBody>
      {budget && (
        <>
          <SettingList bleed="settings">
            <BudgetReading budget={budget} />
          </SettingList>
          {(budget.band !== "normal" || saved || editing) && (
            <PanelBody className="form-stack">
              <BandNote band={budget.band} />
              {saved && (
                <Callout
                  kind="outcome"
                  tone="success"
                  title={t("aiAdmin.saved")}
                >
                  {t("aiAdmin.recovery")}
                </Callout>
              )}
              {editing && (
                <BudgetEditor
                  budget={budget}
                  onClose={() => setEditing(false)}
                  onSaved={() => {
                    setEditing(false);
                    setSaved(true);
                  }}
                />
              )}
            </PanelBody>
          )}
        </>
      )}
    </Panel>
  );
}

// Said only off the normal band: a standing note that all is well is noise
// beside a meter that already shows it.
function BandNote({ band }: Readonly<{ band: Budget["band"] }>) {
  const t = useT();
  if (band === "normal") return null;
  return (
    <Callout
      kind="standing"
      tone="warning"
      title={band === "degraded" ? t("aiAdmin.degraded") : t("aiAdmin.queued")}
    >
      {t("aiAdmin.policy")}
    </Callout>
  );
}

function BudgetReading({ budget }: Readonly<{ budget: Budget }>) {
  const t = useT();
  const { locale } = useLocale();
  const number = (value: number) => formatNumber(value, locale);
  const tokens = (value: number) => formatTokens(value, locale);
  const pct = Math.round((budget.spent_tokens / budget.monthly_tokens) * 100);
  const basis =
    budget.source === "company_override"
      ? t("aiAdmin.fixed")
      : t("aiAdmin.formula", {
          users: number(budget.eligible_full_users),
          tokens: tokens(budget.config.tokens_per_full_user),
        });
  return (
    <>
      <SettingRow
        label={t("aiAdmin.used")}
        value={
          <span className="ai-budget-used">
            <Meter
              value={Math.min(100, pct)}
              max={100}
              label={t("aiAdmin.allowance")}
            />
            <span>
              {t("aiAdmin.consumption", {
                spent: tokens(budget.spent_tokens),
                total: tokens(budget.monthly_tokens),
                pct: number(pct),
              })}
            </span>
          </span>
        }
        control={null}
      />
      <SettingRow
        label={t("aiAdmin.resets")}
        value={t("aiAdmin.reset", {
          date: formatDateTime(budget.resets_at, locale, "UTC"),
        })}
        control={null}
      />
      <SettingRow
        label={t("aiAdmin.basis")}
        value={
          budget.eligible_full_users === 0 &&
          budget.source !== "company_override"
            ? `${basis} ${t("aiAdmin.floor")}`
            : basis
        }
        control={null}
      />
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
      <Heading size="xsmall" as="h3">
        {t("aiAdmin.preview")}
      </Heading>
      <SettingList>
        <BudgetReading budget={preview.proposed} />
      </SettingList>
      <BandNote band={preview.proposed.band} />
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

function BudgetEditor({
  budget,
  onClose,
  onSaved,
}: Readonly<{ budget: Budget; onClose: () => void; onSaved: () => void }>) {
  const t = useT();
  const client = useQueryClient();
  const [perUser, setPerUser] = useState(() =>
    String(budget.config.tokens_per_full_user),
  );
  const [company, setCompany] = useState(() =>
    budget.config.company_monthly_tokens === null
      ? ""
      : String(budget.config.company_monthly_tokens),
  );
  const [revision] = useState(budget.revision);
  const preview = useMutation({
    mutationFn: async (change: Change) => {
      return unwrap(
        await api.POST("/ai/budget/preview", {
          body: change,
        }),
      );
    },
  });
  const save = useMutation({
    mutationFn: async (change: Change) => {
      return unwrap(await api.PUT("/ai/budget", { body: change }));
    },
    onSuccess: async (data) => {
      client.setQueryData(["ai-budget"], data);
      onSaved();
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
        <Field label={t("aiAdmin.company")} hint={t("aiAdmin.overrideHint")}>
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
        <Button disabled={busy} onClick={onClose}>
          {t("aiAdmin.cancel")}
        </Button>
      </div>
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
            {row.available && row.waiting_on_provider ? (
              <div>
                {t("aiAdmin.providerWaiting", {
                  waiting: formatNumber(row.waiting_on_provider, locale),
                })}
              </div>
            ) : null}
          </li>
        ))}
      </ul>
      <p>{t("aiAdmin.recovery")}</p>
    </Disclosure>
  );
}
