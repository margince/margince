// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { BusyMark, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { type Fact, FactList } from "../design-system/factlist";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import {
  formatFinePercent,
  formatNumber,
  formatPercent,
} from "../format/format";
import type { Locale, Translator } from "../i18n";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, unwrap } from "./common";
import { criterionKindLabel } from "./criterionkinds";
import { usePipelines } from "./pipelines.queries";
import { StageRulesCard } from "./settings.stagerules";
import "./settings.stageautomation.css";

type TransitionRecord = components["schemas"]["StageTransitionRecord"];

// The window the report defaults to. Named here as well as on the server
// because the reader needs to be told what "last 30 days" means before they
// read a rate; the server stays the authority and answers the number it
// actually used, which is what the heading shows.
const DEFAULT_WINDOW_DAYS = 30;

/**
 * What each stage transition has earned, and what each is allowed to do.
 *
 * THE TABLE IS READ-ONLY, and that is the whole design. It exists so a contact
 * can decide whether a transition is trustworthy enough to move deals by
 * itself, and a control in every row would invite the flip before the reading.
 * The switches live BELOW it, in their own section, reached after the evidence
 * rather than beside it.
 *
 * Every rate is a share of ANSWERED proposals, and the answered count is drawn
 * beside them rather than under a tooltip: a transition with a perfect
 * acceptance rate over four cards has told you nothing, and a reader who cannot
 * see the four will not know that.
 */
export function StageAutomationCard() {
  const t = useT();
  const { locale } = useLocale();
  const [pipelineId, setPipelineId] = useState<string>("");
  // The SAME read the pipelines card makes, so opening this page after that
  // one costs no request and the two cannot disagree about what exists.
  const pipelines = usePipelines();

  const chosen = pipelineId || pipelines.data?.[0]?.id || "";
  const report = useQuery({
    queryKey: ["stage-automation", "report", chosen],
    // Not asked until a pipeline is known: without the id the request is a 422
    // the reader never asked for.
    enabled: chosen !== "",
    queryFn: async () => {
      return unwrap(
        await api.GET("/stage-automation/report", {
          params: { query: { pipeline_id: chosen } },
        }),
      );
    },
  });

  // Three states, told apart. An empty table means "nothing has been proposed
  // yet" — a fact a reader will act on by waiting. A load that is still running
  // or that FAILED means nothing of the kind, and drawing the same words for
  // all three tells somebody to wait for numbers that were refused or never
  // asked for.
  //
  // Gathered into one helper because they are one question asked three ways —
  // is there a report to draw — and inlining them put this component over the
  // complexity ceiling once the rules card joined it.
  const notYet = beforeTheReport({ pipelines, report, chosen }, t);
  if (notYet) {
    return notYet;
  }

  const rows = report.data?.data ?? [];
  // The window the counts actually came over, named once. The rules card needs
  // it to know whether a rule's own window matches — a rule measured over a
  // different span cannot be judged from these rows.
  const windowDays = report.data?.window_days ?? DEFAULT_WINDOW_DAYS;
  return (
    <>
      <Panel title={t("stageAutomation.title")}>
        <PanelBody>
          <PanelIntro>{t("stageAutomation.intro")}</PanelIntro>
          {(pipelines.data?.length ?? 0) > 1 && (
            <Select
              aria-label={t("stageAutomation.pipeline")}
              value={chosen}
              onChange={setPipelineId}
              options={(pipelines.data ?? []).map((pipeline) => ({
                value: pipeline.id,
                label: pipeline.name,
              }))}
            />
          )}
          {rows.length === 0 && (
            <EmptyState>{t("stageAutomation.empty")}</EmptyState>
          )}
        </PanelBody>
        {rows.length > 0 && (
          <>
            <DataTable
              bleed
              stickyFirst
              label={t("stageAutomation.title")}
              columns={reportColumns(t, locale)}
              rows={rows}
              rowKey={(row) => `${row.from_stage_id}-${row.to_stage_id}`}
              detail={{
                header: t("stageAutomation.detail"),
                toggleLabel: (row) =>
                  t("stageAutomation.detailFor", {
                    transition: transitionName(row),
                  }),
                render: (row) => <TransitionDetail row={row} />,
              }}
            />
            {/* The short headings spelled out in view: a number you must hover
                to understand is one contacts read wrong once and stop reading. */}
            <PanelBody>
              <FactList facts={glossary(t)} />
            </PanelBody>
          </>
        )}
      </Panel>
      {/* The controls, after the evidence. Given the report's own rows
          rather than fetching a second list of transitions: the two
          would otherwise be able to disagree about which transitions
          this pipeline has. */}
      {rows.length > 0 && (
        <StageRulesCard
          pipelineId={chosen}
          transitions={rows}
          reportWindowDays={windowDays}
        />
      )}
    </>
  );
}

const GLOSSARY: readonly (readonly [string, MessageKey, MessageKey])[] = [
  ["reviewed", "stageAutomation.reviewed", "stageAutomation.reviewedHint"],
  ["clean", "stageAutomation.cleanAcceptance", "stageAutomation.acceptedHint"],
  ["edits", "stageAutomation.edits", "stageAutomation.editedHint"],
  ["unsafe", "stageAutomation.unsafe", "stageAutomation.unsafeHint"],
  ["expired", "stageAutomation.expired", "stageAutomation.expiredHint"],
  [
    "observed",
    "stageAutomation.observationDays",
    "stageAutomation.observationHint",
  ],
];

function glossary(t: Translator): Fact[] {
  return GLOSSARY.map(([key, term, hint]) => ({
    key,
    term: t(term),
    value: t(hint),
  }));
}

// The counts beside the rates, and the evidence cut, behind the row's chevron.
function TransitionDetail({ row }: Readonly<{ row: TransitionRecord }>) {
  const t = useT();
  const { locale } = useLocale();
  const evidence: Fact[] =
    row.evidence_kinds.length === 0
      ? [
          {
            key: "evidence",
            term: t("stageAutomation.evidenceKinds"),
            value: "—",
          },
        ]
      : row.evidence_kinds.map((kind) => ({
          key: `evidence-${kind.kind}`,
          term: criterionKindLabel(kind.kind, t),
          value: t("stageAutomation.evidenceShare", {
            accepted: formatNumber(kind.accepted_clean, locale),
            reviewed: formatNumber(kind.reviewed, locale),
          }),
        }));
  return (
    <FactList
      numeric
      facts={[
        {
          key: "open",
          term: t("stageAutomation.open"),
          value: formatNumber(row.proposed, locale),
        },
        {
          key: "expired",
          term: t("stageAutomation.expired"),
          value: formatNumber(row.expired, locale),
        },
        {
          key: "observed",
          term: t("stageAutomation.observationDays"),
          value: formatNumber(row.observation_days, locale),
        },
        ...evidence,
      ]}
    />
  );
}

function transitionName(row: TransitionRecord): string {
  return `${row.from_stage_name} → ${row.to_stage_name}`;
}

/** The panel to draw when there is no report yet, or null when there is one. */
function beforeTheReport(
  state: {
    pipelines: {
      isError: boolean;
      isPending: boolean;
      error: unknown;
      data?: unknown[];
    };
    report: { isError: boolean; isPending: boolean; error: unknown };
    chosen: string;
  },
  t: Translator,
): ReactElement | null {
  const { pipelines, report, chosen } = state;
  if (pipelines.isError || report.isError) {
    return (
      <Panel title={t("stageAutomation.title")}>
        <PanelBody>
          <Callout
            tone="danger"
            kind="outcome"
            title={t("stageAutomation.unreadable")}
          >
            {problemMessageOf(pipelines.error ?? report.error, t)}
          </Callout>
        </PanelBody>
      </Panel>
    );
  }
  if (pipelines.isPending || (chosen !== "" && report.isPending)) {
    return (
      <Panel title={t("stageAutomation.title")}>
        <PanelBody>
          <BusyMark />
        </PanelBody>
      </Panel>
    );
  }
  if (pipelines.data && pipelines.data.length === 0) {
    return (
      <Panel title={t("stageAutomation.title")}>
        <PanelBody>
          <EmptyState>{t("stageAutomation.noPipelines")}</EmptyState>
        </PanelBody>
      </Panel>
    );
  }
  return null;
}

// A rate, or a dash.
//
// A transition nobody has answered has NO rate — not a rate of zero. Printing
// "0%" for it would say contacts reject it every time, when what happened is that
// nobody looked, and those two ask for opposite fixes.
function rateCell(rate: number, reviewed: number, locale: Locale): string {
  if (reviewed === 0) {
    return "—";
  }
  // Through the locale, because a German reader writes it "42 %" with a space
  // and a hand-built template would print the English spelling to everyone.
  return formatPercent(rate, locale);
}

// The SAFETY number, which gets a decimal place the others do not.
//
// formatPercent rounds to whole numbers, and the threshold this rate is
// eventually held to is one percent — so a transition sitting at 0.4% undone
// would print "0%" and read as flawless. The rate that STOPS automation is the
// one place a rounding that flatters is worst, so it keeps the digit that
// distinguishes "none" from "few".
//
// Zero still prints as "0%", not "0.0%": nothing went wrong is a whole fact.
function unsafeCell(rate: number, reviewed: number, locale: Locale): string {
  if (reviewed === 0) {
    return "—";
  }
  return formatFinePercent(rate, locale);
}

// Only the volume and the rates stand in columns, so the table fits its pane at desktop width.
function reportColumns(
  t: Translator,
  locale: Locale,
): DataTableColumn<TransitionRecord>[] {
  const figure = (
    key: string,
    header: string,
    render: (row: TransitionRecord) => string,
  ): DataTableColumn<TransitionRecord> => ({
    key,
    header,
    align: "end",
    render,
  });
  return [
    {
      key: "transition",
      header: t("stageAutomation.transition"),
      render: (row) => (
        <CellStack>
          <span className="stageauto-transition">{transitionName(row)}</span>
          {unanswered(row) && (
            <span className="t-caption">
              {t("stageAutomation.nothingReviewed")}
            </span>
          )}
        </CellStack>
      ),
    },
    figure("reviewed", t("stageAutomation.reviewed"), (row) =>
      unanswered(row) ? "—" : formatNumber(row.reviewed, locale),
    ),
    figure("clean", t("stageAutomation.cleanAcceptance"), (row) =>
      rateCell(row.clean_acceptance_rate, row.reviewed, locale),
    ),
    figure("edits", t("stageAutomation.edits"), (row) =>
      rateCell(row.edit_rate, row.reviewed, locale),
    ),
    figure("rejections", t("stageAutomation.rejections"), (row) =>
      rateCell(row.rejection_rate, row.reviewed, locale),
    ),
    figure("unsafe", t("stageAutomation.unsafe"), (row) =>
      unsafeCell(row.unsafe_rate, row.reviewed, locale),
    ),
  ];
}

function unanswered(row: TransitionRecord): boolean {
  return row.reviewed === 0 && row.proposed > 0;
}
