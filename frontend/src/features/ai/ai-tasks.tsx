// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, EmptyState, Field, Textarea } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate, QueryStates } from "../../design-system/querystates";
import { useSaveDraft } from "../../design-system/savepage";
import { formatDateTime, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useProviderHealth } from "../../lib/ai-provider-health";
import { useCan, useCanWrite } from "../../lib/capability";
import { useCompanyZone } from "../../lib/companyzone";
import {
  isVersionSkewOf,
  problemFieldSentencesOf,
  unwrap,
} from "../../lib/problem";
import { AiFeaturesWithheldPanel, useAiStatus } from "./ai-admin";
import { AiFeatureTable } from "./ai-feature-table";
import { useAiHealth } from "./ai-health";
import { TaskSheet } from "./ai-task-sheet";
import { PanelTitle } from "./ai-terms";

// What each AI task runs on right now, under the bindings above it.
//
// A task's tier is fixed by the task contract and bound on the Model tiers
// card; a row here opens the task's own request settings.
export function AiTasksCard() {
  const t = useT();
  const canManage = useCanWrite("ai_routing", "update");
  const [opened, setOpened] = useState<string | null>(null);
  const canDiagnose = useCan("ai_diagnostics", "read");
  const canBudget = useCan("ai_budget", "read");
  const canRoute = useCan("ai_routing", "read");
  const status = useAiStatus(canDiagnose && canBudget);
  const health = useAiHealth(canDiagnose).data;
  const providers = useProviderHealth(canDiagnose).data;
  // `/ai/status` needs all three: without the diagnostics or budget grant
  // there is no payload, and without routing read its task list is empty —
  // either way the withheld panel says so, rather than drawing a table that
  // reads as "no tasks".
  const route = status.data?.features.find((f) => f.task === opened);
  if (!canDiagnose || !canBudget || !canRoute) {
    return <AiFeaturesWithheldPanel />;
  }
  return (
    <Panel title={<PanelTitle term="task">{t("aiTasks.title")}</PanelTitle>}>
      <PanelBody>
        <PanelIntro>{t("aiTasks.intro")}</PanelIntro>
      </PanelBody>
      {status.isSuccess && status.data.features.length > 0 ? (
        <AiFeatureTable
          rows={status.data.features}
          health={health}
          providers={providers}
          lanesNeedingAKey={status.data.lanes_needing_a_key}
          canTrace={canDiagnose}
          canManage={canManage}
          onEdit={(row) => setOpened(row.task)}
        />
      ) : (
        <PanelBody>
          <QueryStates query={status} pendingLabel={t("aiTasks.title")}>
            <EmptyState>{t("common.empty")}</EmptyState>
          </QueryStates>
        </PanelBody>
      )}
      {/* Outside the gate, from the last good read: a failed refetch
          behind an open sheet must not take the draft in it away. */}
      {route ? (
        <TaskSheet
          route={route}
          canManage={canManage}
          canSeeCalls={canDiagnose}
          onClose={() => setOpened(null)}
        />
      ) : null}
    </Panel>
  );
}

type TeamRules = components["schemas"]["AiTeamRules"];

const TEAM_RULES_KEY = ["ai-team-rules"];
// The maxLength of text in crm.yaml (AiTeamRules).
const TEAM_RULES_MAX_LENGTH = 20000;

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
  const zone = useCompanyZone();
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
                when: formatDateTime(rules.updated_at, locale, zone),
              })
            : t("teamRules.none")}
        </p>
      </PanelBody>
    </Panel>
  );
}
