// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { KeyedName } from "../design-system/keyedname";
import { Popover } from "../design-system/popover";
import { RowOpen } from "../design-system/rowopen";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { TierCallsLine } from "./ai-call-figures";
import {
  callCodeName,
  DECIDE_RUNG,
  gaveUpLabel,
  tierLabel,
} from "./ai-decision-labels";
import type { ModelCatalogue, ModelLane } from "./ai-models";
import { providerName } from "./ai-provider-names";
import { laneGloss, priceLabel, servingSort } from "./ai-routing-lane-text";
import { TermChip } from "./ai-terms";

// The Model tiers table: one row per lane the routing document binds — the
// tiers, the embedder and the optional decision model — joined to the health
// rung that lane's calls are counted under. Apart from the card that holds it,
// because a row is a reading of one binding and knows nothing of the document
// around it.

type Rung = components["schemas"]["AiRungHealth"];
type Health = components["schemas"]["AiHealth"];
type Feature = components["schemas"]["AiFeatureRoute"];
type T = ReturnType<typeof useT>;

const DECISIONS = "decisions";

// The rung each off-ladder lane stamps on its calls (embedlane.go, decidetrace.go).
const RUNG_OF: Readonly<Record<string, string>> = {
  embeddings: "embed",
  decisions: DECIDE_RUNG,
};

function rungOf(laneName: string): string {
  return RUNG_OF[laneName] ?? laneName;
}

// The lane behind a rung nothing on the page binds, named as its lane would
// be: a health-only reader meets `embed` as "embeddings", the word the routing
// document and every gloss use.
function laneNameOf(rung: string): string {
  return (
    Object.keys(RUNG_OF).find((laneName) => RUNG_OF[laneName] === rung) ?? rung
  );
}

export type Lane = Readonly<{
  name: string;
  lane: ModelLane;
  binding?: {
    provider: string;
    model: string;
    base_url?: string;
    routing?: components["schemas"]["AiOpenRouterRouting"];
  };
  // What Edit opens; absent for a lane this reader may see and not bind.
  onEdit?: () => void;
  testId?: string;
}>;

type Row = Readonly<{
  lane: Lane;
  rung: Rung | undefined;
  tasks: number | undefined;
  // The decision lane a document leaves out, offered as a row to add it.
  absent?: boolean;
}>;

/**
 * Health and tasks are reads with their own grants; each half stays silent when
 * its read is not this reader's: an absent dot is not "healthy", and an absent
 * count is not "unused". A row with no binding is a rung that still receives
 * calls after its lane left the document, or one whose bindings this reader
 * may not see.
 */
export function TiersTable({
  lanes,
  health,
  features,
  catalogue,
  canManage,
  onAddDecisions,
}: Readonly<{
  lanes: readonly Lane[];
  health: Health | undefined;
  features: readonly Feature[] | undefined;
  catalogue: ModelCatalogue;
  canManage: boolean;
  // Only when the document leaves the decision model out.
  onAddDecisions?: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const rows = tierRows(lanes, health, features, onAddDecisions);
  const opener = (row: Row) =>
    row.absent ? (canManage ? onAddDecisions : undefined) : row.lane.onEdit;
  const opens = rows.some((row) => opener(row) !== undefined);
  return (
    <DataTable<Row>
      bleed
      fold
      label={t("aiRouting.title")}
      rows={rows}
      rowKey={(row) => row.lane.name}
      rowTestId={(row) => row.lane.testId ?? `ai-routing-tier-${row.lane.name}`}
      onRowClick={opens ? (row) => opener(row)?.() : undefined}
      rowOpens={(row) => opener(row) !== undefined}
      columns={[
        {
          key: "tier",
          header: t("aiTerms.tier"),
          render: (row) => <TierCell row={row} health={health} />,
        },
        {
          key: "model",
          header: t("aiRouting.model.label"),
          grow: true,
          render: (row) =>
            row.absent ? (
              <span className="t-sub">{t("aiRouting.decisions.absent")}</span>
            ) : (
              <BindingCell row={row} catalogue={catalogue} />
            ),
        },
        {
          key: "tasks",
          header: t("aiRouting.colTasks"),
          align: "end",
          render: (row) =>
            row.tasks === undefined ? null : (
              <TermChip term="task">
                {plural("aiRouting.taskCount", row.tasks, {
                  count: formatNumber(row.tasks, locale),
                })}
              </TermChip>
            ),
        },
        {
          key: "open",
          header: t("table.actions"),
          headerHidden: true,
          align: "end",
          fold: "end",
          render: (row) => (
            <OpenVerb row={row} canManage={canManage} onAdd={onAddDecisions} />
          ),
        },
      ]}
    />
  );
}

function tierRows(
  lanes: readonly Lane[],
  health: Health | undefined,
  features: readonly Feature[] | undefined,
  onAddDecisions: (() => void) | undefined,
): Row[] {
  const known = new Set(lanes.map((l) => rungOf(l.name)));
  return [
    ...lanes.map((lane) => ({
      lane,
      rung: health?.rungs.find((r) => r.tier === rungOf(lane.name)),
      // A decision-first task is counted under decisions AND under its leading
      // tier: it starts on the decision model and falls through to that tier,
      // so both lanes carry its calls and each row answers "what runs here".
      tasks: features?.filter((f) =>
        lane.name === DECISIONS
          ? f.decision_first
          : f.leading_tier === lane.name,
      ).length,
    })),
    ...(health?.rungs ?? [])
      .filter((r) => !known.has(r.tier))
      .map((rung) => ({
        lane: { name: laneNameOf(rung.tier), lane: "chat" as const },
        rung,
        tasks: undefined,
      })),
    ...(onAddDecisions
      ? [
          {
            lane: {
              name: DECISIONS,
              lane: "decisions" as const,
              testId: "ai-routing-decisions",
            },
            rung: undefined,
            tasks: undefined,
            absent: true,
          },
        ]
      : []),
  ];
}

function TierCell({
  row,
  health,
}: Readonly<{ row: Row; health: Health | undefined }>) {
  const t = useT();
  const { lane, rung, absent } = row;
  const gloss = laneGloss(lane.name, t);
  return (
    <span className="ai-tier-who">
      {health && !absent && (
        <HealthDot
          lane={lane.name}
          rung={rung}
          health={health}
          sort={servingSort(lane.binding, t)}
        />
      )}
      <CellStack>
        <KeyedName name={tierLabel(lane.name, t)} code={lane.name} />
        {gloss && <span className="t-caption ai-tier-gloss">{gloss}</span>}
      </CellStack>
    </span>
  );
}

function BindingCell({
  row,
  catalogue,
}: Readonly<{ row: Row; catalogue: ModelCatalogue }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { lane, rung } = row;
  const { binding } = lane;
  if (!binding) {
    return failing(rung) ? (
      <ErrorLine inline>{failure(rung, t, plural, locale)}</ErrorLine>
    ) : (
      <Badge>{t("aiRouting.notBound")}</Badge>
    );
  }
  // A bound model the sheet cannot price says so, in the price's own place: a
  // blank line reads as a price of nothing.
  const price =
    priceLabel(
      catalogue,
      binding.provider,
      binding.model,
      lane.lane,
      locale,
      t,
    ) || t("aiRouting.noPrice");
  return (
    <span className="ai-tier-binding">
      <span className="ai-tier-modelline">
        <TermChip term="provider">{providerName(binding.provider, t)}</TermChip>
        <span className="ai-tier-model">{binding.model}</span>
      </span>
      {failing(rung) ? (
        <ErrorLine inline>{failure(rung, t, plural, locale)}</ErrorLine>
      ) : (
        <span className="t-caption">{price}</span>
      )}
    </span>
  );
}

function failing(rung: Rung | undefined): rung is Rung {
  return rung !== undefined && !rung.healthy;
}

function failure(
  rung: Rung,
  t: T,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): string {
  return [
    plural("aiHealth.callCounts", rung.calls, {
      count: formatNumber(rung.calls, locale),
      failures: formatNumber(rung.failures, locale),
    }),
    rung.last_sentinel ? gaveUpLabel(rung.last_sentinel, t) : "",
  ]
    .filter(Boolean)
    .join(" · ");
}

function OpenVerb({
  row,
  canManage,
  onAdd,
}: Readonly<{ row: Row; canManage: boolean; onAdd?: () => void }>) {
  const t = useT();
  if (row.absent) {
    return (
      <RowOpen
        label={t("aiRouting.decisions.add")}
        refusal={canManage ? undefined : t("aiRouting.adminOnly")}
        onOpen={onAdd}
      />
    );
  }
  if (!row.lane.onEdit) return null;
  return (
    <RowOpen
      label={t("aiRouting.editNamed", { name: tierLabel(row.lane.name, t) })}
      onOpen={row.lane.onEdit}
    />
  );
}

// One dot per lane: green when it answered, red when it did not, grey when it
// took no calls in the window. It opens a popover with the numbers behind it,
// the same way the decision table explains its fallback rate. Its name leads
// with the lane, so seven dots read as seven different controls.
function HealthDot({
  lane,
  rung,
  health,
  sort,
}: Readonly<{
  lane: string;
  rung: Rung | undefined;
  health: Health;
  sort?: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = viewerZone();
  const state = !rung ? "idle" : rung.healthy ? "ok" : "bad";
  const reading = !rung
    ? t("aiHealth.noCalls", {
        hours: formatNumber(health.window_hours, locale),
      })
    : rung.healthy
      ? t("aiHealth.answering")
      : t("aiHealth.notAnswering");
  return (
    <Popover
      label={
        <>
          <span
            className={`ai-health-dot ai-health-dot-${state}`}
            aria-hidden
          />
          <span className="sr-only">{`${tierLabel(lane, t)}: ${reading}`}</span>
        </>
      }
    >
      <p>{reading}</p>
      {rung && (
        <>
          <p>
            {plural("aiHealth.callCounts", rung.calls, {
              count: formatNumber(rung.calls, locale),
              failures: formatNumber(rung.failures, locale),
            })}
          </p>
          <p>
            {t("aiRouting.median", {
              ms: formatNumber(rung.median_latency_ms, locale),
            })}
          </p>
          {rung.last_call_at && (
            <p>
              {t("aiRouting.lastResponse", {
                when: formatDateTime(rung.last_call_at, locale, zone),
              })}
            </p>
          )}
          {rung.last_sentinel && (
            <KeyedName
              name={callCodeName(rung.last_sentinel, t)}
              code={rung.last_sentinel}
            />
          )}
        </>
      )}
      <p>
        <TierCallsLine tier={rungOf(lane)} sort={sort} />
      </p>
    </Popover>
  );
}
