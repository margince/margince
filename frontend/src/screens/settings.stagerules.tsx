import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Switch } from "../design-system/switch";
import { formatDate, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, QueryStates, throwProblem } from "./common";

type TransitionPolicy = components["schemas"]["TransitionPolicy"];
type TransitionRecord = components["schemas"]["StageTransitionRecord"];

/**
 * What each transition is allowed to do, and the controls for changing it.
 *
 * SEPARATE from the evidence table above it, deliberately. That table is
 * read-only so a reader reads the record before touching the switch; this is
 * where they touch it, once they have. Merging the two would put a control in
 * every row of a table somebody is still reading.
 *
 * The rules are keyed by transition, and a transition with no rule is the
 * DEFAULT state of every transition in the product — it proposes. So this
 * draws a row per transition the report knows about rather than per rule,
 * or a transition nobody has decided about would be invisible here and the
 * page would look like it had nothing to offer.
 */
export function StageRulesCard({
  pipelineId,
  transitions,
  reportWindowDays,
}: Readonly<{
  pipelineId: string;
  transitions: readonly TransitionRecord[];
  /** The window the report's counts were taken over, so a rule measured over
   * a different one is not judged against them. */
  reportWindowDays: number;
}>) {
  const t = useT();
  // The report opens on `pipeline` READ, and every control here needs update.
  // A viewer who holds only the read sees the evidence and controls that
  // cannot work — so they are drawn refused, with the reason, rather than live
  // and 403ing on the first click.
  const mayChange = useCanWrite("pipeline", "update");
  const queryClient = useQueryClient();
  const rules = useQuery({
    queryKey: ["stage-automation", "policies", pipelineId],
    enabled: pipelineId !== "",
    queryFn: async () => {
      const { data, error } = await api.GET("/stage-automation/policies/{id}", {
        params: { path: { id: pipelineId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });

  const save = useMutation({
    mutationFn: async (next: {
      from: string;
      to: string;
      mode: "propose" | "auto";
      version?: number;
    }) => {
      const { data, error } = await api.PUT("/stage-automation/policies/{id}", {
        params: { path: { id: pipelineId } },
        body: {
          from_stage_id: next.from,
          to_stage_id: next.to,
          mode: next.mode,
          // The version the row was READ at, so a save cannot silently
          // overwrite a change somebody else made while this page was open.
          // Absent on a transition with no rule yet, because there was none
          // to have read.
          ...(next.version === undefined ? {} : { if_version: next.version }),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["stage-automation"] });
    },
  });

  if (transitions.length === 0) {
    return <EmptyState>{t("stageAutomation.noRules")}</EmptyState>;
  }

  const byTransition = new Map(
    (rules.data ?? []).map((rule) => [
      keyOf(rule.from_stage_id, rule.to_stage_id),
      rule,
    ]),
  );
  // The read's own pending and failed faces, rather than this card's: a
  // failure here replaces the whole section, and the shared pair keeps the
  // server's account of it and offers the reader the retry.
  return (
    <QueryStates query={rules} pendingLabel={t("stageAutomation.rulesLoading")}>
      <section>
        <h3 className="t-caption">{t("stageAutomation.rules")}</h3>
        <p className="t-caption">{t("stageAutomation.rulesIntro")}</p>
        {save.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("stageAutomation.saveFailed")}
          >
            {problemMessageOf(save.error, t)}
          </Callout>
        )}
        <ul className="u-list-reset">
          {transitions.map((row) => (
            <TransitionRule
              key={keyOf(row.from_stage_id, row.to_stage_id)}
              row={row}
              rule={byTransition.get(keyOf(row.from_stage_id, row.to_stage_id))}
              pipelineId={pipelineId}
              mayChange={mayChange}
              reportWindowDays={reportWindowDays}
              saving={save.isPending}
              onToggle={(mode, version) =>
                save.mutate({
                  from: row.from_stage_id,
                  to: row.to_stage_id,
                  mode,
                  version,
                })
              }
            />
          ))}
        </ul>
      </section>
    </QueryStates>
  );
}

/** One transition's switch, and its suspension if the product wrote one. */
function TransitionRule({
  row,
  rule,
  pipelineId,
  mayChange,
  reportWindowDays,
  saving,
  onToggle,
}: Readonly<{
  row: TransitionRecord;
  rule: TransitionPolicy | undefined;
  pipelineId: string;
  mayChange: boolean;
  reportWindowDays: number;
  saving: boolean;
  onToggle: (mode: "propose" | "auto", version?: number) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const label = `${row.from_stage_name} → ${row.to_stage_name}`;
  return (
    <li>
      <Switch
        label={label}
        hint={t("stageAutomation.modeHint")}
        checked={rule?.mode === "auto"}
        disabled={!mayChange}
        reason={mayChange ? undefined : t("stageAutomation.readOnly")}
        pending={saving}
        onChange={(on) => onToggle(on ? "auto" : "propose", rule?.version)}
      />
      {rule?.mode === "auto" && !rule.suspended_at && (
        <p className="t-caption">
          {notEarnedYet(row, rule, reportWindowDays)
            ? t("stageAutomation.notEarnedYet", {
                why: notEarnedYet(row, rule, reportWindowDays),
              })
            : t("stageAutomation.undoWindow", {
                // Through the locale: a bare String() prints the English
                // notation to every reader, and a magnitude in somebody
                // else's notation is one they read wrong.
                hours: formatNumber(rule.undo_window_hours, locale),
              })}
        </p>
      )}
      {rule?.suspended_at && (
        <SuspendedRule
          rule={rule}
          pipelineId={pipelineId}
          mayChange={mayChange}
          transitionLabel={label}
        />
      )}
    </li>
  );
}

/**
 * A rule the PRODUCT turned off, with the reason and a way back.
 *
 * The reason is drawn rather than summarised, because it is the whole basis on
 * which somebody decides to start the transition again — and a suspension
 * shown without one is a control asking for a decision it gave no grounds for.
 */
function SuspendedRule({
  rule,
  pipelineId,
  mayChange,
  transitionLabel,
}: Readonly<{
  rule: TransitionPolicy;
  pipelineId: string;
  mayChange: boolean;
  transitionLabel: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The READER's zone. A suspension stamped at 23:40 UTC happened on a
  // different day for half the contacts who will read this, and a date without a
  // zone shows one of them the wrong one.
  const zone = viewerZone();
  const [asking, setAsking] = useState(false);
  const queryClient = useQueryClient();
  const resume = useMutation({
    mutationFn: async () => {
      const { data, error } = await api.POST(
        "/stage-automation/policies/{id}/resume",
        {
          params: { path: { id: pipelineId } },
          body: {
            from_stage_id: rule.from_stage_id,
            to_stage_id: rule.to_stage_id,
          },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      setAsking(false);
      queryClient.invalidateQueries({ queryKey: ["stage-automation"] });
    },
  });

  return (
    <>
      <Callout
        tone="warn"
        kind="event"
        title={t("stageAutomation.suspended")}
        actions={
          mayChange ? (
            <Button onClick={() => setAsking(true)}>
              {t("stageAutomation.resume")}
            </Button>
          ) : undefined
        }
      >
        {rule.suspended_reason && <p>{rule.suspended_reason}</p>}
        {rule.suspended_at && (
          <p>
            {t("stageAutomation.suspendedSince", {
              date: formatDate(rule.suspended_at, locale, zone),
            })}
          </p>
        )}
      </Callout>
      <ConfirmModal
        open={asking}
        onClose={() => setAsking(false)}
        title={t("stageAutomation.resumeTitle")}
        confirmLabel={t("stageAutomation.resume")}
        onConfirm={() => resume.mutate()}
        pending={resume.isPending}
        error={resume.isError ? problemMessageOf(resume.error, t) : undefined}
      >
        <p>{transitionLabel}</p>
        <p>
          {t("stageAutomation.resumeBody", {
            reason: rule.suspended_reason ?? "",
          })}
        </p>
      </ConfirmModal>
    </>
  );
}

/**
 * Which threshold this transition has not met yet, or "" when it has met them
 * all.
 *
 * A switch that says ON while the server keeps proposing is the page's worst
 * failure: the admin turned it on, cards keep arriving, and nothing says the
 * two are consistent. The reasons are the same four the server checks and in
 * the same order — the one a reader can act on soonest first, because being
 * told acceptance is low when the volume bar is also unmet sends somebody
 * hunting for a quality problem that has not been measured yet.
 *
 * It is a RENDERING of the report beside it, never the authority. The server
 * re-checks in the transaction that would apply, and this can only be stale;
 * what it must never do is claim a transition is applying when it is not.
 */
function notEarnedYet(
  row: TransitionRecord,
  rule: TransitionPolicy,
  reportWindowDays: number,
): string {
  // A rule counted over a DIFFERENT window than the report cannot be judged
  // from it. The server takes each rule's rates over that rule's own
  // window_days; this row is whatever window the report was fetched with, and
  // comparing the two would say a transition has earned a bar it was never
  // measured against — in either direction.
  //
  // Silent rather than wrong: the switch still says what the admin asked for,
  // and the line that would explain a withholding is simply absent.
  if (rule.window_days !== reportWindowDays) {
    return "";
  }
  if (row.reviewed < rule.min_reviewed) {
    return `${row.reviewed}/${rule.min_reviewed}`;
  }
  if (row.observation_days < rule.min_observation_days) {
    return `${row.observation_days}/${rule.min_observation_days} d`;
  }
  if (row.clean_acceptance_rate < rule.clean_acceptance_threshold) {
    return comparison(
      row.clean_acceptance_rate,
      rule.clean_acceptance_threshold,
      "<",
    );
  }
  if (row.unsafe_rate > rule.correction_reversal_threshold) {
    return comparison(row.unsafe_rate, rule.correction_reversal_threshold, ">");
  }
  return "";
}

/**
 * Two rates and the sign between them, at a precision that keeps the sign true.
 *
 * Rounding each side independently prints comparisons that read as false:
 * 0.946 against 0.954 both round to 95%, so the line says "95% < 95%" and a
 * reader concludes the screen is broken. So the decimals grow until the two
 * numbers actually differ as printed — the branch was taken on the exact
 * values, and the text must not contradict it.
 */
function comparison(left: number, right: number, sign: "<" | ">"): string {
  for (const decimals of [0, 1, 2]) {
    const a = (left * 100).toFixed(decimals);
    const b = (right * 100).toFixed(decimals);
    if (a !== b) {
      return `${a}% ${sign} ${b}%`;
    }
  }
  // Closer than two decimals apart. The sign is still true — the caller only
  // reached here on the exact comparison — and printing more digits would
  // trade one confusion for another.
  return `${(left * 100).toFixed(2)}% ${sign} ${(right * 100).toFixed(2)}%`;
}

// One transition's identity, spelled once so the map and the lookup cannot
// disagree about what a key looks like.
function keyOf(from: string, to: string): string {
  return `${from}-${to}`;
}
