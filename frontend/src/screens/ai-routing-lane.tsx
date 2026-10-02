// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { PanelRow } from "../design-system/panel";
import { Popover } from "../design-system/popover";
import {
  formatDateTime,
  formatNumber,
  formatUsdPerMTok,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { TierCallsLine } from "./ai-call-figures";
import { DECIDE_RUNG } from "./ai-decision-labels";
import {
  inputOnlyLane,
  type ModelCatalogue,
  type ModelLane,
  unreadablePrice,
} from "./ai-models";
import { isOpenRouter } from "./ai-provider-links";
import { TermChip } from "./ai-terms";

// The Model tiers table: one row per lane the routing document binds — the
// tiers, the embedder and the optional decision model — joined to the health
// rung that lane's calls are counted under. Apart from the card that holds it,
// because a row is a reading of one binding and knows nothing of the document
// around it.

type Rung = components["schemas"]["AiRungHealth"];
type Health = components["schemas"]["AiHealth"];
type Feature = components["schemas"]["AiFeatureRoute"];

// The decision lane's name, as the routing document spells its key. Shown raw,
// like every tier name down the same column.
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
}>;

/**
 * The lanes as a list, each one a binding and — where the reader may see how
 * calls went — a health dot that explains itself on a tap.
 *
 * Two jobs on two levels: the line says which model runs the lane (the tier and
 * the model in full size, price and host under them, small), and the dot says
 * whether it answers, with the numbers behind it in a popover. A lane that is
 * not answering says why on its own line, because the sentinel is what an
 * operator acts on.
 *
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
  unkeyed,
  canManage,
  onAddDecisions,
}: Readonly<{
  lanes: readonly Lane[];
  health: Health | undefined;
  features: readonly Feature[] | undefined;
  catalogue: ModelCatalogue;
  unkeyed: ReadonlySet<string> | null;
  canManage: boolean;
  // Only when the document leaves the decision model out.
  onAddDecisions?: () => void;
}>) {
  const t = useT();
  const known = new Set(lanes.map((l) => rungOf(l.name)));
  const rows: Row[] = [
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
  ];
  return (
    <>
      {rows.map((row) => (
        <TierLine
          key={row.lane.name}
          row={row}
          health={health}
          catalogue={catalogue}
          unkeyed={unkeyed}
        />
      ))}
      {onAddDecisions && (
        <PanelRow>
          <div
            data-testid="ai-routing-decisions"
            className={tierLineClass(health !== undefined)}
          >
            {health && <span />}
            <span className="ai-tier-name">{DECISIONS}</span>
            <span className="t-sub">{t("aiRouting.decisions.absent")}</span>
            <Button
              onClick={onAddDecisions}
              reason={canManage ? undefined : t("aiRouting.adminOnly")}
            >
              {t("aiRouting.decisions.add")}
            </Button>
          </div>
        </PanelRow>
      )}
    </>
  );
}

function tierLineClass(withDot: boolean): string {
  return withDot ? "ai-tier-line" : "ai-tier-line ai-tier-line-plain";
}

function TierLine({
  row,
  health,
  catalogue,
  unkeyed,
}: Readonly<{
  row: Row;
  health: Health | undefined;
  catalogue: ModelCatalogue;
  unkeyed: ReadonlySet<string> | null;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { lane, rung, tasks } = row;
  const { binding } = lane;
  const failing = rung !== undefined && !rung.healthy;
  // A bound model the sheet cannot price says so, in the price's own place: the
  // second line is always there, and a blank one reads as a price of nothing.
  const price = binding
    ? priceLabel(
        catalogue,
        binding.provider,
        binding.model,
        lane.lane,
        locale,
        t,
      ) || t("aiRouting.noPrice")
    : "";
  // The small line: what the binding costs and where it points — or, when the
  // lane is failing, what it said, which matters more than either.
  const secondary = failing
    ? [
        rung.last_sentinel,
        plural("aiHealth.callCounts", rung.calls, {
          count: formatNumber(rung.calls, locale),
          failures: formatNumber(rung.failures, locale),
        }),
      ]
    : [price];
  const gloss = laneGloss(lane.name, t);
  return (
    <PanelRow>
      <div
        data-testid={lane.testId ?? `ai-routing-tier-${lane.name}`}
        className={tierLineClass(health !== undefined)}
      >
        {health && (
          <HealthDot
            lane={lane.name}
            rung={rung}
            health={health}
            sort={servingSort(binding, t)}
          />
        )}
        <span className="ai-tier-who">
          <span className="ai-tier-name">{lane.name}</span>
          {gloss && <span className="t-caption">{gloss}</span>}
        </span>
        <span className="ai-tier-binding">
          <BindingLine binding={binding} unkeyed={unkeyed} />
          {failing ? (
            <ErrorLine inline>
              {secondary.filter(Boolean).join(" · ")}
            </ErrorLine>
          ) : (
            <span
              className="t-caption ai-tier-secondary"
              title={secondary.filter(Boolean).join(" · ")}
            >
              {secondary.filter(Boolean).join(" · ")}
            </span>
          )}
        </span>
        <span className="ai-tier-actions">
          {tasks !== undefined && (
            <TermChip term="task">
              {plural("aiRouting.taskCount", tasks, {
                count: formatNumber(tasks, locale),
              })}
            </TermChip>
          )}
          {lane.onEdit && (
            <Button onClick={lane.onEdit}>{t("aiRouting.edit")}</Button>
          )}
        </span>
      </div>
    </PanelRow>
  );
}

// Which model the lane runs on: the provider's mark and the model id on one
// line, with a warning beside the id when the provider holds no credential.
function BindingLine({
  binding,
  unkeyed,
}: Readonly<{
  binding: Lane["binding"];
  unkeyed: ReadonlySet<string> | null;
}>) {
  const t = useT();
  if (!binding) {
    return <Badge>{t("aiRouting.notBound")}</Badge>;
  }
  return (
    <span className="ai-tier-modelline">
      <TermChip term="provider">{binding.provider}</TermChip>
      <span className="ai-tier-model" title={binding.model}>
        {binding.model}
      </span>
      {unkeyed?.has(binding.provider) && (
        <Badge tone="warning">{t("aiRouting.noKey")}</Badge>
      )}
    </span>
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
          <span className="sr-only">{`${lane}: ${reading}`}</span>
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
          {rung.last_sentinel && <p>{rung.last_sentinel}</p>}
        </>
      )}
      <p>
        <TierCallsLine tier={rungOf(lane)} sort={sort} />
      </p>
    </Popover>
  );
}

// What each lane in the ladder is FOR, in words rather than in its id.
//
// An explicit switch rather than a key built from the tier name: the message
// catalog's type is a closed union, and a runtime-composed key would compile as
// any old string and ship a typo. It also means a tier the task contract grows
// later renders with no gloss — correct, because nobody has written one, and a
// missing sentence is better than a guessed one.
/**
 * How a broker tier orders its hosts: its own sort, the shipped default's when
 * it declares no routing, or the broker's own when it opted out. Nothing for a
 * binding no broker serves.
 */
function servingSort(
  binding: Lane["binding"],
  t: ReturnType<typeof useT>,
): string | undefined {
  if (
    binding?.provider !== "openai_compatible" ||
    !isOpenRouter(binding.base_url ?? "")
  ) {
    return undefined;
  }
  const routing = binding.routing;
  if (!routing) return "throughput";
  const sort = routing.provider?.sort;
  if (!sort) return t("aiFigures.line.brokerOwn");
  if (typeof sort === "string") return sort;
  return sort.partition === "none"
    ? t("aiFigures.line.sortAcross", { by: sort.by })
    : sort.by;
}

function laneGloss(name: string, t: ReturnType<typeof useT>): string | null {
  switch (name) {
    case "local_small":
      return t("aiRouting.lane.local_small");
    case "cheap_cloud":
      return t("aiRouting.lane.cheap_cloud");
    case "premium":
      return t("aiRouting.lane.premium");
    case "frontier":
      return t("aiRouting.lane.frontier");
    case "local_large":
      return t("aiRouting.lane.local_large");
    case "embeddings":
      return t("aiRouting.lane.embeddings");
    case "decisions":
      return t("aiRouting.lane.decisions");
    default:
      return null;
  }
}

// This binding's price, short enough to sit on the row: what goes in, what comes
// out, per million tokens. Empty where the sheet cannot say: printing a zero
// here would be the one thing this product is careful never to say by accident,
// and the editor's rate plate is where a missing price is spelled out.
function priceLabel(
  catalogue: ModelCatalogue,
  provider: string,
  model: string,
  lane: ModelLane,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  const rate = (catalogue ?? []).find(
    (r) => r.provider === provider && r.model_id === model && r.lane === lane,
  );
  if (!rate) {
    return "";
  }
  // A row the sheet cannot state a price for prints NOTHING rather than
  // reaching the formatter. `formatUsdPerMTok` hands the parsed number to
  // `Intl.NumberFormat`'s `minimumFractionDigits`, and NaN there throws a
  // RangeError — during render, on a card the whole settings page is composed
  // from. The picker's own hint guards the same way for the same reason.
  //
  // The output side is only asked about where it MEANS something: an
  // input-only lane's price is a single figure.
  if (unreadablePrice(rate.input_per_mtok)) {
    return "";
  }
  const input = formatUsdPerMTok(rate.input_per_mtok, locale);
  if (inputOnlyLane(lane)) {
    return t("aiAdmin.inputRate", { input });
  }
  if (unreadablePrice(rate.output_per_mtok)) {
    return "";
  }
  return t("aiAdmin.rates", {
    input,
    output: formatUsdPerMTok(rate.output_per_mtok, locale),
  });
}
