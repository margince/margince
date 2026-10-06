// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useUnsavedGuard } from "../app/unsaved";
import { Badge, Button, Field, SegmentedControl } from "../design-system/atoms";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  useCallStats,
  useTaskFlow,
  useWindowLabels,
  WINDOWS,
  type Window,
} from "./ai-call-figures";
import { tierLabel } from "./ai-decision-labels";
import { LatencyAgainstTimeout, TaskOutcome } from "./ai-task-outcome";
import { ReadProblem, SaveProblem, TimeoutField } from "./ai-task-sheet-fields";
import { CALL_TASK_PARAM, callsHrefFor } from "./aicalls";
import { problemCodeOf, throwProblem } from "./common";
import "./ai-settings.css";

// One task's request settings, and what its calls did under them. The figures
// come first because they are what an admin decides by: how many calls got an
// answer, and how close the slow ones run to the timeout.

type Feature = components["schemas"]["AiFeatureRoute"];
type Overrides = components["schemas"]["AiTaskOverrides"];
type Override = components["schemas"]["AiTaskOverride"];
type Thinking = NonNullable<Override["thinking"]>;

const OVERRIDES_KEY = ["ai-task-overrides"];
const THINKING: readonly Thinking[] = ["minimal", "low", "medium", "high"];
const DEFAULT_THINKING = "";
const ADD_TASK_GUIDE =
  "https://github.com/margince/margince/blob/main/docs/how-to/add-an-ai-task.md";
const THINKING_GUIDE =
  "https://github.com/margince/margince/blob/main/docs/reference/ai-thinking.md";

/** The bounds a save is held to, in seconds, as the server's own validation. */
const DECISION_BOUNDS = [5, 60] as const;
const ATTEMPT_BOUNDS = [10, 300] as const;
const THINKING_HELP: Readonly<
  Record<Thinking | typeof DEFAULT_THINKING, MessageKey>
> = {
  "": "aiTaskSheet.thinking.default.help",
  minimal: "aiTaskSheet.thinking.minimal.help",
  low: "aiTaskSheet.thinking.low.help",
  medium: "aiTaskSheet.thinking.medium.help",
  high: "aiTaskSheet.thinking.high.help",
};

export function useTaskOverrides(enabled: boolean) {
  return useQuery({
    queryKey: OVERRIDES_KEY,
    enabled,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/ai/task-overrides");
      if (error) throwProblem(error);
      return { overrides: data, version: response.headers.get("ETag") ?? "" };
    },
  });
}

type Draft = Readonly<{
  thinking: Thinking | typeof DEFAULT_THINKING;
  decision: number;
  attempt: number;
}>;

function draftOf(override: Override | undefined, defaults: Draft): Draft {
  return {
    thinking: override?.thinking ?? DEFAULT_THINKING,
    decision: override?.decision_timeout_ms
      ? override.decision_timeout_ms / 1000
      : defaults.decision,
    attempt: override?.attempt_timeout_ms
      ? override.attempt_timeout_ms / 1000
      : defaults.attempt,
  };
}

/** The override a draft says: only what departs from the defaults is written. */
export function overrideOf(
  draft: Draft,
  defaults: Draft,
  decides: boolean,
): Override {
  const out: Override = {};
  if (draft.thinking) out.thinking = draft.thinking;
  if (decides && draft.decision !== defaults.decision)
    out.decision_timeout_ms = draft.decision * 1000;
  if (draft.attempt !== defaults.attempt)
    out.attempt_timeout_ms = draft.attempt * 1000;
  return out;
}

function same(a: Draft, b: Draft): boolean {
  return (
    a.thinking === b.thinking &&
    a.decision === b.decision &&
    a.attempt === b.attempt
  );
}

export function TaskSheet({
  route,
  canManage,
  canSeeCalls,
  onClose,
}: Readonly<{
  route: Feature;
  canManage: boolean;
  canSeeCalls: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const stored = useTaskOverrides(true);
  const decides = route.decides === true;
  const defaults = defaultsOf(route);
  const saved = draftOf(stored.data?.overrides[route.task], defaults);
  const [edited, setEdited] = useState<Draft | null>(null);
  const draft = edited ?? saved;
  const changed = !same(draft, saved);
  useUnsavedGuard(changed);
  const save = useSaveOverrides(onClose);
  const submit = () => {
    if (!stored.data) return;
    const next = {
      ...stored.data.overrides,
      [route.task]: overrideOf(draft, defaults, decides),
    };
    save.mutate({ overrides: next, version: stored.data.version });
  };
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer-reading">
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {route.display_name}
        </Heading>
        <p className="t-caption">
          {[
            route.task,
            decides ? t("aiTaskSheet.decision") : null,
            route.execution_mode,
          ]
            .filter(Boolean)
            .join(" · ")}
        </p>
        {decides ? <Badge>{t("aiTasks.decisionFirst")}</Badge> : null}
      </DrawerHead>
      <DrawerBody>
        {canSeeCalls ? (
          <TaskRecentCalls route={route} draft={draft} decides={decides} />
        ) : null}
        <ReadProblem
          error={stored.isError ? stored.error : null}
          onRetry={() => stored.refetch()}
        />
        <TaskSettings
          draft={draft}
          defaults={defaults}
          decides={decides}
          canManage={canManage}
          busy={!canManage || save.isPending || !stored.data}
          onChange={setEdited}
        />
        <ContractFacts route={route} />
        <SaveProblem error={save.isError ? save.error : null} />
      </DrawerBody>
      <DrawerFoot className="actions">
        <span className="t-caption actions-lead">
          {changed ? t("aiTaskSheet.unsaved") : t("aiTaskSheet.applies")}
        </span>
        <span className="actions-pair">
          <Button onClick={onClose} disabled={save.isPending}>
            {t("aiAdmin.cancel")}
          </Button>
          <Button
            variant="primary"
            pending={save.isPending}
            disabled={!canManage || !changed || !stored.data}
            reason={canManage ? undefined : t("aiRouting.adminOnly")}
            onClick={submit}
          >
            {t("aiTaskSheet.save")}
          </Button>
        </span>
      </DrawerFoot>
    </Modal>
  );
}

/** What a task is sent with when nothing overrides it, in seconds. */
function defaultsOf(route: Feature): Draft {
  return {
    thinking: DEFAULT_THINKING,
    decision: (route.defaults?.decision_timeout_ms ?? 15000) / 1000,
    attempt: (route.defaults?.attempt_timeout_ms ?? 300000) / 1000,
  };
}

function useSaveOverrides(onSaved: () => void) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (vars: { overrides: Overrides; version: string }) => {
      const { error } = await api.PUT("/ai/task-overrides", {
        // Always sent: an absent If-Match is an unconditional overwrite.
        headers: { "If-Match": vars.version },
        body: vars.overrides,
      });
      if (error) throwProblem(error);
    },
    // A colleague's newer save is read back, so the next Save is held to it
    // rather than to the revision this sheet opened on.
    onError: async (error) => {
      if (problemCodeOf(error) === "version_skew") {
        await queryClient.invalidateQueries({ queryKey: OVERRIDES_KEY });
      }
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: OVERRIDES_KEY });
      await queryClient.invalidateQueries({ queryKey: ["ai-status"] });
      onSaved();
    },
  });
}

function TaskSettings({
  draft,
  defaults,
  decides,
  canManage,
  busy,
  onChange,
}: Readonly<{
  draft: Draft;
  defaults: Draft;
  decides: boolean;
  canManage: boolean;
  busy: boolean;
  onChange: (next: Draft) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const set = (patch: Partial<Draft>) => onChange({ ...draft, ...patch });
  return (
    <section className="ai-sheet-section">
      <div className="ai-figures-head">
        <Heading size="small" className="t-h3">
          {t("aiTaskSheet.settings")}
        </Heading>
        <a
          className="t-caption"
          href={THINKING_GUIDE}
          target="_blank"
          rel="noreferrer"
        >
          {t("aiTaskSheet.thinkingGuide")}
        </a>
      </div>
      <Field
        label={t("aiTaskSheet.thinking")}
        hint={t(THINKING_HELP[draft.thinking])}
      >
        {(control) => (
          <Select
            {...control}
            value={draft.thinking}
            disabled={busy}
            options={[
              {
                value: DEFAULT_THINKING,
                label: t("aiTaskSheet.thinking.default"),
              },
              ...THINKING.map((level) => ({ value: level, label: level })),
            ]}
            onChange={(value) =>
              set({
                thinking: THINKING.find((l) => l === value) ?? DEFAULT_THINKING,
              })
            }
          />
        )}
      </Field>
      {decides ? (
        <TimeoutField
          label={t("aiTaskSheet.decisionTimeout")}
          hint={t("aiTaskSheet.decisionTimeout.help", {
            low: formatNumber(DECISION_BOUNDS[0], locale),
            high: formatNumber(DECISION_BOUNDS[1], locale),
          })}
          bounds={DECISION_BOUNDS}
          value={draft.decision}
          fallback={defaults.decision}
          disabled={busy}
          onChange={(decision) => set({ decision })}
        />
      ) : null}
      <TimeoutField
        label={t("aiTaskSheet.attemptTimeout")}
        hint={t(
          decides
            ? "aiTaskSheet.attemptTimeout.help.decision"
            : "aiTaskSheet.attemptTimeout.help",
          {
            low: formatNumber(ATTEMPT_BOUNDS[0], locale),
            high: formatNumber(ATTEMPT_BOUNDS[1], locale),
          },
        )}
        bounds={ATTEMPT_BOUNDS}
        value={draft.attempt}
        fallback={defaults.attempt}
        disabled={busy}
        onChange={(attempt) => set({ attempt })}
      />
      {!same(draft, defaults) && canManage ? (
        <Button
          variant="link"
          disabled={busy}
          onClick={() => onChange(defaults)}
        >
          {t("aiTaskSheet.reset")}
        </Button>
      ) : null}
    </section>
  );
}

function ContractFacts({ route }: Readonly<{ route: Feature }>) {
  const t = useT();
  return (
    <section className="ai-sheet-section">
      <Heading size="small" className="t-h3">
        {t("aiTaskSheet.fixed")}
      </Heading>
      <dl className="ai-task-facts">
        <dt>{t("aiTaskSheet.tiersTried")}</dt>
        <dd>
          {route.normal_candidates
            .map((c) => tierLabel(c.tier, t))
            .join(" → ") || "—"}
        </dd>
        <dt>{t("aiTaskSheet.runs")}</dt>
        <dd>{route.execution_mode}</dd>
      </dl>
      <p className="t-caption">
        {t("aiTaskSheet.fixed.help")}{" "}
        <a href={ADD_TASK_GUIDE} target="_blank" rel="noreferrer">
          {t("aiTaskSheet.addTask")}
        </a>
      </p>
    </section>
  );
}

function TaskRecentCalls({
  route,
  draft,
  decides,
}: Readonly<{ route: Feature; draft: Draft; decides: boolean }>) {
  const t = useT();
  const [window, setWindow] = useState<Window>("7d");
  const windows = useWindowLabels();
  const flow = useTaskFlow(route.task, window, true);
  // The latency read against the timeout that stops it: the decision model's
  // calls on a deciding task, every attempt otherwise.
  const stats = useCallStats(
    { window, task: route.task, group: decides ? "tier" : "task" },
    true,
  );
  const row = stats.data?.rows.find((r) =>
    decides ? r.key === "decide" : r.key === route.task,
  );
  return (
    <section className="ai-sheet-section">
      <div className="ai-figures-head">
        <Heading size="small" className="t-h3">
          {t("aiFigures.recentCalls")}
        </Heading>
        <SegmentedControl
          label={t("aiFigures.window")}
          options={WINDOWS}
          value={window}
          labels={windows}
          onChange={setWindow}
        />
      </div>
      {flow.data ? (
        <TaskOutcome flow={flow.data} />
      ) : (
        <p className="t-caption">
          {flow.isError ? t("aiFigures.unread") : t("aiFigures.pending")}
        </p>
      )}
      {row && row.calls > 0 ? (
        <LatencyAgainstTimeout
          p50Ms={row.p50_ms}
          p95Ms={row.p95_ms}
          timeoutMs={(decides ? draft.decision : draft.attempt) * 1000}
          decision={decides}
        />
      ) : null}
      <p className="t-caption">
        <a href={callsHrefFor({ [CALL_TASK_PARAM]: route.task })}>
          {t("aiTaskSheet.viewCalls")}
        </a>{" "}
        {t("aiTaskSheet.hostsUnder")}
      </p>
    </section>
  );
}
