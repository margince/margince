// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Ban, Check, Clock, type LucideIcon, Minus, X } from "lucide-react";
import { type ReactNode, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  EmptyState,
  SegmentedControl,
} from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { AutonomyDot } from "../design-system/trust";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { LoadMoreButton, QueryStates, unwrap } from "./common";
// The shapes this file draws. Imported here rather than left to the caller:
// the row that folds these open lives in another sheet, and a story or a test
// that mounts one panel alone would otherwise render it unstyled.
import "./automationdetail.css";

// The human surface for the two human-only automation ops
// (listAutomationRuns / previewAutomation), in its own file so automations.tsx
// stays legible. Both panels are pure reads; neither writes.

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
  fired: { tone: "success", icon: Check, word: "auto.runs.outcomeFired" },
  failed: { tone: "danger", icon: X, word: "auto.runs.outcomeFailed" },
  blocked: { tone: "warning", icon: Ban, word: "auto.runs.outcomeBlocked" },
  skipped: { tone: "warning", icon: Minus, word: "auto.runs.outcomeSkipped" },
  queued_for_approval: {
    tone: "warning",
    icon: Clock,
    word: "auto.runs.outcomeQueued",
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

// A labelled detail line, rendered ONLY by the caller when the field is
// present — never a blank "Label:" row for a null optional field (T7).
function DetailLine({
  label,
  value,
  color,
}: Readonly<{ label: string; value: string; color?: string }>) {
  return (
    <p className="t-caption auto-detail-line" style={{ color }}>
      <span className="t-label">{label}</span> {value}
    </p>
  );
}

function RunRow({ run }: Readonly<{ run: AutomationRun }>) {
  const t = useT();
  const { locale } = useLocale();
  // A run is an audit-style event: read its time in the VIEWER's own timezone
  // (as audit.tsx / aicalls.tsx do), never a fixed one — an operator in any
  // region sees the firing in their local wall-clock.
  const zone = viewerZone();
  return (
    <PanelRow>
      <div className="auto-run-head">
        <OutcomeBadge outcome={run.outcome} />
        <time
          className="t-caption"
          dateTime={run.occurred_at}
          title={run.occurred_at}
        >
          {formatDateTime(run.occurred_at, locale, zone)}
        </time>
        <AutonomyDot tier={run.tier === "auto_execute" ? "auto" : "confirm"} />
        {run.approval_required && (
          <span className="t-caption">{t("auto.runs.needsApproval")}</span>
        )}
      </div>
      {run.trigger_evidence && (
        <DetailLine label={t("auto.runs.why")} value={run.trigger_evidence} />
      )}
      {run.target_ref && (
        <DetailLine label={t("auto.runs.target")} value={run.target_ref} />
      )}
      {run.action_result && (
        <DetailLine label={t("auto.runs.result")} value={run.action_result} />
      )}
      {run.reason && (
        <DetailLine
          label={t("auto.runs.reason")}
          value={run.reason}
          color={reasonColor(run.outcome)}
        />
      )}
    </PanelRow>
  );
}

// The outcome filter chip row: `all` clears the filter, every other chip pins
// one outcome. `all` is a distinct sentinel (not an Outcome) so the query key
// carries `undefined` when unfiltered — changing it resets keyset paging.
const FILTER_OPTIONS = [
  "all",
  "fired",
  "failed",
  "blocked",
  "skipped",
  "queued_for_approval",
] as const;
type FilterOption = (typeof FILTER_OPTIONS)[number];

const FILTER_LABELS: Record<FilterOption, MessageKey> = {
  all: "auto.runs.filterAll",
  fired: "auto.runs.filterFired",
  failed: "auto.runs.filterFailed",
  blocked: "auto.runs.filterBlocked",
  skipped: "auto.runs.filterSkipped",
  queued_for_approval: "auto.runs.filterQueued",
};

// AU-2: the run-history panel over GET /automations/{id}/runs — keyset-paged,
// outcome-filterable, newest-first, every outcome first-class. Mirrors the
// history.tsx useInfiniteQuery + LoadMoreButton + QueryStates shape exactly.
export function AutomationRuns({
  automationId,
}: Readonly<{ automationId: string }>) {
  const t = useT();
  const [outcome, setOutcome] = useState<Outcome | undefined>(undefined);

  const query = useInfiniteQuery({
    // outcome is part of the key: changing the filter starts a fresh first
    // page rather than reusing a cursor minted under a different filter.
    queryKey: ["automation-runs", automationId, outcome ?? ""],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/automations/{id}/runs", {
          params: {
            path: { id: automationId },
            query: {
              limit: 20,
              ...(pageParam ? { cursor: pageParam } : {}),
              ...(outcome ? { outcome } : {}),
            },
          },
        }),
      );
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });

  const runs = query.data?.pages.flatMap((page) => page.data) ?? [];

  let body: ReactNode;
  if (runs.length === 0) {
    // filtered-empty (a narrowing that found nothing) reads differently from
    // never-fired — the operator should know whether the automation is idle
    // or just quiet for this outcome.
    body = (
      <PanelBody>
        <EmptyState>
          {outcome ? t("auto.runs.emptyFiltered") : t("auto.runs.empty")}
        </EmptyState>
      </PanelBody>
    );
  } else {
    body = (
      <>
        {runs.map((run) => (
          <RunRow key={run.id} run={run} />
        ))}
        <PanelBody>
          <LoadMoreButton query={query} />
        </PanelBody>
      </>
    );
  }

  return (
    <Panel className="auto-inspector" title={t("auto.runs.title")}>
      <PanelBody className="auto-run-filters">
        {FILTER_OPTIONS.map((option) => {
          const active =
            option === "all" ? outcome === undefined : outcome === option;
          return (
            <Button
              key={option}
              variant={active ? "primary" : "ghost"}
              aria-pressed={active}
              onClick={() => setOutcome(option === "all" ? undefined : option)}
            >
              {t(FILTER_LABELS[option])}
            </Button>
          );
        })}
      </PanelBody>
      <QueryStates query={query} pendingLabel={t("auto.runs.title")}>
        {body}
      </QueryStates>
    </Panel>
  );
}

type AutomationPreviewResult = components["schemas"]["AutomationPreview"];

// The three offered windows, as the SegmentedControl's string options (the atom
// is keyed on strings). Kept as a typed tuple so the control and its labels can
// never drift apart; each parses to a valid 1..90 the server accepts (the 422
// branch below stays a defensive honesty guard, not a path the UI can reach).
const WINDOWS = ["7", "30", "90"] as const;
type WindowOption = (typeof WINDOWS)[number];

const WINDOW_LABELS: Record<WindowOption, MessageKey> = {
  "7": "auto.preview.window7",
  "30": "auto.preview.window30",
  "90": "auto.preview.window90",
};

// AU-1: the dry-run blast-radius panel over POST /automations/{id}/preview.
// A pure 🟢 read (no writes) — it uses POST only to carry the window in a body;
// see the useQuery note below for why it is modelled as a query, not a mutation.
export function AutomationPreview({
  automationId,
}: Readonly<{ automationId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const [windowDays, setWindowDays] = useState<WindowOption>("30");

  // A preview is a pure read keyed on the selected window — so it is a query,
  // not a mutation. useQuery refetches when the window changes, dedupes React
  // StrictMode's double-mount (one in-flight request per key), starts in the
  // pending state (no empty first frame), and routes loading/error/retry
  // through the shared QueryStates — the same recovery the runs panel gets.
  const query = useQuery({
    queryKey: ["automation-preview", automationId, windowDays],
    queryFn: async (): Promise<AutomationPreviewResult> => {
      return unwrap(
        await api.POST("/automations/{id}/preview", {
          params: { path: { id: automationId } },
          body: { window_days: Number(windowDays) },
        }),
      );
    },
  });

  const result = query.data;
  const hidden = result?.excluded_by_permission ?? 0;

  // SegmentedControl takes resolved label strings (not i18n keys); build them
  // from the one WINDOW_LABELS map so the control and its copy stay in step.
  const windowLabels: Record<WindowOption, string> = {
    "7": t(WINDOW_LABELS["7"]),
    "30": t(WINDOW_LABELS["30"]),
    "90": t(WINDOW_LABELS["90"]),
  };

  return (
    <Panel className="auto-inspector" title={t("auto.preview.title")}>
      <PanelBody className="auto-preview-window">
        <span className="t-label">{t("auto.preview.window")}</span>
        <SegmentedControl
          options={WINDOWS}
          value={windowDays}
          onChange={setWindowDays}
          labels={windowLabels}
          label={t("auto.preview.window")}
        />
      </PanelBody>
      <PanelBody>
        {/* Polite live region: announce when the estimate resolves or the
            window result changes, without stealing focus from the control. */}
        <div aria-live="polite">
          <QueryStates query={query} pendingLabel={t("auto.preview.title")}>
            {result && (
              <div className="auto-preview-figures">
                <p className="t-body">
                  {t("auto.preview.matchesNow", {
                    n: formatNumber(result.matches_now, locale),
                  })}
                </p>
                <p className="t-caption">
                  {result.would_have_fired == null
                    ? t("auto.preview.notComputable")
                    : t("auto.preview.wouldFire", {
                        n: formatNumber(result.would_have_fired, locale),
                        days: formatNumber(result.window_days, locale),
                      })}
                </p>
                {hidden > 0 && (
                  <p className="t-caption">
                    {t("auto.preview.hidden", {
                      n: formatNumber(hidden, locale),
                    })}
                  </p>
                )}
              </div>
            )}
          </QueryStates>
        </div>
        <p className="t-caption auto-preview-explainer">
          {t("auto.preview.explainer")}
        </p>
      </PanelBody>
    </Panel>
  );
}

// The two expandable panels the automation row mounts below its header. Kept as
// one component so the row owns only the open/close toggles, not the panels'
// conditional rendering — each panel still mounts lazily, only while open.
//
// `canConfigure` gates the panels themselves, not only the toggle buttons: if a
// live `/me` refresh drops the caller's admin/ops role while a panel is open,
// the read surface (and its polling) disappears with the buttons. The server's
// RBAC gate is still the real boundary — this keeps the UI honest about it.
export function AutomationInspectors({
  automationId,
  runsOpen,
  previewOpen,
  canConfigure,
}: Readonly<{
  automationId: string;
  runsOpen: boolean;
  previewOpen: boolean;
  canConfigure: boolean;
}>) {
  if (!canConfigure) {
    return null;
  }
  return (
    <>
      {runsOpen && <AutomationRuns automationId={automationId} />}
      {previewOpen && <AutomationPreview automationId={automationId} />}
    </>
  );
}
