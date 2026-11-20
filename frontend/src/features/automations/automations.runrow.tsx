// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Ban, Check, Clock, type LucideIcon, Minus, X } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Badge } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { AutonomyDot } from "../../design-system/trust";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { RUN_OUTCOME_WORD, runReasonWords } from "../../lib/automationrun";
import { recordRoute } from "../../lib/entity";
import { routeHash } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import "./automationdetail.css";
import {
  runSentences,
  runTrigger,
  unnamedTarget,
} from "./automations.runwords";

// One run of an automation, as the run history lists it.

type AutomationRun = components["schemas"]["AutomationRun"];
type Outcome = AutomationRun["outcome"];

// Outcome → tone, glyph and word. A Record over the enum stops compiling when
// the contract adds an outcome, so a new one cannot ship as a blank badge.
export const OUTCOME_LOOK: Readonly<
  Record<
    Outcome,
    Readonly<{
      tone: "success" | "warning" | "danger";
      icon: LucideIcon;
      word: MessageKey;
    }>
  >
> = {
  fired: { tone: "success", icon: Check, word: RUN_OUTCOME_WORD.fired },
  failed: { tone: "danger", icon: X, word: RUN_OUTCOME_WORD.failed },
  blocked: { tone: "warning", icon: Ban, word: RUN_OUTCOME_WORD.blocked },
  skipped: { tone: "warning", icon: Minus, word: RUN_OUTCOME_WORD.skipped },
  queued_for_approval: {
    tone: "warning",
    icon: Clock,
    word: RUN_OUTCOME_WORD.queued_for_approval,
  },
};

export function OutcomeBadge({ outcome }: Readonly<{ outcome: Outcome }>) {
  const t = useT();
  const look = OUTCOME_LOOK[outcome];
  return (
    <Badge tone={look.tone} icon={look.icon}>
      {t(look.word)}
    </Badge>
  );
}

// The reason line reads in its badge's tone, so a run's row reads one way.
function reasonColor(outcome: Outcome): string {
  return OUTCOME_LOOK[outcome].tone === "danger"
    ? "var(--dangerText)"
    : "var(--warningText)";
}

// A labelled detail line, drawn only when its value is present.
function DetailLine({
  label,
  value,
  color,
}: Readonly<{ label: string; value: ReactNode; color?: string }>) {
  return (
    <p className="t-caption auto-detail-line" style={{ color }}>
      <span className="t-label">{label}</span> {value}
    </p>
  );
}

// The record a run acted on: a link where it has a name and a page, else its
// name. A record the reader may not see is named by its kind alone.
function RunTarget({ run }: Readonly<{ run: AutomationRun }>) {
  const t = useT();
  const target = run.target;
  if (!target?.label) return <>{unnamedTarget(target?.type, t)}</>;
  const route = recordRoute(target.type, target.id);
  return route ? (
    <AppLink href={routeHash(route)}>{target.label}</AppLink>
  ) : (
    target.label
  );
}

function RunReason({ run }: Readonly<{ run: AutomationRun }>) {
  const t = useT();
  if (!run.reason_code && !run.reason) return null;
  return (
    <DetailLine
      label={t("auto.runs.reason")}
      value={
        run.reason_code
          ? runReasonWords(run.reason_code, t)
          : (run.reason ?? "")
      }
      color={reasonColor(run.outcome)}
    />
  );
}

// What the run did and to which record. A server that sends no action kinds,
// or one this client predates, keeps its own lines.
function RunAction({ run }: Readonly<{ run: AutomationRun }>) {
  const t = useT();
  const sentences = runSentences(run, t);
  if (sentences) {
    return (
      <>
        {sentences.map(([before, after]) => (
          <p key={before + after} className="auto-run-sentence">
            {before}
            <RunTarget run={run} />
            {after}
          </p>
        ))}
      </>
    );
  }
  return (
    <>
      {run.target ? (
        <DetailLine
          label={t("auto.runs.target")}
          value={<RunTarget run={run} />}
        />
      ) : (
        run.target_ref && (
          <DetailLine label={t("auto.runs.target")} value={run.target_ref} />
        )
      )}
      {run.action_result && (
        <DetailLine label={t("auto.runs.result")} value={run.action_result} />
      )}
    </>
  );
}

export function RunRow({ run }: Readonly<{ run: AutomationRun }>) {
  const t = useT();
  const { locale } = useLocale();
  // In the viewer's own zone, as every audit-style event reads.
  const zone = useViewerZone();
  const trigger = runTrigger(run, t);
  const tier = run.tier === "auto_execute" ? "auto" : "confirm";
  return (
    <li className="auto-run">
      <div className="auto-run-head">
        <time
          className="t-caption"
          dateTime={run.occurred_at}
          title={run.occurred_at}
        >
          {formatDateTime(run.occurred_at, locale, zone)}
        </time>
        <OutcomeBadge outcome={run.outcome} />
        {!run.event_id && <AutonomyDot tier={tier} />}
        {run.approval_required && (
          <span className="t-caption">{t("auto.runs.needsApproval")}</span>
        )}
      </div>
      <RunAction run={run} />
      {trigger ? (
        <DetailLine label={t("auto.runs.when")} value={trigger} />
      ) : (
        run.trigger_evidence && (
          <DetailLine label={t("auto.runs.why")} value={run.trigger_evidence} />
        )
      )}
      <RunReason run={run} />
      {run.event_id && (
        <Disclosure
          className="auto-run-technical"
          summary={t("auto.runs.technical")}
        >
          <DetailLine label={t("auto.runs.event")} value={run.event_id} />
          <DetailLine
            label={t("auto.runs.mode")}
            value={<AutonomyDot tier={tier} withLabel />}
          />
        </Disclosure>
      )}
    </li>
  );
}
