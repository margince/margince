// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { type ReactNode, useEffect, useId, useRef, useState } from "react";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import { Field, SegmentedControl } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { LoadMoreButton, QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
// The shapes this file draws. Imported here rather than left to the caller:
// the row that folds these open lives in another sheet, and a story or a test
// that mounts one panel alone would otherwise render it unstyled.
import "./automationdetail.css";
import { labelFor } from "../../lib/labelfor";
import { useAutomationPreview } from "./automations.previewquery";
import { RenewalPreviewDetails } from "./automations.renewalpreview";
import { RunRow } from "./automations.runrow";

// The human surface for the two human-only automation ops
// (listAutomationRuns / previewAutomation), in its own file so automations.tsx
// stays legible. Both panels are pure reads; neither writes.

type AutomationRun = components["schemas"]["AutomationRun"];
type Outcome = AutomationRun["outcome"];

// The outcome dial: "" clears the filter, every other option pins one outcome,
// so the query key carries `undefined` when unfiltered and paging restarts.
const ALL_OUTCOMES = "";
const FILTER_OPTIONS = [
  "fired",
  "failed",
  "blocked",
  "skipped",
  "queued_for_approval",
] as const satisfies readonly Outcome[];

const FILTER_LABELS: Record<Outcome, MessageKey> = {
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
  titled,
  focusKey,
}: Readonly<{ automationId: string; titled?: boolean; focusKey?: number }>) {
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
  // Drawn while the first page loads, so it does not pop in above the runs. A
  // chosen filter stays, so a narrowing that found nothing can be undone.
  const filterable =
    outcome !== undefined ||
    (!query.isError && (query.isPending || runs.length > 0));

  let body: ReactNode;
  if (runs.length === 0) {
    // filtered-empty (a narrowing that found nothing) reads differently from
    // never-fired — the operator should know whether the automation is idle
    // or just quiet for this outcome.
    body = (
      <p>{outcome ? t("auto.runs.emptyFiltered") : t("auto.runs.empty")}</p>
    );
  } else {
    body = (
      <>
        <ul className="auto-run-list">
          {runs.map((run) => (
            <RunRow key={run.id} run={run} />
          ))}
        </ul>
        <LoadMoreButton query={query} />
      </>
    );
  }

  return (
    <Inspector
      title={t("auto.runs.title")}
      titled={titled ?? false}
      focusKey={focusKey}
    >
      {filterable && (
        <Field label={t("auto.runs.outcome")} className="auto-run-filter">
          {(field) => (
            <Select
              {...field}
              value={outcome ?? ALL_OUTCOMES}
              onChange={(next) =>
                setOutcome(FILTER_OPTIONS.find((one) => one === next))
              }
              options={[
                { value: ALL_OUTCOMES, label: t("auto.runs.filterAll") },
                ...FILTER_OPTIONS.map((option) => ({
                  value: option,
                  label: t(labelFor(FILTER_LABELS, option)),
                })),
              ]}
            />
          )}
        </Field>
      )}
      <QueryStates query={query} pendingLabel={t("auto.runs.title")}>
        {body}
      </QueryStates>
    </Inspector>
  );
}

// One inspector in an opened row. Its heading shows only beside a second one;
// alone, the row's own chevron already names it.
function Inspector({
  title,
  titled,
  focusKey,
  children,
}: Readonly<{
  title: string;
  titled: boolean;
  /** Changes each time a menu choice opens this panel, which then takes focus. */
  focusKey?: number;
  children: ReactNode;
}>) {
  const headingId = useId();
  const section = useRef<HTMLElement | null>(null);
  // Passive, so it lands after the menu that opened this puts focus back on
  // its own trigger.
  useEffect(() => {
    if (focusKey !== undefined) section.current?.focus();
  }, [focusKey]);
  return (
    <section
      ref={section}
      tabIndex={-1}
      className="auto-inspector"
      aria-labelledby={titled ? headingId : undefined}
      aria-label={titled ? undefined : title}
    >
      {titled && (
        <Heading size="xsmall" as="h3" id={headingId}>
          {title}
        </Heading>
      )}
      {children}
    </section>
  );
}

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

export function AutomationPreview({
  automationId,
  titled,
  focusKey,
}: Readonly<{ automationId: string; titled?: boolean; focusKey?: number }>) {
  const t = useT();
  const { locale } = useLocale();
  const [windowDays, setWindowDays] = useState<WindowOption>("30");

  const query = useAutomationPreview({
    id: automationId,
    windowDays: Number(windowDays),
  });

  const result = query.data;
  const hidden = result?.excluded_by_permission ?? 0;
  const eventPreview = result?.preview_kind === "event";

  // SegmentedControl takes resolved label strings (not i18n keys); build them
  // from the one WINDOW_LABELS map so the control and its copy stay in step.
  const windowLabels: Record<WindowOption, string> = {
    "7": t(WINDOW_LABELS["7"]),
    "30": t(WINDOW_LABELS["30"]),
    "90": t(WINDOW_LABELS["90"]),
  };

  return (
    <Inspector
      title={t("auto.preview.title")}
      titled={titled ?? false}
      focusKey={focusKey}
    >
      {!eventPreview && result?.historical_preview_available !== false && (
        <div className="auto-preview-window">
          <span className="t-label">{t("auto.preview.window")}</span>
          <SegmentedControl
            options={WINDOWS}
            value={windowDays}
            onChange={setWindowDays}
            labels={windowLabels}
            label={t("auto.preview.window")}
          />
        </div>
      )}
      {/* Polite live region: announce when the estimate resolves or the
          window result changes, without stealing focus from the control. */}
      <div aria-live="polite">
        <QueryStates query={query} pendingLabel={t("auto.preview.title")}>
          {result &&
            (eventPreview ? (
              <div className="auto-preview-figures">
                <p className="t-body">{t("auto.preview.fromNowOn")}</p>
                <p className="t-caption">
                  {t("auto.preview.historyUnavailable")}
                </p>
              </div>
            ) : (
              <div className="auto-preview-figures">
                <p className="t-body">
                  {result.matches_now_available !== false &&
                    t("auto.preview.matchesNow", {
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
                {result.renewal && (
                  <RenewalPreviewDetails renewal={result.renewal} />
                )}
                {hidden > 0 && (
                  <p className="t-caption">
                    {t("auto.preview.hidden", {
                      n: formatNumber(hidden, locale),
                    })}
                  </p>
                )}
              </div>
            ))}
        </QueryStates>
      </div>
      <p className="t-caption">
        {t(
          eventPreview
            ? "auto.preview.futureEventsOnly"
            : "auto.preview.explainer",
        )}
      </p>
    </Inspector>
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
  focus,
}: Readonly<{
  automationId: string;
  runsOpen: boolean;
  previewOpen: boolean;
  canConfigure: boolean;
  /** The panel a menu choice just opened, which takes focus. */
  focus?: Readonly<{ panel: "runs" | "preview"; seq: number }>;
}>) {
  if (!canConfigure) {
    return null;
  }
  const focusKey = (panel: "runs" | "preview") =>
    focus?.panel === panel ? focus.seq : undefined;
  return (
    <>
      {runsOpen && (
        <AutomationRuns
          automationId={automationId}
          titled={previewOpen}
          focusKey={focusKey("runs")}
        />
      )}
      {previewOpen && (
        <AutomationPreview
          automationId={automationId}
          titled={runsOpen}
          focusKey={focusKey("preview")}
        />
      )}
    </>
  );
}
