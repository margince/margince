// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useUnsavedGuard } from "../../components/unsaved";
import {
  Button,
  Disclosure,
  EmptyState,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { Meter } from "../../design-system/readings";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { useSaveDraft } from "../../design-system/savepage";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatDateTime, formatNumber } from "../../format/format";
import { lockFor } from "../../format/settinglocks";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import {
  isVersionSkewOf,
  problemFieldSentencesOf,
  problemMessageOf,
  unwrap,
} from "../../lib/problem";
import { wholeNumberFault } from "../../lib/wholenumber";
import { AiFeatureTable } from "./ai-feature-table";
import { formatTokens } from "./tokens";
import "./ai-admin.css";

type Budget = components["schemas"]["AiBudgetSnapshot"];
type Change = components["schemas"]["AiBudgetChange"];
type Deferred = components["schemas"]["AiDeferredWork"];
type TeamRules = components["schemas"]["AiTeamRules"];

const TEAM_RULES_KEY = ["ai-team-rules"];
// The maxLength of text in crm.yaml (AiTeamRules).
const TEAM_RULES_MAX_LENGTH = 20000;

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
  // The allowance is one setting: a boot source that sets it refuses the editor.
  const lock = lockFor(budget?.locks, "ai.budget", t);
  return (
    <Panel
      title={t("aiAdmin.allowance")}
      titleAction={
        budget && canManage ? (
          <Button
            reason={lock.disabled ? lock.reason : undefined}
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
          {(budget.band !== "normal" || saved) && (
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
            </PanelBody>
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
  const plural = usePlural();
  const { locale } = useLocale();
  const number = (value: number) => formatNumber(value, locale);
  const tokens = (value: number) => formatTokens(value, locale);
  const pct = Math.round((budget.spent_tokens / budget.monthly_tokens) * 100);
  const basis =
    budget.source === "company_override"
      ? t("aiAdmin.fixed")
      : plural("aiAdmin.formula", budget.config.tokens_per_full_user, {
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

function companyText(config: Budget["config"]): string {
  return config.company_monthly_tokens === null
    ? ""
    : String(config.company_monthly_tokens);
}

// The server's bounds on either allowance, in whole tokens.
const ALLOWANCE_BOUNDS = { min: 1, max: 1e12 } as const;

function BudgetEditor({
  budget,
  onClose,
  onSaved,
}: Readonly<{ budget: Budget; onClose: () => void; onSaved: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const [perUser, setPerUser] = useState(() =>
    String(budget.config.tokens_per_full_user),
  );
  const [company, setCompany] = useState(() => companyText(budget.config));
  const [revision, setRevision] = useState(budget.revision);
  // The values the editor opened on, so a save elsewhere is not a draft here.
  const [opened, setOpened] = useState(budget.config);
  const perUserBox = useRef<HTMLInputElement | null>(null);
  useUnsavedGuard(
    perUser !== String(opened.tokens_per_full_user) ||
      company !== companyText(opened),
  );
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
  const perUserFault = wholeNumberFault(
    perUser,
    locale,
    t("aiAdmin.perUser"),
    ALLOWANCE_BOUNDS,
  );
  const companyFault =
    company.trim() === ""
      ? undefined
      : wholeNumberFault(
          company,
          locale,
          t("aiAdmin.companyName"),
          ALLOWANCE_BOUNDS,
        );
  const valid = perUserFault === undefined && companyFault === undefined;
  const change: Change = {
    config: {
      tokens_per_full_user: Number(perUser.trim()),
      company_monthly_tokens:
        company.trim() === "" ? null : Number(company.trim()),
    },
    expected_revision: revision,
  };
  const busy = preview.isPending || save.isPending;
  const reload = () => {
    setPerUser(String(budget.config.tokens_per_full_user));
    setCompany(companyText(budget.config));
    setRevision(budget.revision);
    setOpened(budget.config);
    preview.reset();
    save.reset();
    // The button goes with the callout, so focus lands on the first value.
    perUserBox.current?.focus();
  };
  return (
    <RecordFormDialog
      open
      title={t("aiAdmin.allowance")}
      onClose={onClose}
      fieldCount={2}
      closeDisabled={busy}
      form={
        <div className="form-stack">
          <div className="form-row">
            <Field
              label={t("aiAdmin.perUser")}
              hint={t("aiAdmin.range")}
              error={perUserFault}
            >
              {(control) => (
                <TextInput
                  {...control}
                  ref={perUserBox}
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
              error={companyFault}
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
            <Callout
              kind="standing"
              tone="warning"
              title={t("aiAdmin.stale")}
              actions={<Button onClick={reload}>{t("aiAdmin.reload")}</Button>}
            >
              {t("aiAdmin.staleHelp")}
            </Callout>
          )}
          {(preview.isError || save.isError) && (
            <Callout kind="outcome" tone="danger" title={t("aiAdmin.failed")}>
              {problemMessageOf(preview.error ?? save.error, t)}
            </Callout>
          )}
          {preview.data && <BudgetPreview preview={preview.data} />}
        </div>
      }
      actions={
        <>
          <Button
            disabled={revision !== budget.revision}
            reason={valid ? undefined : t("aiAdmin.fixToPreview")}
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
        </>
      }
    />
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

/** Settings → AI → Rules: the one text every AI draft in the workspace follows. */
export function TeamRulesCard() {
  const t = useT();
  const query = useQuery({
    queryKey: TEAM_RULES_KEY,
    queryFn: async () => {
      const data = unwrap(await api.GET("/ai/team-rules"));
      if (!data) throw new Error("Team rules are unavailable");
      return data;
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("teamRules.title")}>
      {(rules) => <TeamRulesEditor rules={rules} />}
    </QueryGate>
  );
}

// What the editor opened on, kept apart from the latest server answer. A save
// is judged against the version the text was edited from, never a newer one.
type TeamRulesEdit = Readonly<{
  text: string;
  baseText: string;
  baseUpdatedAt: string | undefined;
}>;

function TeamRulesEditor({ rules }: Readonly<{ rules: TeamRules }>) {
  const t = useT();
  const { locale } = useLocale();
  const client = useQueryClient();
  const canEdit = useCanWrite("ai_team_rules", "update");
  const [edit, setEdit] = useState<TeamRulesEdit | null>(null);
  const [skewed, setSkewed] = useState(false);
  const text = edit?.text ?? rules.text;
  const dirty = edit !== null && edit.text !== edit.baseText;
  // Somebody saved while this text was being edited: the server moved on, or a
  // save was refused as stale.
  const conflict = dirty && (skewed || edit.baseUpdatedAt !== rules.updated_at);
  const settle = () => {
    setEdit(null);
    setSkewed(false);
  };
  const save = useMutation({
    mutationFn: async (from: TeamRulesEdit) =>
      unwrap(
        await api.PUT("/ai/team-rules", {
          body: { text: from.text, expected_updated_at: from.baseUpdatedAt },
        }),
      ),
    onSuccess: (data) => client.setQueryData(TEAM_RULES_KEY, data),
    onError: (error) => {
      if (isVersionSkewOf(error)) {
        setSkewed(true);
        void client.invalidateQueries({ queryKey: TEAM_RULES_KEY });
      }
    },
  });
  useSaveDraft({
    dirty,
    save: async () => {
      if (edit === null) return;
      await save.mutateAsync(edit);
      settle();
    },
    placed: ["text"],
    discard: settle,
  });
  const faults = problemFieldSentencesOf(save.error, locale);
  return (
    <Panel title={t("teamRules.title")}>
      <PanelBody className="form-stack">
        <PanelIntro>{t("teamRules.intro")}</PanelIntro>
        <Field
          label={t("teamRules.label")}
          hint={t("teamRules.length", {
            used: formatNumber(text.length, locale),
            max: formatNumber(TEAM_RULES_MAX_LENGTH, locale),
          })}
          error={faults.get("text")}
        >
          {(control) => (
            <Textarea
              {...control}
              rows={12}
              maxLength={TEAM_RULES_MAX_LENGTH}
              disabled={!canEdit || save.isPending}
              value={text}
              onChange={(event) =>
                setEdit({
                  text: event.target.value,
                  baseText: edit?.baseText ?? rules.text,
                  baseUpdatedAt: edit ? edit.baseUpdatedAt : rules.updated_at,
                })
              }
            />
          )}
        </Field>
        {conflict && (
          <Callout
            kind="outcome"
            tone="warning"
            title={t("teamRules.conflictTitle")}
            actions={
              <>
                <Button onClick={settle}>{t("teamRules.reload")}</Button>
                <Button
                  onClick={() => {
                    setEdit({
                      text,
                      baseText: rules.text,
                      baseUpdatedAt: rules.updated_at,
                    });
                    setSkewed(false);
                  }}
                >
                  {t("teamRules.keepMine")}
                </Button>
              </>
            }
          >
            {t("teamRules.conflictBody")}
          </Callout>
        )}
        <p className="t-caption">
          {rules.updated_at
            ? t("teamRules.savedBy", {
                who: rules.updated_by ?? t("teamRules.unknownEditor"),
                when: formatDateTime(rules.updated_at, locale, "UTC"),
              })
            : t("teamRules.none")}
        </p>
      </PanelBody>
    </Panel>
  );
}
