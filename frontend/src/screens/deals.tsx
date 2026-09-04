import {
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  type ComponentProps,
  type Dispatch,
  type DragEvent,
  type MouseEvent,
  type ReactNode,
  type SetStateAction,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { approvalDotTier, useAgentTierMap, verbTier } from "../app/autonomy";
import { useCanWriteRecord } from "../app/capability";
import { PageAsideToggle, usePageAside } from "../app/pageaside";
import { usePageName } from "../app/pagemeta";
import { useRecordZone } from "../app/recordzone";
import { navigate, routeHash } from "../app/router";
import { useInstallationSettings } from "../app/uploadlimit";
import { currentParams, type UrlParams, useUrlParams } from "../app/urlstate";
import { activityTimeline } from "../design-system/activitytimeline";
import {
  Badge,
  Button,
  Card,
  DataTable,
  EmptyState,
  Modal,
  OverflowMenu,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import {
  type BoardColumn,
  type BoardDeal,
  type BoardMoneyColumn,
  PipelineBoard,
  RecordView,
} from "../design-system/composed";
import {
  IdentityFact,
  IdentityLine,
  IdentityMeta,
} from "../design-system/identityline";
import type { ListChip } from "../design-system/listsurface";
import type { ListColumn, ListSelection } from "../design-system/listtable";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { FieldGuard } from "../design-system/rbac";
import { RecordTabs } from "../design-system/recordtabs";
import {
  useRecordTimeline,
  useTimelineFilters,
} from "../design-system/recordtimeline";
import { Select } from "../design-system/select";
import { StageLadder, type StageStep } from "../design-system/stageladder";
import { TimelineFilterBar } from "../design-system/timelinefilterbar";
import { useToast } from "../design-system/toast";
import { AutonomyDot, ProvenanceTag } from "../design-system/trust";
import {
  formatDate,
  formatDuration,
  formatMoney,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { idleSince } from "../format/idlebase";
import { toMajorUnits, toMinorUnits } from "../format/minorunits";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { dealRecordKeys, dealWinKeys } from "./activitykeys";
import { approvalKindLabel } from "./approvalkind";
import { usePendingApprovals } from "./approvals.queries";
import { ArchiveAction } from "./archive";
import {
  LoadMoreButton,
  OverlayUnavailable,
  problemFieldErrorsOf,
  problemMessageOf,
  provenanceOf,
  QueryGate,
  QueryStates,
  throwProblem,
  timelineZoneNotice,
  useMe,
  useSorMode,
  useViewerId,
} from "./common";
import { RecordContextPanel } from "./context";
import type { CreateField } from "./create";
import { CreateAction } from "./create";
import { CustomFieldsCard } from "./customfields.card";
import {
  type ObjectCustomFields,
  useObjectCustomFields,
} from "./customfields.form";
import { DealCommitteeMap } from "./deal360/dealcommittee";
import { DealPulse } from "./deal360/dealpulse";
import { DealSeats } from "./deal360/dealseats";
import { DealStrip } from "./deal360/dealstrip";
import { useDealCoverage } from "./deal360/usedealcoverage";
import { useDealRecipientAddress } from "./deal360/usedealrecipient";
import { DealBulkBar } from "./dealbulk";
import { DealEmailAside } from "./dealemail";
import { DealFiles } from "./dealfiles";
import {
  DealProjectChip,
  dealProjectFields,
  resolveDealProject,
  StartDeliveryPrompt,
  useProjectsOfCompany,
} from "./dealproject";
import { DealRoomAside } from "./dealroom";
import { DealStatusCardPanel, useDealStatusCard } from "./dealstatus";
import { EditAction } from "./edit";
import {
  EntityRef,
  rosterOwnerName,
  useEntityName,
  useRoster,
  useRosterPartial,
} from "./entityref";
import { RecordHistoryTab } from "./history";
import {
  type FilterSpec,
  LIST_PAGE_SIZES,
  type ListQuery,
  type ListState,
  ListTable,
  listPageOf,
  listQueryFromParams,
  mergeScreenDials,
  paramsFromListQuery,
  useTagChips,
  withListPage,
  withoutScreenDials,
} from "./listquery";
import { LogActivity } from "./logactivity";
import { useOpenEmail, withEmailOpener } from "./openemail";
import type { Project } from "./projects.form";
import { RecordReading, RecordReadingPair, TimelineThread } from "./record360";
import { RecordEmailVerb } from "./recordemail";
import { tagsColumn } from "./recordlist";
import { invalidateRecord } from "./recordwritekeys";
import { RelationshipsTab } from "./relationships";
import { SaveViewAction, useSavedViewTabs } from "./savedviews";
import { ShareAction } from "./share";
import { parseTagIDs, parseTagMode, tagQueryParams } from "./tagfilter";
import { TagsPanel } from "./tagspanel";
import { TimelineActions } from "./timelineactions";
import { groupChronology } from "./timelinegroups";

// Deal surfaces (B-EP09.11a/b/c): the five-stage Kanban with drag-to-advance
// (terminal stages are a 🟡 confirm, AC-deal-6), the board↔table segmented
// control over the SAME fetched set (no reload), and the deal 360 with the
// stage stepper and the live pending-approval staged cards. Weighting math
// stays out of the UI beyond same-currency page-local sub-lines: a mixed-
// currency column renders no sum (the FX rule: never sum native minors
// across currencies).

type Deal = components["schemas"]["Deal"];
type Organization = components["schemas"]["Organization"];
type Stage = components["schemas"]["Stage"];
type Pipeline = components["schemas"]["Pipeline"];
type Offer = components["schemas"]["Offer"];
type DealStatusCard = components["schemas"]["DealStatusCard"];
type Activity = components["schemas"]["Activity"];

/**
 * The pipeline a record belongs to, falling back to the default.
 *
 * `pipelineId` is the deal's own. Without it this answered the DEFAULT
 * pipeline for every deal, which was harmless while the stepper only drew
 * stage names but is not once those stages are the moves on offer: the stages
 * of a pipeline the deal is not in are targets the server refuses as a
 * pipeline mismatch.
 */
function usePipeline(pipelineId?: string | null) {
  return useQuery({
    queryKey: ["pipelines", pipelineId ?? "default"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      const pipeline =
        (pipelineId &&
          data.data.find((candidate) => candidate.id === pipelineId)) ||
        data.data.find((candidate) => candidate.is_default) ||
        data.data[0];
      if (!pipeline) {
        throw new Error("no pipeline");
      }
      return pipeline;
    },
  });
}

// The plural read over ALL pipelines (D-9's selector) — a DISTINCT cache key
// from usePipeline's ["pipelines"] (which DealScreen still reads as a single
// Pipeline). Sharing the key would let the cache hold either shape depending
// on which screen loaded last; ["pipelines","all"] still gets refreshed by
// any mutation that invalidates the ["pipelines"] prefix (react-query prefix
// matching), so freshness is preserved without a shape collision.
// enabled is false in overlay mode: the overlay deals view renders no
// pipeline board or picker (a stage-less mirror has no pipelines to show),
// so it never needs this fetch.
function usePipelines(enabled: boolean) {
  return useQuery({
    queryKey: ["pipelines", "all"],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

type DealFilters = {
  pipelineId: string;
  sort: string;
  includeArchived: boolean;
  filters: Record<string, string>;
  // Overlay mode reads a mirror that refuses every dial below (sort, and the
  // pipeline/stage/owner/org filters) with a 422 — so in overlay we send none
  // of them and let the deals list come back flat. The screen forces the table
  // view and hides the pickers to match (a stage-keyed board cannot place a
  // mirror deal, whose pipeline/stage is null in overlay, OVA-MAP-6).
  overlay: boolean;
};

// The two dials this screen owns beyond the shared list vocabulary.
//
// `pipeline_id` is already a wire parameter name, so the address and the
// endpoint say it the same way. `view` is the screen's own, because which of
// the board and the table is drawn changes nothing about which deals exist —
// and it is why these two are held out of the list codec rather than passed
// through it: read as filters they would be sent to /deals, which takes
// neither.
const PIPELINE_PARAM = "pipeline_id";
const VIEW_PARAM = "view";

// This screen's own dials, held out of the list's parameter space by the codec's
// shared pair rather than by copies kept here: the copies were the difference
// between this screen getting it right and the leads queue sending its drawing
// choice to the server as a filter.
const DEAL_SCREEN_DIALS: readonly string[] = [PIPELINE_PARAM, VIEW_PARAM];

/** `params` with one dial set, or removed when the value is empty. */
function withDialSet(params: UrlParams, key: string, value: string): UrlParams {
  const next = new Map(params);
  if (value) {
    next.set(key, value);
  } else {
    next.delete(key);
  }
  return next;
}

// FORECAST_FILTER_VALUES are the four buckets a deal's own column can hold.
// `slipped` is the report's derivation from a claimed category and a close
// date, so it is a dimension of the forecast tiles and never a filter here.
const FORECAST_FILTER_VALUES = [
  "commit",
  "best_case",
  "pipeline",
  "omitted",
] as const;

// forecastCategoryFilter narrows a stored dial to what the wire admits.
//
// The value arrives as a bare string because a SAVED VIEW carries it, and a
// view written before a category was renamed — or by hand — can hold anything.
// An unadmitted value selects nothing rather than being sent: the request
// would be refused, and a list that fails to load says less than one that
// simply is not narrowed. Same reading `tag_mode` takes of a stored mode its
// own enum no longer admits.
function forecastCategoryFilter(
  value: string | undefined,
): (typeof FORECAST_FILTER_VALUES)[number] | undefined {
  return FORECAST_FILTER_VALUES.find((admitted) => admitted === value);
}

// dealsQueryParams builds the native board's /deals query — the full dial
// set (pipeline/stage/owner/org filters + sort). It is never called in
// overlay mode (useDeals is disabled there and OverlayDealsTable sends its
// own overlay-shaped params), so it carries no overlay branch.
function dealsQueryParams(f: DealFilters) {
  const { filters } = f;
  return {
    limit: 100,
    include_archived: f.includeArchived || undefined,
    pipeline_id: f.pipelineId || undefined,
    sort: f.sort || undefined,
    stage_id: filters.stage_id || undefined,
    owner_id: filters.owner_id || undefined,
    organization_id: filters.organization_id || undefined,
    partner_org_id: filters.partner_org_id || undefined,
    stalled: filters.stalled === "true" ? true : undefined,
    forecast_category: forecastCategoryFilter(filters.forecast_category),
    partner_sourced: filters.partner_sourced === "true" ? true : undefined,
    // Several ids and their mode, out of the one comma-joined string the
    // address carries them in. Spelled by the shared encoder rather than here,
    // so this board and the three lists cannot read one address two ways.
    ...tagQueryParams(
      parseTagIDs(filters.tag_id),
      parseTagMode(filters.tag_mode),
    ),
  };
}

// The board is not paginated — limit:100 is an honest documented cap (a
// live Kanban reads one screenful, not a keyset walk). Disabled in overlay
// mode: there the flat mirror table paginates through OverlayDealsTable
// (its own keyset walk), so this single-page native query does not fetch.
/**
 * The deals the board and the table share.
 *
 * A keyset walk rather than one fixed page. The board draws a column per
 * stage out of whatever this holds, so a single capped read meant a busy
 * stage quietly showed a fraction of its cards while its header — which
 * comes from the deals-by-stage report over EVERY matching deal — went on
 * naming the true count. A column saying "40 deals" above six cards is the
 * one thing a pipeline view must not do.
 */
function useDeals(f: DealFilters) {
  return useInfiniteQuery({
    queryKey: ["deals", f],
    enabled: !f.overlay,
    // `as` steers useInfiniteQuery's TPageParam generic to the cursor type,
    // exactly as OverlayDealsTable does and for the same reason: a bare
    // `undefined` infers TPageParam=undefined and the string cursor no longer
    // type-checks.
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/deals", {
        params: { query: { ...dealsQueryParams(f), cursor: pageParam } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) =>
      last.page?.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
}

// dealsByStageReportFilters translates the board's own filter dials into
// the deals-by-stage report's filter shape — the SAME dials dealsQueryParams
// sends to /deals, so a card's stage total and the cards shown for it never
// disagree about which deals are in scope. stage_id is deliberately absent:
// the report is grouped BY stage_id, which already answers "per stage".
function dealsByStageReportFilters(f: DealFilters): Record<string, unknown> {
  const { filters } = f;
  const out: Record<string, unknown> = {};
  if (f.pipelineId) out.pipeline_id = f.pipelineId;
  if (filters.owner_id) out.owner_id = filters.owner_id;
  if (filters.organization_id) out.organization_id = filters.organization_id;
  if (filters.partner_org_id) out.partner_org_id = filters.partner_org_id;
  if (filters.stalled === "true") out.stalled = true;
  if (filters.partner_sourced === "true") out.partner_sourced = true;
  return out;
}

// useStageTotals reads the board's per-column totals from the
// deals-by-stage report — a full aggregate over every matching deal, not
// just the capped page useDeals fetches. Grouped by
// [stage_id, currency] so a mixed-currency stage arrives as more than one
// row, which buildStageTotals reads as "hide the sum" — the report never
// includes archived deals (like every report), so the totals reflect the
// live pipeline regardless of the board's "show archived" toggle.
function useStageTotals(f: DealFilters) {
  return useQuery({
    // Under ["deals"] on purpose, so the ONE invalidation every deal mutation
    // already fires refreshes the column headers along with the cards. Keyed
    // apart from them, a moved card sat under a header still counting it in
    // the stage it left — and a foreign-currency deal arriving in a
    // single-currency stage left the old sum standing, which is the mixed-
    // currency refusal not happening.
    queryKey: ["deals", "by-stage-totals", f],
    // Not while a tag narrows the board. The report's filter vocabulary has no
    // tag field — sending one is a 422 — so the totals would count deals the
    // board is not showing, and a column header reporting more than the cards
    // under it is a number the reader has no way to reconcile. Withheld is the
    // honest answer, and buildStageTotals already draws the no-sum column for
    // the mixed-currency case.
    enabled: !f.overlay && parseTagIDs(f.filters.tag_id).length === 0,
    queryFn: async () => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report: "deals-by-stage" } },
        body: {
          group_by: ["stage_id", "currency"],
          aggregates: [
            { fn: "count", as: "deals" },
            { fn: "sum", field: "amount_minor", as: "raw_minor" },
            {
              fn: "sum",
              field: "weighted_amount_minor",
              as: "weighted_minor",
            },
          ],
          filters: dealsByStageReportFilters(f),
        },
      });
      if (error) {
        throwProblem(error);
      }
      return buildStageTotals(data.rows);
    },
  });
}

// OverlayDealsTable is the overlay-mode deals view: a flat mirror table
// (a stage-keyed board cannot place a mirror deal, whose pipeline/stage is
// null — OVA-MAP-6) that walks the keyset cursor the API returns
// (page.next_cursor / page.has_more) with a Load-more affordance, rather
// than the native board's honest one-screenful cap. Overlay reads 422 every
// sort/filter dial, so it sends only limit + include_archived + cursor.
function OverlayDealsTable({
  includeArchived,
}: Readonly<{ includeArchived: boolean }>) {
  const query = useInfiniteQuery({
    queryKey: ["deals", "overlay", includeArchived],
    // `as` steers useInfiniteQuery's TPageParam generic to the cursor type:
    // a bare `undefined` infers TPageParam=undefined, which then rejects the
    // string cursor getNextPageParam returns (the whole query's data type
    // collapses to unknown). A typed local does not carry through the
    // generic inference — so the assertion is load-bearing here, not
    // cosmetic. biome (the frontend gate) does not flag it.
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/deals", {
        params: {
          query: {
            limit: 100,
            include_archived: includeArchived || undefined,
            cursor: pageParam,
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) =>
      last.page?.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
  const t = useT();
  // Once ANY page has loaded, render the table — a later Load-more failure
  // must NOT discard the rows already fetched (routing the whole thing
  // through QueryGate would show the full error state on any page error,
  // throwing away usable results). Only the INITIAL load goes through
  // QueryStates' pending/error; a failed next page leaves the table up and
  // re-enables the Load-more button to retry.
  const pages = query.data?.pages ?? [];
  if (pages.length === 0) {
    return (
      <QueryStates
        query={query}
        pendingLabel={t("deals.loading")}
        // A table's worth of rows. The default placeholder is three lines, and
        // a list that arrives into a third of the room the placeholder held
        // pushes the page down under the reader as it lands.
        pendingLines={8}
      >
        {null}
      </QueryStates>
    );
  }
  const deals = pages.flatMap((p) => p.data);
  if (deals.length === 0) {
    return <EmptyState>{t("common.empty")}</EmptyState>;
  }
  return (
    <>
      <DealTable deals={deals} />
      <LoadMoreButton query={query} />
    </>
  );
}

/** A company's display name and the mark drawn beside it. */
type OrgMark = { name: string; logoUrl?: string | null };

/**
 * Every company the loaded deals name, id → mark (`useOrgMarks` resolves them).
 *
 * A company this reader may not read is in no map: the wire sends
 * `organization_id` as null and names it in `masked_fields`, so what the card
 * needs there is the withheld READING, which the card itself spells as the mask
 * — not a name this screen could supply.
 */
export type CompanyMarks = ReadonlyMap<string, OrgMark>;

/**
 * What the screen knows about the companies its deals name.
 *
 * `unreadable` is the reading the board used to lose. A read that FAILED — a
 * 403 because the reader holds row visibility of the company but no
 * `organization:read` grant, a 5xx, a dropped connection — is not the same fact
 * as a deal that names no company, and collapsing the two told the reader the
 * most misleading of the two. The table has always had this reading through
 * `EntityRef`'s failed state; this is the board's half of it.
 */
export type CompanyNaming = Readonly<{
  marks: CompanyMarks;
  unreadable: ReadonlySet<string>;
}>;

/**
 * What a deal's company reads as on its card, in the four readings it has.
 *
 * Withheld carries the mask, the same control the table's company cell draws. A
 * company the screen has a name for is named. A company whose read FAILED says
 * so, because a deal that names a company the reader could not fetch is not a
 * deal with no company. Only a deal naming no company draws nothing.
 *
 * A name still in flight also draws nothing rather than a uuid: the card's
 * company line is a name a reader recognises and an id is not one. That is the
 * one case where an empty slot is a wait rather than a claim, and it resolves
 * itself.
 */
function dealCompany(
  deal: Deal,
  naming: CompanyNaming,
): Pick<BoardDeal, "org" | "orgLogoUrl" | "orgWithheld" | "orgUnreadable"> {
  if (deal.masked_fields?.includes("organization_id")) {
    return { org: "", orgWithheld: true };
  }
  if (deal.organization_id && naming.unreadable.has(deal.organization_id)) {
    return { org: "", orgUnreadable: true };
  }
  const mark = deal.organization_id
    ? naming.marks.get(deal.organization_id)
    : undefined;
  return { org: mark?.name ?? "", orgLogoUrl: mark?.logoUrl };
}

export function toBoardDeal(deal: Deal, naming: CompanyNaming): BoardDeal {
  const since = idleSince(deal);
  return {
    id: deal.id,
    name: deal.name,
    ...dealCompany(deal, naming),
    // Both halves as the wire sent them. Nobody has priced every deal, and a
    // card that filled in either half would state a figure this deal does not
    // have — a zero amount, or a euro sign over an unknown currency.
    valueMinor: deal.amount_minor ?? null,
    currency: deal.currency ?? null,
    ageMs: Math.max(0, Date.now() - new Date(since).getTime()),
    stalled: deal.stalled ?? false,
    archived: deal.archived_at != null,
    tags: deal.tags,
  };
}

type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];
type CreateDealRequest = components["schemas"]["CreateDealRequest"];

// One deal as the edit form's initial values. Extracted from the badge row
// that renders the form: mapping a record onto form fields is its own job, and
// keeping it inline made a component that already draws badges, an edit dialog
// and an archive verb carry a twentieth concern.
//
// It is also the patch's BASELINE (mapDealUpdate), so a field this function
// misreads becomes a field every save reports as changed.
//
// Every absent value becomes "" rather than a default. A currency the FORM
// chose is a currency the SAVE writes, so seeding one made an unpriced deal
// acquire it the moment a reader edited its name — and since amount and
// currency are paired by CHECK, that turned an innocent rename into a refusal.
function dealEditRecord(deal: Deal): Record<
  string,
  string | number | undefined
> & {
  id: string;
  version?: number;
} {
  return {
    id: deal.id,
    version: deal.version,
    name: deal.name,
    // The currency's own scale, not a hundred. amount_minor and currency are
    // NULL together (the deal_amount_currency_pair CHECK), so a priced deal
    // always carries the code — but a row that WITHHELD the currency for row
    // scope while sending the amount would be scaled at the two-digit default
    // and shown wrong. There is no honest figure without its unit, so the field
    // stays empty rather than guessing one.
    amount:
      deal.amount_minor != null && deal.currency
        ? String(toMajorUnits(deal.amount_minor, deal.currency))
        : "",
    currency: deal.currency ?? "",
    owner_id: deal.owner_id ?? "",
    organization_id: deal.organization_id ?? "",
    partner_org_id: deal.partner_org_id ?? "",
    partner_attribution: deal.partner_attribution ?? "",
    forecast_category: deal.forecast_category ?? "",
    expected_close_date: deal.expected_close_date ?? "",
    wait_until: deal.wait_until ?? "",
    project_id: deal.project_id ?? "",
  };
}

// The edit form's values are typed `unknown` per key; the project resolver
// reads the three it needs as strings, and anything else is not one.
function stringValues(values: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(values)) {
    if (typeof value === "string") {
      out[key] = value;
    }
  }
  return out;
}

// The attribution the form may send, narrowed the same way the forecast
// category is: the wire type is a closed vocabulary, and a free string from a
// form input is not one until something checks it.
function partnerAttribution(
  v: string,
): UpdateDealRequest["partner_attribution"] {
  switch (v) {
    case "sourced":
      return "sourced";
    case "influenced":
      return "influenced";
    default:
      return null;
  }
}

function forecastCategory(v: string): UpdateDealRequest["forecast_category"] {
  switch (v) {
    case "commit":
      return "commit";
    case "best_case":
      return "best_case";
    case "pipeline":
      return "pipeline";
    case "omitted":
      return "omitted";
    default:
      return null;
  }
}

/**
 * The edit form's values as a deal patch — a DIFF, not a snapshot.
 *
 * `seeded` is the record the form opened on (`dealEditRecord`). A key appears
 * only where the form's value differs from it, and that is the whole point: the
 * body used to carry every field on every save, so a deal missing a value the
 * form does not even RENDER — `partner_attribution` on an installation with no
 * partners — resubmitted `null` as a real instruction to clear it. The API reads
 * an explicit null as "forget this" and refused, correctly, naming a field the
 * person had never seen. Nothing the person did not touch travels now.
 *
 * A blank over a stored value is still a change, and still travels as null: the
 * pickers offer "Unset" in words, so clearing a company or a partner is a
 * choice somebody made rather than a gap in what the form knows.
 *
 * `amount` arrives in major units from the form and the wire is minor units
 * (deal creation applies the same conversion above). The two money halves are
 * compared as the form spells them, because that is where the person's edit is.
 *
 * `masked` names the fields THIS reader was not shown. A withheld reference
 * arrives as null with `masked_fields` naming it — deliberately, so a reader
 * can tell "you may not see this" from "there is nothing here" — but the form
 * has only the null, and sending it back would ask the server to clear a
 * partner the reader never saw and never touched. So a masked field is left
 * out of the patch entirely: omitted means unchanged.
 */
export function mapDealUpdate(
  values: Record<string, unknown>,
  seeded: Record<string, unknown> = {},
  masked: readonly string[] = [],
): UpdateDealRequest {
  const str = (v: unknown) => (typeof v === "string" ? v.trim() : "");
  const currency = str(values.currency);
  const amount = str(values.amount);
  const patch: UpdateDealRequest = {};
  // One reading of "the person moved this field", over the form's own spelling
  // of the value rather than the wire's: a select left alone holds exactly the
  // string it was seeded with, and the wire shape (minor units, a narrowed
  // vocabulary) is derived from that string afterwards.
  //
  // The wire key and the form key are named together at every call, because the
  // one place this can go wrong is a field compared under one name and sent
  // under another — which sends an untouched field, or swallows a real edit.
  const moved = (formKey: string) =>
    str(values[formKey]) !== str(seeded[formKey]);
  function onMove<K extends keyof UpdateDealRequest>(
    formKey: string,
    wireKey: K,
    value: () => UpdateDealRequest[K],
  ) {
    if (moved(formKey)) {
      patch[wireKey] = value();
    }
  }
  // A required field: an emptied name is a form the person has not finished,
  // not an instruction to erase the deal's name.
  onMove("name", "name", () => str(values.name) || undefined);
  // The money pair travels together whenever either half moves, because the
  // SCALE is part of the amount: amount_minor is denominated in the currency's
  // own minor units — a dong has none, a dinar has three — so a currency moving
  // alone re-denominates the figure the row already holds. 10000 JPY re-saved as
  // EUR became 10000 minor EUR, which is €100, and the fx freeze and the
  // forecast history then recorded that as the price.
  //
  // The exception is a deal with no figure at all: naming a currency there is
  // half a money value, and the server's own pair refusal says so better than an
  // amount_minor this form would have to invent.
  if (moved("amount") || (moved("currency") && amount)) {
    patch.amount_minor = amount ? toMinorUnits(Number(amount), currency) : null;
  }
  onMove("currency", "currency", () => currency || undefined);
  onMove(
    "organization_id",
    "organization_id",
    () => str(values.organization_id) || null,
  );
  onMove("owner_id", "owner_id", () => str(values.owner_id) || null);
  onMove(
    "partner_org_id",
    "partner_org_id",
    () => str(values.partner_org_id) || null,
  );
  // Only alongside a partner. Clearing the partner clears what they did with
  // it — the two are one fact, stored under one CHECK — so sending the
  // attribution's own null beside it would name a claim with nobody left to
  // attribute it to, which the API refuses.
  if (str(values.partner_org_id)) {
    onMove("partner_attribution", "partner_attribution", () =>
      partnerAttribution(str(values.partner_attribution)),
    );
  }
  onMove("forecast_category", "forecast_category", () =>
    forecastCategory(str(values.forecast_category)),
  );
  onMove(
    "expected_close_date",
    "expected_close_date",
    () => str(values.expected_close_date) || null,
  );
  onMove("wait_until", "wait_until", () => str(values.wait_until) || null);
  // Resolved by the screen before mapping: the "new project" answer has
  // already become an id by the time the patch is built.
  onMove("project_id", "project_id", () => str(values.project_id) || null);
  return withoutMasked(patch, masked);
}

/**
 * The patch minus every field the reader was not shown.
 *
 * `partner_org_id` carries its attribution: the server withholds the pair
 * together, so returning one half of it would decide what a partner nobody
 * could see is owed.
 */
function withoutMasked(
  patch: UpdateDealRequest,
  masked: readonly string[],
): UpdateDealRequest {
  if (masked.length === 0) {
    return patch;
  }
  const out: Record<string, unknown> = { ...patch };
  for (const field of masked) {
    delete out[field];
    if (field === "partner_org_id") {
      delete out.partner_attribution;
    }
    if (field === "amount_minor") {
      delete out.currency;
    }
  }
  return out as UpdateDealRequest;
}

/**
 * The create form's values as the deal-birth body.
 *
 * A deal names its partner at birth rather than only through a later edit: the
 * win that pays the partner can land before anybody revisits the record, and
 * commission accrues on a `sourced` attribution alone. Both partner fields
 * travel here for the same reason the update body carries them — a create that
 * quietly dropped them told the caller its write had succeeded while the
 * partner was gone.
 */
export function mapDealCreate(
  values: Record<string, unknown>,
  pipelineId: string,
): CreateDealRequest {
  const str = (v: unknown) => (typeof v === "string" ? v.trim() : "");
  const amount = str(values.amount);
  const currency = str(values.currency) || "EUR";
  return {
    name: str(values.name),
    pipeline_id: pipelineId,
    stage_id: str(values.stage_id),
    // The UI takes major units; the wire is minor units, at the scale THIS
    // currency carries — a dong has none, so multiplying by a hundred here
    // priced the deal a hundredfold and nothing downstream could tell.
    amount_minor: amount ? toMinorUnits(Number(amount), currency) : null,
    currency,
    organization_id: str(values.organization_id) || null,
    partner_org_id: str(values.partner_org_id) || null,
    // The empty option means the caller made no claim, and null is how that
    // travels: the server then reads a named partner as `sourced`, which is
    // what the option says it does. An attribution naming no partner is
    // refused 422 rather than defaulted — there would be nobody to credit.
    partner_attribution: partnerAttribution(str(values.partner_attribution)),
    expected_close_date: str(values.expected_close_date) || null,
    project_id: str(values.project_id) || null,
    source: "manual",
  };
}

const FORECAST_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "commit", label: "deal.fcCommit" },
  { value: "best_case", label: "deal.fcBestCase" },
  { value: "pipeline", label: "deal.fcPipeline" },
  { value: "omitted", label: "deal.fcOmitted" },
];

// What a partner did for the deal. Only "sourced" earns commission — a partner
// who helped a deal we already had is recorded, not paid.
//
// The empty value leads and says what leaving it unset MEANS. The server
// defaults a named partner to "sourced", so a bare "Unset" here would let a
// reader believe they had made no claim while the deal quietly became
// commission-eligible. A field that offers the empty value in its own words
// suppresses the generic entry (create.tsx), which is why this one is spelled
// out rather than inherited.
const ATTRIBUTION_OPTIONS: { value: string; label: MessageKey }[] = [
  { value: "", label: "deal.attributionUnset" },
  { value: "sourced", label: "deal.attributionSourced" },
  { value: "influenced", label: "deal.attributionInfluenced" },
];

/**
 * The companies a deal may name as the partner who brought it.
 *
 * Only companies that ARE partners: the picker once offered every
 * organization, which let a deal be attributed to an ordinary customer — and
 * an attribution to a company with no partner row has no margin tier behind
 * it, so it looks attributed and silently never earns anything.
 *
 * An empty list is also the answer to "is there a partner programme here": an
 * installation that has never made a company a partner has no partner
 * question to ask, and the two fields stay off the form entirely.
 */
export function usePartnerOptions(
  orgs: { id: string; display_name: string }[],
): { value: string; label: string }[] {
  const partners = useQuery({
    queryKey: ["partners", "options"],
    queryFn: async () => {
      const { data, error } = await api.GET("/partners", {
        params: { query: { limit: 200 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
    staleTime: 60_000,
  });
  // The names come from a read of the PARTNER companies rather than from
  // whatever page of organizations the screen happens to hold.
  //
  // The caller's `orgs` is one capped page (50 on the deals screen), and
  // intersecting the two dropped any partner whose company fell outside it —
  // silently, and differently depending on which screen asked. That is
  // survivable for a form picker, which injects the deal's own stored partner
  // when it is missing; a FILTER has no such fallback, so a partner it cannot
  // name is a partner nobody can narrow by.
  const names = useQuery({
    queryKey: ["partners", "names"],
    staleTime: 60_000,
    queryFn: async () => {
      // relationship_type=partner reads the same set from the other side, so
      // the page is partner companies rather than the first N companies of
      // any kind. The cap matches /partners' own.
      const { data, error } = await api.GET("/organizations", {
        params: { query: { relationship_type: "partner", limit: 200 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
  // Still the caller's page as a fallback while the keyed read is in flight,
  // so the options do not blink empty on every mount.
  const named = new Map<string, string>(
    orgs.map((org) => [org.id, org.display_name]),
  );
  for (const org of names.data ?? []) {
    named.set(org.id, org.display_name);
  }
  return (partners.data ?? []).flatMap((partner) => {
    const label = named.get(partner.organization_id);
    // A partner whose organization the caller cannot read is left out rather
    // than offered as a bare id: picking it would name a company the picker
    // could not show them.
    return label ? [{ value: partner.organization_id, label }] : [];
  });
}

/**
 * The two fields that record a deal coming through a partner, for both the
 * create form and the edit form — one spelling, so the two cannot drift.
 *
 * Nothing at all when the installation has no partners AND the deal names
 * none: an empty picker asks a question with no answers in it, and a form that
 * shows partner controls to a company running no partner programme is claiming
 * a feature it does not have.
 *
 * `attributed` is what keeps that from eating data. A form omits nothing a
 * record already carries: the pickable list is one capped page of companies, so
 * a deal's own partner can be missing from it, and dropping the field would
 * blank the stored partner the next time anybody edited anything else on the
 * deal — taking the commission attribution with it, silently.
 *
 * The attribution follows the partner rather than standing beside it. What a
 * partner DID is a question about a partner, so it appears once one is named
 * and disappears when the choice is cleared — asking it first offers a claim
 * with nobody to attach it to.
 */
function partnerFields(
  t: (k: MessageKey) => string,
  partnerOptions: { value: string; label: string }[],
  attributed?: { id: string; label: string },
  // The reader was not shown this deal's partner (`masked_fields` names it).
  // Checked BEFORE the empty-list exit: a withheld partner is a fact about
  // this deal, so it is stated whether or not the installation has partners
  // left to offer.
  withheld = false,
): CreateField[] {
  if (withheld) {
    // One entry, and it names the field as withheld. A picker seeded blank
    // over a partner the reader never saw invites them to choose a different
    // one, and the attribution travels with the partner — so there is nothing
    // here to attribute either. `mapDealUpdate` leaves both out of the patch.
    return [
      {
        key: "partner_org_id",
        label: "deal.partnerOrg",
        type: "select",
        options: [{ value: "", label: t("deal.partnerWithheld") }],
      },
    ];
  }
  if (partnerOptions.length === 0 && !attributed) {
    return [];
  }
  // The deal's own partner is always among the choices, even when the capped
  // page of partners this form could offer does not include it. A select whose
  // stored value is not an option shows blank, and saving that blank clears the
  // partner nobody meant to touch.
  const options =
    attributed && !partnerOptions.some((o) => o.value === attributed.id)
      ? [{ value: attributed.id, label: attributed.label }, ...partnerOptions]
      : partnerOptions;
  return [
    {
      key: "partner_org_id",
      label: "deal.partnerOrg",
      type: "select",
      options,
    },
    // Commission accrues on "sourced" only, so this is the field that decides
    // whether a win pays the partner.
    {
      key: "partner_attribution",
      label: "deal.partnerAttribution",
      type: "select",
      showWhen: (values) => Boolean(values.partner_org_id),
      options: ATTRIBUTION_OPTIONS.map((o) => ({
        value: o.value,
        label: t(o.label),
      })),
    },
  ];
}

/**
 * The partner a deal already names, ready to stand as its own option.
 *
 * Falls back to the raw id when the org list cannot name it — an unreadable or
 * off-page company. Ugly, and correct: the alternative is a blank select whose
 * save clears an attribution the reader never touched.
 */
function attributedPartner(
  deal: Deal,
  orgs: { id: string; display_name: string }[],
): { id: string; label: string } | undefined {
  if (!deal.partner_org_id) {
    return undefined;
  }
  const named = orgs.find((org) => org.id === deal.partner_org_id);
  return {
    id: deal.partner_org_id,
    label: named?.display_name ?? deal.partner_org_id,
  };
}

/**
 * The company picker on a deal's edit form, in the three readings a stored
 * company has.
 *
 * Withheld is the reading that offers no company at all. The reader was not
 * shown which company this is, so a picker seeded blank invites them to pick
 * one and silently re-point the deal away from the company a colleague linked;
 * its single entry names the field as withheld instead, and `mapDealUpdate`
 * leaves the field out of the patch entirely.
 *
 * A company the pickable page cannot reach is still an option, for the reason
 * the partner field carries the same rule: a select whose stored value is not
 * among its options shows blank, and saving that blank clears a link nobody
 * meant to touch.
 */
function companyEditField(
  t: (k: MessageKey) => string,
  opts: {
    orgs: { id: string; display_name: string }[];
    masked: readonly string[];
    currentCompany?: { id: string; label: string };
  },
): CreateField {
  if (opts.masked.includes("organization_id")) {
    return {
      key: "organization_id",
      label: "create.organization",
      type: "select",
      options: [{ value: "", label: t("deal.companyWithheld") }],
    };
  }
  const options = opts.orgs.map((org) => ({
    value: org.id,
    label: org.display_name,
  }));
  const current = opts.currentCompany;
  return {
    key: "organization_id",
    label: "create.organization",
    type: "select",
    options:
      current && !options.some((option) => option.value === current.id)
        ? [{ value: current.id, label: current.label }, ...options]
        : options,
  };
}

export function dealEditFields(
  t: (k: MessageKey) => string,
  opts: {
    orgs: { id: string; display_name: string }[];
    partnerOptions: { value: string; label: string }[];
    // The partner this deal ALREADY names, when it names one. Keeps the field
    // (and its stored value) on a form whose pickable list does not reach it.
    attributedPartner?: { id: string; label: string };
    // The company this deal already names, resolved even when the pickable
    // page does not reach it — the same rule as `attributedPartner`, for the
    // same reason.
    currentCompany?: { id: string; label: string };
    // The fields of THIS deal the reader was not shown, as `masked_fields`
    // named them. A withheld reference is offered as withheld rather than as
    // an empty picker.
    masked: readonly string[];
    me: string;
    currentOwner: string | null;
    currency: string;
  },
): CreateField[] {
  const currencies = ["EUR", "USD", "GBP", "CHF"];
  if (opts.currency && !currencies.includes(opts.currency)) {
    currencies.unshift(opts.currency);
  }
  const ownerOptions = [
    ...(opts.currentOwner && opts.currentOwner !== opts.me
      ? [{ value: opts.currentOwner, label: t("deal.ownerKeep") }]
      : []),
    { value: opts.me, label: t("deal.ownerMe") },
    { value: "", label: t("deal.ownerUnassign") },
  ];
  return [
    { key: "name", label: "create.dealName", required: true },
    { key: "amount", label: "create.amount", type: "number" },
    {
      key: "currency",
      label: "create.currency",
      type: "select",
      required: true,
      options: currencies.map((c) => ({ value: c, label: c })),
    },
    {
      key: "owner_id",
      label: "deal.ownerMe",
      type: "select",
      options: ownerOptions,
    },
    companyEditField(t, opts),
    // Both partner fields are absent where no company has been made a partner:
    // there is no partner programme to attribute anything to, and an empty
    // picker is a question the reader cannot answer.
    ...partnerFields(
      t,
      opts.partnerOptions,
      opts.attributedPartner,
      opts.masked.includes("partner_org_id"),
    ),
    {
      key: "forecast_category",
      label: "deal.forecastCategory",
      type: "select",
      options: FORECAST_OPTIONS.map((o) => ({
        value: o.value,
        label: t(o.label),
      })),
    },
    { key: "expected_close_date", label: "create.expectedClose", type: "date" },
    { key: "wait_until", label: "deal.waitUntil", type: "date" },
  ];
}

// StageTotals is one stage's count/raw/weighted total, sourced from the
// deals-by-stage report rather than the board's own (capped) card fetch —
// a pipeline with more deals than the board's one-screenful cap was
// showing a confidently wrong sum.
export type StageTotals = {
  count: number;
  // Null where the report stated no figure. A stage can hold a real count of
  // deals nobody has priced, and its `SUM` then arrives as null with no
  // currency beside it — which is not the zero a naive read makes of it.
  rawMinor: number | null;
  weightedMinor: number | null;
  currency: string | null;
  sumHidden: boolean;
};

// A report cell as a figure, or nothing. An absent or non-numeric cell is the
// report declining to state an amount, and `Number(null)` turns that into a 0
// the server never sent.
function reportMinor(value: unknown): number | null {
  if (value == null || value === "") {
    return null;
  }
  const amount = Number(value);
  return Number.isFinite(amount) ? amount : null;
}

// buildStageTotals shapes a deals-by-stage report grouped by
// `["stage_id","currency"]` into one entry per stage. More than one
// currency row for a stage means the sum is genuinely cross-currency — the
// same rule the board has always applied, decided here from the report's
// full row set rather than from whichever cards happened to load.
export function buildStageTotals(
  rows: Record<string, unknown>[],
): Map<string, StageTotals> {
  const byStage = new Map<string, Record<string, unknown>[]>();
  for (const row of rows) {
    const stageId = String(row.stage_id ?? "");
    const forStage = byStage.get(stageId) ?? [];
    forStage.push(row);
    byStage.set(stageId, forStage);
  }
  const totals = new Map<string, StageTotals>();
  for (const [stageId, stageRows] of byStage) {
    const count = stageRows.reduce(
      (sum, row) => sum + Number(row.deals ?? 0),
      0,
    );
    const mixed = stageRows.length > 1;
    const single = stageRows[0];
    // ONE row is not the same fact as one CURRENCY. A stage whose deals are all
    // unpriced groups into a single row whose `currency` is null, so the
    // cross-currency test says nothing about it — and its figures belong to no
    // currency at all. Naming EUR there is indistinguishable from a real EUR
    // total, which is the reading a rep would act on.
    const currency =
      !mixed && typeof single.currency === "string" && single.currency !== ""
        ? single.currency
        : null;
    totals.set(stageId, {
      count,
      rawMinor: currency ? reportMinor(single.raw_minor) : null,
      weightedMinor: currency ? reportMinor(single.weighted_minor) : null,
      currency,
      sumHidden: mixed,
    });
  }
  return totals;
}

/**
 * The company marks the board draws, for every company its cards name.
 *
 * The create form's picker reads ONE capped page of organizations, and the
 * board took its marks from exactly that page — so a deal whose company fell
 * outside it drew a card with no company row at all, which a reader reads as a
 * deal nobody has linked. The set that has to be resolvable is the set the
 * loaded deals actually name, so the ids that page did not cover are read one
 * at a time and cached per id: reading the same board again, or scrolling back
 * over the same companies, costs no further request.
 *
 * A withheld company is never among them — the wire sends no id to read — so
 * this cannot turn a mask into a name.
 */
export function useOrgMarks(
  deals: Deal[],
  page: Organization[],
  pageSettled: boolean,
): CompanyNaming {
  const fromPage = new Map<string, OrgMark>(
    page.map((org) => [
      org.id,
      { name: org.display_name, logoUrl: org.logo_url },
    ]),
  );
  // Nothing is fanned out until the picker's page has ANSWERED. The two reads
  // are issued together and settle in no fixed order, so on every render where
  // the deals have arrived and the organizations have not, `fromPage` is empty
  // and every company a loaded deal names looks unresolved — one request each,
  // for a page that is about to answer most of them. A cold board paint fired
  // up to a hundred, and nothing un-sends a request.
  const unnamed = pageSettled
    ? [
        ...new Set(
          deals.flatMap((deal) =>
            deal.organization_id && !fromPage.has(deal.organization_id)
              ? [deal.organization_id]
              : [],
          ),
        ),
      ]
    : [];
  const reads = useQueries({
    queries: unnamed.map((id) => ({
      queryKey: ["organizations", "mark", id],
      queryFn: async (): Promise<OrgMark | null> => {
        const { data, error, response } = await api.GET("/organizations/{id}", {
          params: { path: { id } },
        });
        if (error) {
          // A 404 is an ANSWER — the company is archived, or row scope hides
          // its existence from this reader — and no retry turns it into a
          // name, so the card has no company to draw. Every other failure is a
          // read that never arrived and throws, so it is held as an error
          // rather than settled as an absence. The same rule the shared
          // reference resolver states (screens/entityref.tsx).
          if (response.status === 404) {
            return null;
          }
          throwProblem(error);
        }
        return { name: data.display_name, logoUrl: data.logo_url };
      },
      // A company's name and mark change far more rarely than the board
      // refetches, so a card that already has one does not ask again.
      staleTime: 60_000,
    })),
  });
  const marks = new Map(fromPage);
  const unreadable = new Set<string>();
  reads.forEach((read, index) => {
    const id = unnamed[index];
    if (!id) {
      return;
    }
    if (read.data) {
      marks.set(id, read.data);
      return;
    }
    // The error the queryFn deliberately threw rather than settling as an
    // absence. Read here, or the card it belongs to says "no company" — which
    // is the one thing this read exists to stop it saying.
    if (read.isError) {
      unreadable.add(id);
    }
  });
  return { marks, unreadable };
}

export function buildColumns(
  stages: Stage[],
  deals: Deal[],
  totals: Map<string, StageTotals>,
  naming: CompanyNaming,
): BoardMoneyColumn[] {
  return [...stages]
    .sort((a, b) => a.position - b.position)
    .map((stage) => {
      const stageDeals = deals.filter((deal) => deal.stage_id === stage.id);
      const stageTotals = totals.get(stage.id);
      return {
        stage: stage.id,
        label: stage.name,
        probabilityPct: stage.win_probability,
        // No totals row yet — the report is still in flight, or this stage was
        // not in it. Either way the figure is unknown rather than zero, and the
        // column draws it as absent while still stating the count below.
        rawMinor: stageTotals?.rawMinor ?? null,
        weightedMinor: stageTotals?.weightedMinor ?? null,
        currency: stageTotals?.currency ?? null,
        deals: stageDeals.map((deal) => toBoardDeal(deal, naming)),
        // The true count, not the loaded page's — falls back to the page
        // count while totals are still loading, so the column shows SOME
        // number rather than a misleading 0.
        count: stageTotals?.count ?? stageDeals.length,
        sumHidden: stageTotals?.sumHidden ?? false,
      };
    });
}

/**
 * One of a deal's company references, as a table cell.
 *
 * Three readings, and only the last of them is blank. Withheld reads as
 * withheld: the wire sends the id as null and names the field in
 * `masked_fields`, so the null is a refusal rather than an absence, and a cell
 * that drew nothing would state the opposite. A reference the reader may see
 * resolves to the company's name. A deal that names no company has an empty
 * cell, which is the one case where empty is the truth.
 *
 * `asText` because the row is already the link to the deal: a control nested
 * inside one is invalid markup, and the second route would go where the first
 * one already goes.
 */
function CompanyCell({
  deal,
  field,
}: Readonly<{ deal: Deal; field: "organization_id" | "partner_org_id" }>) {
  if (deal.masked_fields?.includes(field)) {
    return <FieldGuard mode="masked" />;
  }
  const id = deal[field];
  if (!id) {
    return null;
  }
  return <EntityRef kind="organization" id={id} asText />;
}

// The amount's three readings, in one place because two tables draw them: the
// native list and the overlay mirror. Withheld is not empty — the mirror
// carries the same `masked_fields` the native list does, and reading a refused
// amount as an unpriced deal is the defect this cell exists to prevent.
function AmountCell({
  deal,
  locale,
}: Readonly<{ deal: Deal; locale: Locale }>) {
  if (deal.masked_fields?.includes("amount_minor")) {
    return <FieldGuard mode="masked" />;
  }
  if (deal.amount_minor == null || !deal.currency) {
    return null;
  }
  return (
    <span className="t-mono">
      {formatMoney(deal.amount_minor, deal.currency, locale)}
    </span>
  );
}

// The table-view column set. Module-level (not inlined in DealsScreen,
// which is already at the cognitive-complexity ceiling) — stage_id → name
// and amount/close formatting are the only per-row logic, everything else
// is direct field access. Only amount_minor and expected_close_date are in
// the deals list's sortable vocabulary (data-model.md DM-VOCAB-3); name,
// stage and status carry no `sort` because the API has no column for them.
function dealColumns(
  t: ReturnType<typeof useT>,
  locale: Locale,
  recordZone: string,
  stageName: Map<string, string>,
): ListColumn<Deal>[] {
  return [
    {
      key: "name",
      header: t("people.name"),
      cell: (deal) => deal.name,
      fixed: true,
    },
    tagsColumn<Deal>(t),
    {
      // The company the deal is with. Withheld is not empty: the wire sends a
      // null `organization_id` and names the field in `masked_fields` when the
      // reader may not read that company, and a blank cell cannot be told
      // apart from a deal nobody has linked.
      //
      // No `sort`, for the reason the partner column below carries none: the
      // API's sortable vocabulary does not include it, and a header that
      // looked sortable and refused would be worse than one that never
      // offered.
      key: "company",
      header: t("create.organization"),
      cell: (deal) => <CompanyCell deal={deal} field="organization_id" />,
    },
    {
      // Which partner brought the deal, when one did. Optional: a workspace
      // that runs no partner programme has an empty column, and hiding it
      // per-row is worse in a list than an empty cell — a column that comes
      // and goes cannot be scanned down.
      //
      // It carries no `sort`, because the API's sortable vocabulary is a fixed
      // five-field set that does not include it. That limitation is not this
      // column's to fix (see the sorting issue), and a header that looked
      // sortable and refused would be worse than one that never offered.
      key: "partner",
      header: t("deal.partnerOrg"),
      cell: (deal) => <CompanyCell deal={deal} field="partner_org_id" />,
    },
    {
      key: "stage",
      header: t("deals.stage"),
      // stage_id is null for an overlay-mirror deal (OVA-MAP-6) — no native
      // stage row to name; a native deal always has one.
      cell: (deal) =>
        deal.stage_id ? (stageName.get(deal.stage_id) ?? "") : "",
    },
    {
      key: "amount",
      header: t("deals.amount"),
      numeric: true,
      sort: "amount_minor",
      cell: (deal) => <AmountCell deal={deal} locale={locale} />,
    },
    {
      key: "close",
      header: t("deals.close"),
      sort: "expected_close_date",
      cell: (deal) =>
        deal.expected_close_date
          ? formatDate(deal.expected_close_date, locale, recordZone)
          : null,
    },
    {
      // How long since anything happened on this deal. It is the figure a
      // forecast argument rests on — an amount with no recent signal behind it
      // is a number nobody can defend — and the server already flags the ones
      // that have gone quiet, so the row says so rather than leaving the reader
      // to subtract dates.
      key: "last_signal",
      header: t("deals.lastSignal"),
      numeric: true,
      sort: "last_activity_at",
      cell: (deal) =>
        deal.last_activity_at ? (
          <span className="deal-signal">
            {formatDuration(
              Math.max(
                0,
                Date.now() - new Date(deal.last_activity_at).getTime(),
              ),
              locale,
            )}
            {deal.stalled && <Badge tone="warn">{t("deal.stalled")}</Badge>}
          </span>
        ) : (
          <span className="t-caption">{t("deals.lastSignalNone")}</span>
        ),
    },
    {
      key: "status",
      header: t("lead.status"),
      cell: (deal) => (
        <Badge tone={dealStatusTone(deal.status)}>{deal.status}</Badge>
      ),
    },
  ];
}

/**
 * The closed vocabulary for winning a deal with no contract behind it.
 *
 * The type comes from the generated contract, and the labels are a Record over
 * it, so adding a member to `crm.yaml` stops this file compiling until the new
 * member has a label — rather than leaving a choice the server accepts and no
 * screen offers.
 */
type WonReason = NonNullable<
  NonNullable<
    components["schemas"]["AdvanceDealRequest"]["won_without_contract_reason"]
  >
>;

const WON_REASON_LABELS: Record<WonReason, MessageKey> = {
  purchase_order: "deals.winReasonPurchaseOrder",
  verbal: "deals.winReasonVerbal",
  renewal_by_email: "deals.winReasonRenewalByEmail",
  imported: "deals.winReasonImported",
  other: "deals.winReasonOther",
};

// Display order — deliberately not the contract's, which is a storage list.
// This one puts the answers a rep reaches for first. The Record above is what
// guarantees the set is complete; this only decides the sequence.
const WON_REASONS: readonly WonReason[] = [
  "purchase_order",
  "verbal",
  "renewal_by_email",
  "imported",
  "other",
];

// Narrows the Select's plain string back to the vocabulary. The control is
// built from WON_REASONS, so this never rejects in practice — but a cast would
// make that an assumption instead of a check, and the value goes on to be
// stored as an assertion about how a deal closed.
function asWonReason(value: string): WonReason | "" {
  const known = WON_REASONS.find((reason) => reason === value);
  return known ?? "";
}

/**
 * Whether a detail carries any visible character, matching the rule the server
 * applies (`saysSomething` in win_evidence.go).
 *
 * `trim()` alone is not the same test. A zero-width space is not whitespace to
 * either language, so a detail of "​" would pass a trim check here, enable
 * Confirm, and then be refused by the server — the reader having explained
 * precisely nothing, which is the state the vocabulary exists to prevent.
 */
function saysSomething(text: string): boolean {
  return /\P{White_Space}/u.test(text.replace(/\p{Cf}/gu, ""));
}

// The one member that explains nothing on its own, so the server demands a
// detail after it (`WonReasonDetailRequiredError`).
const WON_REASON_NEEDING_DETAIL: WonReason = "other";

// The server's refusal when a win names neither a contract nor a reason. The
// dialog keys on THIS rather than on the 422 status: an advance can be refused
// for reasons that have nothing to do with evidence, and asking "how was it
// won?" after a version conflict would be nonsense.
const WIN_EVIDENCE_REQUIRED = "win_evidence_required";

type PendingAdvance = {
  dealId: string;
  // Carried through the confirm rather than looked up when it closes: the write
  // pins the deal as it stood on the board the reader dropped it on, so a stage
  // change made while the dialog was open fails loud instead of being erased.
  version: number | undefined;
  toStage: Stage;
};

type AdvanceInput = {
  dealId: string;
  version: number | undefined;
  toStage: Stage;
  lostReason?: string;
  // Why this win has no signed contract behind it. Absent on the ordinary win,
  // where the contract IS the answer — the server distinguishes the two, and
  // that distinction is what makes "how many won deals have no paper" a
  // question reports can answer.
  wonWithoutContractReason?: WonReason;
  wonWithoutContractDetail?: string;
};

/**
 * The ONE way this screen advances a deal, shared by the board's drag and the
 * record page's stepper.
 *
 * An advance is a write like any other, so it is pinned like any other: the
 * version the reader's own card or record was drawn from rides the variables,
 * and two people moving one deal at the same moment no longer both succeed —
 * the second reads the version the first replaced and fails 409 version_skew
 * instead of quietly undoing a stage change nobody saw.
 */
/**
 * What a terminal advance says about HOW the deal closed, on top of the stage
 * and status every advance carries.
 *
 * A lost deal states its reason. A won deal states one only when there is no
 * signed contract to point at — the server looks for the contract first, and a
 * win that has one says nothing here, which is what keeps "won with paper" and
 * "won without it" distinguishable in reports.
 */
function closingFields(input: AdvanceInput) {
  if (input.toStage.semantic === "lost") {
    return { lost_reason: input.lostReason };
  }
  if (input.toStage.semantic === "won" && input.wonWithoutContractReason) {
    return {
      won_without_contract_reason: input.wonWithoutContractReason,
      won_without_contract_detail: input.wonWithoutContractDetail,
    };
  }
  return {};
}

function useAdvanceDeal() {
  const toast = useToast();
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    // No single record: one instance serves every card on the board, and which
    // deal is moving is known at mutate() time rather than here, so the agent
    // rail names the write without naming a record (app/agentrail-copy.ts).
    mutationKey: ["deal-edit"],
    mutationFn: async (input: AdvanceInput) => {
      const terminal = input.toStage.semantic !== "open";
      const { data, error } = await api.POST("/deals/{id}/advance", {
        params: {
          path: { id: input.dealId },
          ...ifMatch(requireVersion(input.version)),
        },
        body: {
          to_stage_id: input.toStage.id,
          ...(terminal ? { status: input.toStage.semantic } : {}),
          ...closingFields(input),
        },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (deal, input) => {
      // The advanced deal goes into the cache SYNCHRONOUSLY, before the
      // refetch the invalidation schedules. isPending clears the moment this
      // returns, so a reader who clicks a second stage immediately would
      // otherwise pin the version this write just replaced and read a 409 they
      // did not cause.
      if (deal) {
        queryClient.setQueryData(["deal", input.dealId], deal);
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
      for (const queryKey of dealRecordKeys(input.dealId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      // A win moves the deal's project into delivery in the same server
      // write, so the project page and list are stale the moment this
      // returns. Without this a reader who follows the project chip within
      // the 30s stale window reads a won deal on a project still being
      // pursued — the contradiction the server's one-transaction move exists
      // to prevent.
      for (const queryKey of dealWinKeys(deal)) {
        queryClient.invalidateQueries({ queryKey });
      }
      toast.show(t("deals.advanced", { stage: input.toStage.name }));
    },
  });
}

// Won reads success, lost reads danger, an open deal carries no status tone.
// Exported so the partner page's sourced-deals panel reads a deal's status the
// same way the board and the deal record do — a second mapping is how the same
// status came to render in two colours on two screens.
export function dealStatusTone(
  status: Deal["status"],
): "success" | "danger" | undefined {
  if (status === "won") {
    return "success";
  }
  if (status === "lost") {
    return "danger";
  }
  return undefined;
}

// Bespoke selects for the filters whose option labels are runtime strings
// (pipeline/stage/org names) — a chip's option label is a MessageKey, so
// these three cannot go through ListTable's chips. Each writes into the
// same ListQuery.filters bag the table's chips read, deleting the key on a
// blank choice so the two stay in one coherent query state.
function setOrClearFilter(
  setQuery: Dispatch<SetStateAction<ListQuery>>,
  key: string,
  value: string,
) {
  setQuery((q) => {
    const next = { ...q.filters };
    if (value) {
      next[key] = value;
    } else {
      delete next[key];
    }
    return { ...q, filters: next };
  });
}

// The company filter's own value source: a workspace holds more organizations
// than any fixed list should offer, so the value step searches /organizations
// by name instead of one this screen happened to fetch for something else.
async function searchCompanies(
  query: string,
): Promise<readonly { value: string; label: string }[]> {
  const { data, error } = await api.GET("/organizations", {
    params: { query: { q: query, limit: 20 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((org) => ({ value: org.id, label: org.display_name }));
}

// Whether the reader has narrowed this list themselves.
//
// The same question `SaveViewAction` asks before it offers to save, asked here
// because the pipeline has to be folded into the saved query WITHOUT being the
// thing that makes the query look narrowed: a pipeline is always selected, so
// counting it would offer to save the default list.
function narrowsTheDealList(query: ListQuery): boolean {
  return (
    Boolean(query.q) ||
    Boolean(query.sort) ||
    query.includeArchived ||
    Object.values(query.filters).some(Boolean)
  );
}

// The stage and company filters. The stage list is loaded whole already (a
// pipeline has few stages), so it stays a fixed chip; the company filter
// searches rather than listing (see searchCompanies above). Both are still
// filters, so they read as the same chip as every other one instead of as a
// native select sitting among them.
function dealFilterChips(
  stages: Stage[],
  t: ReturnType<typeof useT>,
): ListChip[] {
  return [
    {
      key: "stage_id",
      label: t("deals.stage"),
      allLabel: t("deals.filterStageAll"),
      options: stages.map((stage) => ({ value: stage.id, label: stage.name })),
    },
    {
      key: "organization_id",
      label: t("create.organization"),
      allLabel: t("deals.filterOrgAll"),
      options: [],
      search: searchCompanies,
    },
  ];
}

// How the deals are shown rather than which ones: the board/table switch and
// the pipeline being looked at. Both sit on the right with the table's own
// display controls. All board-only dials the overlay mirror refuses, so
// overlay never calls this.
function DealViewTools({
  view,
  setView,
  pipelines,
  pipelineId,
  setPipelineId,
  setQuery,
}: Readonly<{
  view: "board" | "table";
  setView: (v: "board" | "table") => void;
  pipelines: Pipeline[];
  pipelineId: string;
  setPipelineId: (id: string) => void;
  setQuery: Dispatch<SetStateAction<ListQuery>>;
}>) {
  const t = useT();
  return (
    <>
      <SegmentedControl
        options={["board", "table"] as const}
        value={view}
        onChange={setView}
        labels={{
          board: t("deals.viewBoard"),
          table: t("deals.viewTable"),
        }}
      />
      {/* Both views read one pipeline: the table binds the same query the
          board does, and its stage chip offers that pipeline's stages. So the
          picker stands in both — hidden on the table it locked the reader to a
          pipeline they could neither see nor change.
          A CHOICE OF ONE IS NOT A CHOICE. An installation with a single
          pipeline was offered a menu whose only entry was the pipeline already
          showing, which is a dial that refuses every press. The NAME still
          stands, as text: the reader keeps the fact and loses only the control,
          which is what they lost nothing by. */}
      {pipelines.length > 1 ? (
        <Select
          className="input"
          aria-label={t("deals.pipeline")}
          placeholder={t("deals.pipeline")}
          value={pipelineId}
          onChange={(next) => {
            // A stage belongs to one pipeline; switching pipeline strands any
            // stage_id filter (the chip blanks out but useDeals would still
            // forward the old id and filter a foreign stage → 0 rows).
            setPipelineId(next);
            setOrClearFilter(setQuery, "stage_id", "");
          }}
          options={pipelines.map((pipeline) => ({
            value: pipeline.id,
            label: pipeline.name,
          }))}
        />
      ) : (
        pipelines.length === 1 && (
          <span className="lt-scope">
            {/* The reader SEES a name where the dial was and infers what it
                names from the place; a screen reader gets no place, so the
                field says its own name to it. */}
            <span className="sr-only">{`${t("deals.pipeline")}: `}</span>
            {pipelines[0].name}
          </span>
        )
      )}
    </>
  );
}

// The board surface's body, drawn only while the board is showing.
//
// Its own component because the choice it makes is three deep — a pipeline to
// draw in, a first load that has not landed, then the board itself — and read
// inside the screen that choice sat under every other dial the screen holds.
function DealBoardBody({
  dealsQuery,
  pipelinesQuery,
  effectivePipeline,
  loadedDeals,
  stageTotalsQuery,
  orgs,
  orgsSettled,
  openDeal,
  cardDragHandlers,
  columnDropHandlers,
}: Readonly<{
  dealsQuery: ReturnType<typeof useDeals>;
  pipelinesQuery: ReturnType<typeof usePipelines>;
  effectivePipeline?: Pipeline;
  loadedDeals: Deal[];
  stageTotalsQuery: ReturnType<typeof useStageTotals>;
  orgs: Organization[];
  orgsSettled: boolean;
  openDeal: ComponentProps<typeof PipelineBoard>["onOpen"];
  cardDragHandlers: ComponentProps<typeof PipelineBoard>["cardDragHandlers"];
  columnDropHandlers: ComponentProps<
    typeof PipelineBoard
  >["columnDropHandlers"];
}>) {
  const t = useT();
  // Every company the CARDS name. The picker's capped page answers most of them
  // for free; the rest are resolved by id (useOrgMarks), so no card is left
  // standing over a company the board simply failed to look up.
  //
  // Only the board asks: the table names its companies through the same
  // per-record reference every other cross-record cell uses, and handing it
  // this map as well would read each company twice.
  const orgMarks = useOrgMarks(loadedDeals, orgs, orgsSettled);
  return (
    <>
      <QueryGate query={pipelinesQuery} pendingLabel={t("nav.deals")}>
        {() =>
          effectivePipeline ? (
            // Only the INITIAL load goes through the gate. An infinite
            // query reports isError when ANY page fails, later ones
            // included, so keeping the gate around a loaded board would
            // let one failed "load more" throw away every card already on
            // screen. Past the first page the board stands and the button
            // retries — exactly what OverlayDealsTable does above, and for
            // the same reason.
            (dealsQuery.data?.pages ?? []).length === 0 ? (
              <QueryGate query={dealsQuery} pendingLabel={t("nav.deals")}>
                {() => null}
              </QueryGate>
            ) : (
              <>
                <PipelineBoard
                  cardHref={(deal) =>
                    routeHash({ screen: "deals", id: deal.id })
                  }
                  columns={buildColumns(
                    effectivePipeline.stages ?? [],
                    loadedDeals,
                    stageTotalsQuery.data ?? new Map(),
                    orgMarks,
                  )}
                  onOpen={openDeal}
                  cardDragHandlers={cardDragHandlers}
                  columnDropHandlers={columnDropHandlers}
                />
                <LoadMoreButton query={dealsQuery} />
              </>
            )
          ) : null
        }
      </QueryGate>
    </>
  );
}

// Everything the board does when it is touched: the drag it started, the drop
// it landed, and the click that is a click rather than the tail of a drag.
//
// A hook rather than four callbacks in the screen. They share the two refs
// that tell a drag from a click and nothing else on the screen reads those,
// so holding them out here is what keeps the drag protocol readable in one
// place instead of interleaved with the screen's dials.
function useBoardInteractions({
  stages,
  loadedDeals,
  advance,
  setPending,
}: Readonly<{
  stages: Stage[];
  loadedDeals: Deal[];
  advance: ReturnType<typeof useAdvanceDeal>;
  setPending: (pending: PendingAdvance) => void;
}>) {
  // Which card is in flight, and WHICH card a drag last ended on, with when —
  // a drop and a click arrive as the same event pair, so the board tells them
  // apart by time. The id travels with the timestamp because the window is
  // board-wide otherwise: a reader who drops one card and reaches straight for
  // another would have that second click swallowed, and a link that does
  // nothing is indistinguishable from a broken one.
  const dragging = useRef<string | null>(null);
  const lastDrop = useRef<{ dealId: string; at: number } | null>(null);

  const requestAdvance = (dealId: string, stageId: string) => {
    const toStage = stages.find((stage) => stage.id === stageId);
    if (!toStage) {
      return;
    }
    // The version the reader saw. The cards and this lookup read the one deal
    // array this render was handed, so the precondition names the row as it was
    // drawn on the board rather than whatever it has become since — which is the
    // whole claim optimistic concurrency makes.
    const version = loadedDeals.find((deal) => deal.id === dealId)?.version;
    if (toStage.semantic === "open") {
      advance.mutate({ dealId, version, toStage });
    } else {
      // Terminal-stage advance is a 🟡 confirm (AC-deal-6).
      setPending({ dealId, version, toStage });
    }
  };

  // Board interactions are hoisted here so the render-prop tree below doesn't
  // nest their event callbacks past the readable depth.
  // The card is a LINK now, so this no longer navigates — the href does. What
  // is left is the one thing a link cannot know: the click that ends a drag is
  // not a click on the card, and following it would open a deal the reader was
  // only moving.
  const openDeal = (deal: BoardDeal, event: MouseEvent) => {
    const drop = lastDrop.current;
    if (drop?.dealId === deal.id && Date.now() - drop.at <= 250) {
      event.preventDefault();
    }
  };

  const cardDragHandlers = (deal: BoardDeal) => ({
    draggable: true as const,
    onDragStart: (event: DragEvent) => {
      dragging.current = deal.id;
      event.dataTransfer.setData("text/plain", deal.id);
    },
  });

  const columnDropHandlers = (column: BoardColumn) => ({
    onDragOver: (event: DragEvent) => {
      event.preventDefault();
      (event.currentTarget as HTMLElement).classList.add("droptarget");
    },
    onDragLeave: (event: DragEvent) => {
      (event.currentTarget as HTMLElement).classList.remove("droptarget");
    },
    onDrop: (event: DragEvent) => {
      event.preventDefault();
      (event.currentTarget as HTMLElement).classList.remove("droptarget");
      const dealId =
        event.dataTransfer.getData("text/plain") || dragging.current;
      dragging.current = null;
      if (dealId) {
        lastDrop.current = { dealId, at: Date.now() };
        requestAdvance(dealId, column.stage);
      }
    },
  });

  return { openDeal, cardDragHandlers, columnDropHandlers };
}

// Every dial on this screen, read from and written to the ADDRESS.
//
// A hook of its own because this screen hand-rolls what screens/listquery.tsx
// does for every other list — it drives a board as well as a table, so it
// reaches for the same codec rather than growing a second answer to what a
// narrowed list's URL looks like — and the hand-rolling is the part worth
// reading in one piece.
//
// `pipeline_id` and `view` sit beside the query's own dials: the first is
// already a wire parameter name, and the second is the one dial here that is
// about DRAWING rather than about which deals exist.
function useDealScreenDials({
  overlay,
  pipelines,
}: Readonly<{ overlay: boolean; pipelines?: Pipeline[] }>) {
  const [params, setParams] = useUrlParams();
  const opening = useMemo<ListQuery>(
    () => ({
      q: "",
      sort: "",
      includeArchived: false,
      filters: {},
      // The deal list reads its own fixed page (the board is capped at 100 and
      // documented as such), so this is the shape's default rather than a dial
      // the footer offers.
      perPage: LIST_PAGE_SIZES[0],
    }),
    [],
  );
  const query = useMemo(
    () =>
      listQueryFromParams(
        withoutScreenDials(params, DEAL_SCREEN_DIALS),
        opening,
        true,
      ),
    [params, opening],
  );
  const setQuery = (update: SetStateAction<ListQuery>) => {
    const live = currentParams();
    const next =
      typeof update === "function"
        ? update(
            listQueryFromParams(
              withoutScreenDials(live, DEAL_SCREEN_DIALS),
              opening,
              true,
            ),
          )
        : update;
    setParams(
      mergeScreenDials(
        // The rendered page is carried across rather than dropped, exactly as
        // useListQuery carries it: the codec does not compute it, so a write
        // that rebuilt the address from the query alone would take the page out
        // of an address a reader had been sent to. A write that really is a
        // narrowing still resets it — the table's own reset fires next.
        withListPage(paramsFromListQuery(next, opening), listPageOf(live)),
        live,
        DEAL_SCREEN_DIALS,
      ),
    );
  };
  const pipelineId = params.get(PIPELINE_PARAM) ?? "";
  const setPipelineId = (next: string) =>
    setParams(withDialSet(currentParams(), PIPELINE_PARAM, next));
  const effectivePipeline: Pipeline | undefined =
    pipelines?.find((p) => p.id === pipelineId) ??
    pipelines?.find((p) => p.is_default) ??
    pipelines?.[0];
  const dealFilters: DealFilters = {
    pipelineId: effectivePipeline?.id ?? "",
    sort: query.sort,
    includeArchived: query.includeArchived,
    filters: query.filters,
    overlay,
  };

  const view: "board" | "table" =
    overlay || params.get(VIEW_PARAM) === "table" ? "table" : "board";
  const setView = (next: "board" | "table") =>
    setParams(
      withDialSet(currentParams(), VIEW_PARAM, next === "table" ? next : ""),
    );

  return {
    query,
    setQuery,
    pipelineId,
    setPipelineId,
    effectivePipeline,
    dealFilters,
    view,
    setView,
  };
}

// The surface's own narrowing chips, beside the stage and company ones
// dealFilterChips builds.
//
// A function because two of the four are CONDITIONAL, and the condition is
// the interesting part of each: a chip offered before its options are known
// reads as "clear this filter" to the table, and a chip withdrawn while its
// filter is applied leaves the list narrowed with no dial to clear it.
function dealSurfaceChips({
  me,
  partnerOptions,
  partnerApplied,
}: Readonly<{
  me?: ReturnType<typeof useMe>["data"];
  partnerOptions: { value: string; label: string }[];
  partnerApplied?: string;
}>): FilterSpec[] {
  return [
    {
      key: "stalled",
      label: "deals.filterStalled",
      allLabel: "deals.filterStalledAll",
      options: [{ value: "true", label: "deals.filterStalled" }],
    },
    // Offered only once the viewer's own id is known. An option whose
    // value is still "" reads as "clear this filter" to the table, so
    // picking "Only mine" mid-load would quietly narrow nothing.
    ...(me
      ? [
          {
            key: "owner_id",
            label: "deals.filterOwnerMe" as const,
            allLabel: "deals.filterOwnerAll" as const,
            options: [
              {
                value: me.user.id,
                label: "deals.filterOwnerMe" as const,
              },
            ],
          },
        ]
      : []),
    {
      key: "partner_sourced",
      label: "deals.filterPartnerSourced",
      allLabel: "deals.filterPartnerAll",
      options: [{ value: "true", label: "deals.filterPartnerSourced" }],
    },
    // The forecast's own buckets, in the order the tiles read them. Four, not
    // five: `slipped` is derived by the report from a claimed category and a
    // close date, so there is no column to filter on and a chip offering it
    // would narrow to nothing.
    //
    // Unconditional, unlike the two above: the vocabulary is the schema's, so
    // there is no moment when the options are not yet known.
    {
      key: "forecast_category",
      label: "deals.filterForecast",
      allLabel: "deals.filterForecastAll",
      options: [
        { value: "commit", label: "deal.fcCommit" },
        { value: "best_case", label: "deal.fcBestCase" },
        { value: "pipeline", label: "deal.fcPipeline" },
        { value: "omitted", label: "deal.fcOmitted" },
      ],
    },
    // Which partner, not just whether there is one. Absent entirely
    // when the installation has made no company a partner: a picker
    // with nothing in it asks a question that has no answers, the same
    // rule the deal form's own partner fields follow.
    //
    // The options come from usePartnerOptions, so a partner whose
    // company this reader cannot open is not offered — picking it
    // would name a company the screen could not then show them.
    // Present whenever there are partners to pick OR one is already
    // applied. A saved view can restore a partner_org_id after the
    // programme was wound down or while the options are still in
    // flight, and hiding the chip then would leave the list narrowed
    // by a filter with no dial to see or clear it.
    ...(partnerOptions.length > 0 || partnerApplied
      ? [
          {
            key: "partner_org_id" as const,
            label: "deals.filterPartner" as const,
            allLabel: "deals.filterPartnerAnyOne" as const,
            // `text`, not `label`: a partner's name is the server's
            // data, not this screen's vocabulary, and FilterOption's
            // union exists for exactly that. Every other chip here
            // names a message key because its options are a fixed set
            // somebody wrote; a company name has nothing to translate.
            options: partnerOptions.map((option) => ({
              value: option.value,
              text: option.label,
            })),
          },
        ]
      : []),
  ];
}

// The create form, with the one question it has to ask the server: which
// projects the company named in the OPEN FORM is on.
//
// Its own component because that question is the form's and not the screen's.
// The company comes from the form's own values, so the state that holds it
// and the query that follows it belong beside the form rather than among the
// screen's dials.
function DealCreateAction({
  pipeline,
  cf,
  openStages,
  orgs,
  partnerOptions,
  startOpen,
}: Readonly<{
  pipeline?: Pipeline;
  // Read at screen level and handed down, so the schema request runs beside the
  // screen's own rather than waiting for the pipelines this form is gated on.
  cf: ObjectCustomFields;
  openStages: Stage[];
  orgs: Organization[];
  partnerOptions: { value: string; label: string }[];
  startOpen: boolean;
}>) {
  const t = useT();
  // The picker asks the server for ONE company's projects, so the form's chosen
  // company is what it is keyed on: a project is worked by several companies,
  // and only the server can say which of them this one is on.
  //
  // The company comes from the OPEN FORM rather than from anything on this
  // screen, because there is nothing else it could come from: a create form has
  // no record, and the reader picks the company inside the same dialog. The
  // form publishes its answers (CreateAction's onValuesChange) and the query
  // follows them, which is the read `optionsFor` cannot do — it is a pure
  // function of the values, so it can only filter a list already fetched, and a
  // project list row names only its anchor company, so nothing in the browser
  // can compute which projects a company is on.
  const [formCompany, setFormCompany] = useState("");
  const openProjects = useProjectsOfCompany(formCompany || undefined);

  const createDeal = async (values: Record<string, string>) => {
    if (!pipeline) {
      throwProblem(null);
    }
    // A project asked for on the form is born first, on the deal's company,
    // so the deal can name it at birth.
    const projectId = await resolveDealProject(
      values,
      values.organization_id?.trim() || null,
      t,
    );
    const { data, error } = await api.POST("/deals", {
      body: {
        ...mapDealCreate(
          { ...values, project_id: projectId ?? "" },
          pipeline.id,
        ),
        ...cf.toBody(values),
      },
    });
    if (error) {
      throwProblem(error, t);
    }
    return data;
  };

  return (
    <CreateAction
      label={t("create.deal")}
      invalidate="deals"
      screen="deals"
      create={createDeal}
      startOpen={startOpen}
      onValuesChange={(values) => setFormCompany(values.organization_id ?? "")}
      fields={[
        { key: "name", label: "create.dealName", required: true },
        { key: "amount", label: "create.amount", type: "number" },
        {
          key: "currency",
          label: "create.currency",
          type: "select",
          required: true,
          options: ["EUR", "USD", "GBP", "CHF"].map((code) => ({
            value: code,
            label: code,
          })),
        },
        {
          key: "stage_id",
          label: "create.stage",
          type: "select",
          required: true,
          options: openStages.map((stage) => ({
            value: stage.id,
            label: stage.name,
          })),
        },
        {
          key: "organization_id",
          label: "create.organization",
          type: "select",
          options: orgs.map((org) => ({
            value: org.id,
            label: org.display_name,
          })),
        },
        // The body of work this deal is about, chosen or started here: a
        // project begins during the deal, in its initiative phase.
        // Narrowed BY THE SERVER, for the company the form currently names:
        // the query above re-reads when the reader changes it, so the picker
        // repopulates instead of going empty.
        ...dealProjectFields(t, openProjects, undefined, formCompany),
        // A deal brought by a partner is attributed at birth, not by editing
        // it afterwards: the win that pays them can come before anybody thinks
        // to revisit the record.
        ...partnerFields(t, partnerOptions),
        {
          key: "expected_close_date",
          label: "create.expectedClose",
          type: "date",
        },
        ...cf.formFields,
      ]}
    />
  );
}

// The bulk-selection contract the table is handed, and the board is not.
//
// The row checkboxes live inside the grid's identity cell, and the board draws
// no rows to put one in. Offered while the board is showing, the bulk bar would
// name rows the reader can neither see nor deselect.
function dealRowSelection({
  view,
  liveSelection,
  selectedRows,
  stages,
  setSelected,
  t,
}: Readonly<{
  view: "board" | "table";
  liveSelection: ReadonlySet<string>;
  selectedRows: Deal[];
  stages: Stage[];
  setSelected: Dispatch<SetStateAction<ReadonlySet<string>>>;
  t: ReturnType<typeof useT>;
}>): ListSelection<Deal> | undefined {
  return view === "board"
    ? undefined
    : {
        selected: liveSelection,
        // A closed or archived deal takes no bulk write: archiving it is
        // done or meaningless, and moving it between open stages would be
        // the silent reopen the stepper already refuses.
        selectable: (deal) =>
          deal.archived_at == null && deal.status === "open",
        onToggle: (deal) =>
          setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(deal.id)) {
              next.delete(deal.id);
            } else {
              next.add(deal.id);
            }
            return next;
          }),
        label: (deal) => t("deals.bulkSelectRow", { name: deal.name }),
        bar: (
          <DealBulkBar
            deals={selectedRows}
            stages={stages}
            // The rows that went through leave the selection; the ones that
            // refused stay in it, named, so the reader can retry them once
            // the list has refetched their versions.
            onDone={(outcomes) =>
              setSelected(
                new Set(
                  outcomes
                    .filter((outcome) => outcome.error)
                    .map((outcome) => outcome.id),
                ),
              )
            }
          />
        ),
      };
}

// A saved view of this list, with the pipeline folded in.
//
// The pipeline goes in as a filter because it is the strongest dial on this
// screen and it lives in its own state, outside `query`. Left out, a view saved
// while looking at one pipeline would restore against whichever pipeline
// happened to be showing — a different list under the saved name.
//
// Folded in only once the reader has narrowed something else. A pipeline is
// always selected, so folding it in unconditionally would make every list look
// narrowed and offer to save the default view, which is the clutter
// SaveViewAction's own check exists to prevent.
function savableDealQuery(query: ListQuery, pipelineId: string): ListQuery {
  return narrowsTheDealList(query)
    ? { ...query, filters: { ...query.filters, pipeline_id: pipelineId } }
    : query;
}

// The dials that sit above whichever non-overlay surface is showing.
//
// The board/table switch and the pipeline picker are shared, so the reader
// sees the same dials either way. The SAVE action is not: a view holds a sort
// as well as its filters, and the board offers no way to see or change a sort
// — its order is the pipeline's stage order. Saving from there would pin an
// ordering the reader never chose into a view they will restore on the table.
function DealSurfaceTools({
  view,
  setView,
  pipelines,
  pipeline,
  setPipelineId,
  query,
  setQuery,
}: Readonly<{
  view: "board" | "table";
  setView: (next: "board" | "table") => void;
  pipelines: Pipeline[];
  pipeline?: Pipeline;
  setPipelineId: (next: string) => void;
  query: ListQuery;
  setQuery: (update: SetStateAction<ListQuery>) => void;
}>) {
  const pipelineId = pipeline?.id ?? "";
  const dials = (
    <DealViewTools
      view={view}
      setView={setView}
      pipelines={pipelines}
      pipelineId={pipelineId}
      setPipelineId={setPipelineId}
      setQuery={setQuery}
    />
  );
  if (view === "board") {
    return dials;
  }
  return (
    <>
      {dials}
      <SaveViewAction
        resource="deals"
        query={savableDealQuery(query, pipelineId)}
      />
    </>
  );
}

export function DealsScreen({
  startCreating = false,
}: Readonly<{ startCreating?: boolean }>) {
  const t = useT();
  const pageName = usePageName("deals");
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const cf = useObjectCustomFields("deal");
  const overlay = useSorMode() === "overlay";
  const pipelinesQuery = usePipelines(!overlay);
  const meQuery = useMe();
  const savedViews = useSavedViewTabs("deals");
  const {
    query,
    setQuery,
    setPipelineId,
    effectivePipeline,
    dealFilters,
    view,
    setView,
  } = useDealScreenDials({ overlay, pipelines: pipelinesQuery.data });
  const dealsQuery = useDeals(dealFilters);
  // The board's column totals: a per-stage server aggregate
  // over EVERY matching deal, not just the capped page useDeals fetches —
  // built from the SAME filter dials so cards and totals never disagree
  // about which deals are in view.
  const stageTotalsQuery = useStageTotals(dealFilters);
  // A stage-keyed board cannot place a mirror deal (its pipeline/stage is the
  // null pipeline/stage), so overlay mode opens on the flat table and hides the toggle
  // (below) — the mode is fixed for the page's life, so a static initial value
  // is enough.
  const [pending, setPending] = useState<PendingAdvance | null>(null);
  // Bulk selection, by deal id. Cleared after any bulk run except for the rows
  // that refused, since every other row's version has moved.
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());

  const advance = useAdvanceDeal();

  const stages = effectivePipeline?.stages ?? [];
  const stageName = new Map(stages.map((stage) => [stage.id, stage.name]));
  // Only ids the list currently holds count as selected: a row that left the
  // result set (refetched away, filtered out, archived by this very run) must
  // not linger as an invisible selection nobody can clear.
  // Every page walked so far, in one list. The board draws its columns from
  // this and the table renders it directly, so both surfaces grow together as
  // the reader asks for more.
  const loadedDeals = (dealsQuery.data?.pages ?? []).flatMap(
    (page) => page.data,
  );
  const selectedRows = loadedDeals.filter((deal) => selected.has(deal.id));
  const liveSelection = new Set(selectedRows.map((deal) => deal.id));

  // The table's own dials over the same keyset walk the board reads, so
  // "load more" on either surface advances both.
  const dealsListState: ListState<Deal> = {
    rows: loadedDeals,
    query,
    setQuery,
    isPending: dealsQuery.isPending,
    isError: dealsQuery.isError,
    error: dealsQuery.error,
    refetch: () => dealsQuery.refetch(),
    hasMore: dealsQuery.hasNextPage,
    loadMore: () => {
      dealsQuery.fetchNextPage();
    },
  };

  const orgsQuery = useQuery({
    queryKey: ["organizations"],
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations", {
        params: { query: { limit: 50 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const partnerOptions = usePartnerOptions(orgsQuery.data?.data ?? []);

  // Open-stage targets only: a deal is born open (INV-CLOSE-PAST twin rule);
  // won/lost are reached through the confirmed advance, never at create.
  const openStages = stages.filter((stage) => stage.semantic === "open");

  const { openDeal, cardDragHandlers, columnDropHandlers } =
    useBoardInteractions({ stages, loadedDeals, advance, setPending });

  // Create writes a native deal — the mirror refuses it (unsupported_by_sor),
  // so the affordance is hidden in overlay, matching the board mutations.
  // Shared between the board and the table surface: whichever view is
  // showing, the action lives in the surface's own header, not a wrap sibling.
  const createAction = !overlay && openStages.length > 0 && (
    <DealCreateAction
      pipeline={effectivePipeline}
      cf={cf}
      openStages={openStages}
      orgs={orgsQuery.data?.data ?? []}
      partnerOptions={partnerOptions}
      startOpen={startCreating}
    />
  );

  const tools = (
    <DealSurfaceTools
      view={view}
      setView={setView}
      pipelines={pipelinesQuery.data ?? []}
      pipeline={effectivePipeline}
      setPipelineId={setPipelineId}
      query={dealsListState.query}
      setQuery={setQuery}
    />
  );
  const dealChips = dealFilterChips(stages, t);
  const tagChips = useTagChips();
  const rowSelection = dealRowSelection({
    view,
    liveSelection,
    selectedRows,
    stages,
    setSelected,
    t,
  });

  // The board is the surface's alternate BODY, not a surface of its own. The
  // saved views, the chips and the archived toggle all describe the ONE query
  // both views read, and a board that replaced the surface took them with it —
  // leaving the reader looking at a narrowed pipeline with nothing on screen
  // saying what narrowed it, or how to widen it again.
  //
  // It carries its own continuation control, because `bodyOwnsPaging` withholds
  // the surface's: that belongs to the paged grid this body does not draw, and
  // the board holds every card loaded so far at once rather than a page of
  // them. Its COUNT goes to the surface's head all the same (`bodyCount`) — a
  // reader looks for how much is here beside the page's name, and under the
  // toolbar it read as a caption about the dials above it.
  const boardBody =
    view === "board" ? (
      <DealBoardBody
        dealsQuery={dealsQuery}
        pipelinesQuery={pipelinesQuery}
        effectivePipeline={effectivePipeline}
        loadedDeals={loadedDeals}
        stageTotalsQuery={stageTotalsQuery}
        orgs={orgsQuery.data?.data ?? []}
        orgsSettled={orgsQuery.isSuccess}
        openDeal={openDeal}
        cardDragHandlers={cardDragHandlers}
        columnDropHandlers={columnDropHandlers}
      />
    ) : undefined;

  // Overlay mode draws the flat, keyset-paginated mirror table — no pipeline
  // board and no stage columns, because a stage-keyed board cannot place a deal
  // whose pipeline and stage are null. It goes in the surface's own body slot,
  // the way the board does: it had been rendered beside the surface instead,
  // with a hand-rolled archived checkbox borrowing the list stylesheet's
  // private class, and so lost the count, the note saying why the dials are
  // missing, and every other piece of chrome a list on this product has.
  const overlayBody = overlay ? (
    <OverlayDealsTable includeArchived={query.includeArchived} />
  ) : undefined;

  return (
    <div className="wrap">
      <ListTable
        title={pageName}
        state={dealsListState}
        unit="deals.unit"
        columns={dealColumns(t, locale, recordZone, stageName)}
        rowKey={(deal) => deal.id}
        rowRoute={(deal) => ({ screen: "deals", id: deal.id })}
        searchable={false}
        action={createAction}
        tools={tools}
        body={overlayBody ?? boardBody}
        bodyOwnsPaging={overlay || view === "board"}
        bodyCount={
          view === "board" &&
          dealsQuery.data && (
            <>
              {t("board.count", {
                count: formatNumber(loadedDeals.length, locale),
              })}
            </>
          )
        }
        // The pipeline picker is screen state, not a filter, so switching it
        // changes every row without touching `filters`. Naming it here is
        // what puts the reader back on page 1.
        scopeKey={effectivePipeline?.id ?? ""}
        dataChips={[...dealChips, ...tagChips]}
        dataViews={savedViews}
        selection={rowSelection}
        chips={dealSurfaceChips({
          me: meQuery.data,
          partnerOptions,
          partnerApplied: query.filters.partner_org_id,
        })}
        views={[{ label: "deals.sortNewest", sort: "-created_at" }]}
      />
      {advance.isError && (
        <p
          className="t-caption"
          style={{ color: "var(--danger)", marginTop: "var(--space-2)" }}
        >
          {problemMessageOf(advance.error, t)}
        </p>
      )}
      <ConfirmAdvanceModal
        pending={pending}
        onClose={() => setPending(null)}
        onConfirm={(input) =>
          // mutateAsync REJECTS on failure; this dialog wants the outcome, and
          // an unhandled rejection in a click handler is not one. onError still
          // runs, so the screen's own error surface is unaffected.
          advance.mutateAsync(input).then(
            () => null,
            (error: unknown) => error,
          )
        }
      />
    </div>
  );
}

/**
 * The 🟡 confirm a terminal advance goes through (AC-deal-6), wherever the
 * advance was asked for — the board's drag or the record page's stepper.
 *
 * Closing a deal is the one stage move that cannot be undone by moving it
 * back, so the question is asked in ONE place: a second copy of this dialog is
 * how the two surfaces would end up disagreeing about whether a lost deal
 * needs a reason.
 */
function ConfirmAdvanceModal({
  pending,
  onClose,
  onConfirm,
}: Readonly<{
  pending: PendingAdvance | null;
  onClose: () => void;
  // Resolves when the advance settles, so this dialog acts on the outcome of
  // THIS attempt. It returns the error rather than throwing it: the caller's
  // own error surface still reports the failure, and a rejection here would be
  // an unhandled one in an event handler.
  onConfirm: (input: AdvanceInput) => Promise<unknown>;
}>) {
  const t = useT();
  const tierMap = useAgentTierMap();
  const [lostReason, setLostReason] = useState("");
  const [wonReason, setWonReason] = useState<WonReason | "">("");
  const [wonDetail, setWonDetail] = useState("");
  const [submitting, setSubmitting] = useState(false);
  // The deal this dialog has been told has no contract behind it, pinned to the
  // exact attempt that was refused.
  //
  // Read off the shared mutation's `error` instead, the refusal outlives the
  // deal that earned it: cancel here, open Won on a DIFFERENT deal, and the
  // reason panel greets a deal that may well have a contract — and the server
  // takes a stated reason at its word without looking for one, so that deal is
  // recorded as won-without-paper when it was not. That falsifies the exact
  // count the reason vocabulary exists to make truthful.
  const [refusedDealId, setRefusedDealId] = useState<string | null>(null);

  // EVERY way out of this dialog clears what was typed — the buttons, Escape,
  // and the backdrop alike. The component stays mounted between openings, so a
  // reason typed and then abandoned would otherwise still be sitting there the
  // next time a deal is closed, and it would describe a different deal.
  const dismiss = () => {
    setLostReason("");
    setWonReason("");
    setWonDetail("");
    setRefusedDealId(null);
    onClose();
  };

  const needsLostReason = pending?.toStage.semantic === "lost";
  // The reason panel appears only once the server has asked for it, and only
  // for the deal it asked about. A win with a signed contract is one click,
  // exactly as before: making every rep justify a win the paperwork already
  // explains is how a required field becomes a field everyone fills with the
  // same lie.
  const needsWonReason =
    pending?.toStage.semantic === "won" && refusedDealId === pending.dealId;
  const detailMissing =
    wonReason === WON_REASON_NEEDING_DETAIL && !saysSomething(wonDetail);

  return (
    <Modal open={pending !== null} onClose={dismiss} labelledBy="advance-title">
      {pending && (
        <>
          <p className="t-sub" id="advance-title">
            <AutonomyDot tier={verbTier("progress_deal", tierMap)} />{" "}
            {t("deals.confirmAdvance", { stage: pending.toStage.name })}
          </p>
          <p className="t-caption" style={{ marginTop: "var(--space-2)" }}>
            {t("deals.confirmTerminal", { status: pending.toStage.semantic })}
          </p>
          {needsLostReason && (
            <div className="field" style={{ marginTop: "var(--space-2)" }}>
              <span className="t-label" id="lost-reason-label">
                {t("deals.lostReason")}
              </span>
              <TextInput
                aria-labelledby="lost-reason-label"
                value={lostReason}
                onChange={(event) => setLostReason(event.target.value)}
              />
            </div>
          )}
          {needsWonReason && (
            <WonReasonFields
              reason={wonReason}
              detail={wonDetail}
              onReason={(next) => {
                setWonReason(next);
                // The detail belongs to "Something else" alone. Kept across a
                // change of reason it would sit invisibly behind a field the
                // reader can no longer see, which is not a state they can
                // correct.
                if (next !== WON_REASON_NEEDING_DETAIL) {
                  setWonDetail("");
                }
              }}
              onDetail={setWonDetail}
            />
          )}
          <div className="actions">
            <Button onClick={dismiss}>{t("deals.cancel")}</Button>
            <Button
              variant="primary"
              disabled={
                submitting ||
                (needsLostReason && lostReason.trim() === "") ||
                (needsWonReason && (wonReason === "" || detailMissing))
              }
              onClick={async () => {
                setSubmitting(true);
                const error = await onConfirm({
                  dealId: pending.dealId,
                  version: pending.version,
                  toStage: pending.toStage,
                  lostReason: lostReason.trim() || undefined,
                  ...wonAnswer(needsWonReason, wonReason, wonDetail),
                });
                setSubmitting(false);
                // ONE refusal keeps this dialog open: the server saying this
                // win names no evidence, because the answer to that is a field
                // the reader can fill in right here. Every other outcome closes
                // it — a success has nothing left to ask, and a 403 or a 409
                // has no answer this dialog can offer, so holding it open would
                // trap the reader behind a modal whose error text renders on
                // the screen underneath it.
                if (error && winEvidenceRefused(error)) {
                  setRefusedDealId(pending.dealId);
                  return;
                }
                dismiss();
              }}
            >
              {t("deals.confirm")}
            </Button>
          </div>
        </>
      )}
    </Modal>
  );
}

/**
 * The won-without-contract answer as the advance should carry it, or nothing.
 *
 * The detail rides ONLY with the reason that needs one. Sent alongside any
 * other reason it would be stored anyway — the server writes both columns as
 * given — so a reader who typed a detail under "Something else" and then chose
 * "On a purchase order" would leave text on the deal, and in its audit trail,
 * that they had every reason to believe they had discarded when the field
 * disappeared.
 */
function wonAnswer(active: boolean, reason: WonReason | "", detail: string) {
  if (!active || reason === "") {
    return {};
  }
  const explained = reason === WON_REASON_NEEDING_DETAIL;
  return {
    wonWithoutContractReason: reason,
    wonWithoutContractDetail: explained ? detail.trim() : undefined,
  };
}

// Whether a failed advance is the server asking how a contract-less deal was
// won. Keyed on the field code, not the 422: an advance is refused for several
// reasons, and only this one has an answer the reader can give here.
function winEvidenceRefused(error: unknown): boolean {
  return problemFieldErrorsOf(error).some(
    (fault) => fault.code === WIN_EVIDENCE_REQUIRED,
  );
}

// The reason a deal was won with no paper behind it: a closed vocabulary, plus
// the free-text detail the one open-ended member needs.
function WonReasonFields({
  reason,
  detail,
  onReason,
  onDetail,
}: Readonly<{
  reason: WonReason | "";
  detail: string;
  onReason: (value: WonReason | "") => void;
  onDetail: (value: string) => void;
}>) {
  const t = useT();
  return (
    <>
      <p className="t-caption" style={{ marginTop: "var(--space-2)" }}>
        {t("deals.winNoEvidence")}
      </p>
      <div className="field" style={{ marginTop: "var(--space-2)" }}>
        <span className="t-label" id="won-reason-label">
          {t("deals.winReason")}
        </span>
        <Select
          aria-labelledby="won-reason-label"
          placeholder={t("deals.winReasonPick")}
          value={reason}
          onChange={(value) => onReason(asWonReason(value))}
          options={WON_REASONS.map((option) => ({
            value: option,
            label: t(WON_REASON_LABELS[option]),
          }))}
        />
      </div>
      {reason === WON_REASON_NEEDING_DETAIL && (
        <div className="field" style={{ marginTop: "var(--space-2)" }}>
          <span className="t-label" id="won-detail-label">
            {t("deals.winReasonDetail")}
          </span>
          <TextInput
            aria-labelledby="won-detail-label"
            value={detail}
            onChange={(event) => onDetail(event.target.value)}
          />
        </div>
      )}
    </>
  );
}

/**
 * The flat deal table the overlay mirror is drawn as.
 *
 * No sort of its own. It holds the pages walked so far of a cursor keyed on
 * `external_id`, so ordering that subset would present an order the rest of the
 * set does not share — and the mirror answers 422 to every sort dial, so there
 * is no server order to ask for either. This table therefore draws rows in
 * cursor order and says nothing about sorting at all.
 */
function DealTable({ deals }: Readonly<{ deals: Deal[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();

  return (
    <div>
      <DataTable
        label={t("nav.deals")}
        columns={[
          {
            key: "name",
            header: t("people.name"),
            render: (deal: Deal) => deal.name,
          },
          {
            key: "stage",
            header: t("deals.stage"),
            // Always empty HERE, and named rather than looked up. This table
            // draws the overlay mirror and nothing else, and a mirror deal
            // carries no native pipeline or stage (OVA-MAP-6) — so the column
            // keeps the shape the native table has while having nothing of its
            // own to say. A stage map passed in to be read would only ever
            // answer for a row this table cannot hold.
            render: () => "",
          },
          {
            key: "amount",
            header: t("deals.amount"),
            render: (deal: Deal) => <AmountCell deal={deal} locale={locale} />,
          },
          {
            key: "close",
            header: t("deals.close"),
            render: (deal: Deal) =>
              deal.expected_close_date
                ? formatDate(deal.expected_close_date, locale, recordZone)
                : null,
          },
          {
            key: "status",
            header: t("lead.status"),
            render: (deal: Deal) => (
              <Badge tone={dealStatusTone(deal.status)}>{deal.status}</Badge>
            ),
          },
        ]}
        rows={deals}
        rowKey={(deal) => deal.id}
        onRowClick={(deal) => navigate({ screen: "deals", id: deal.id })}
      />
    </div>
  );
}

// The FX-converted base-currency sub-line (D-14): shown only when the deal
// carries a frozen fx_rate_to_base (won/lost deals freeze it at close; open
// deals in a non-base currency may not have one yet). Prop-driven and
// exported so a later Storybook task can render it without a live fetch.
export function FxLine({
  amountMinor,
  baseCurrency,
  fxRateToBase,
  fxRateDate,
  locale,
}: Readonly<{
  amountMinor: number | null;
  // The installation's own base currency, from its settings. Not a constant:
  // an installation whose base is not the euro was reading a euro sign over a
  // figure converted into something else, which is the one error a converted
  // figure must not make. Null while the settings read is in flight or refused
  // — an unnamed base is not a euro base.
  baseCurrency: string | null;
  fxRateToBase: string;
  fxRateDate: string | null;
  locale: Locale;
}>) {
  const t = useT();
  const recordZone = useRecordZone();
  // A deal carrying a rate but no amount converts to nothing, not to zero.
  const baseMinor =
    amountMinor == null ? null : Math.round(amountMinor * Number(fxRateToBase));
  return (
    <p className="t-caption">
      {t("deal.fxBase", {
        value: formatMoneyOrAbsent(baseMinor, baseCurrency, locale),
        rate: fxRateToBase,
        date: fxRateDate ? formatDate(fxRateDate, locale, recordZone) : "—",
      })}
    </p>
  );
}

// Reopens a won/lost deal back to an open-semantic stage — the same advance
// mutation shape the board drag uses, with status:"open" forced. Split out
// of DealActions for the same readability reason as the other header actions.
function ReopenAction({
  dealId,
  dealVersion,
  openStages,
  disabledReasonId,
}: Readonly<{
  dealId: string;
  // The version the header this button sits in was rendered from, so the reopen
  // pins the deal the reader was looking at. Stated by the caller rather than
  // read here: this action holds no query of its own to read a fresh one from,
  // and a fresh one would be the wrong answer anyway.
  dealVersion: number | undefined;
  openStages: Stage[];
  // The id of the sentence saying why this reopen is refused, when it is.
  // STATE-4a: a control blocked by the record's STATE rather than by a
  // permission stays visible and says why, because the reason is the
  // information and hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [stageId, setStageId] = useState<string | null>(null);
  const reopen = useMutation({
    mutationKey: ["deal-edit", dealId],
    // Stage and version both ride the variables: a version read out of the
    // closure would be the one from the render before this dialog opened, and a
    // reopen that pins the wrong version either fails for no reason the reader
    // can see or lands on a deal somebody else has since moved.
    mutationFn: async (input: {
      toStageId: string;
      version: number | undefined;
    }) => {
      const { data, error } = await api.POST("/deals/{id}/advance", {
        params: {
          path: { id: dealId },
          ...ifMatch(requireVersion(input.version)),
        },
        body: { to_stage_id: input.toStageId, status: "open" },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: () => {
      setOpen(false);
      for (const queryKey of dealRecordKeys(dealId)) {
        queryClient.invalidateQueries({ queryKey });
      }
      queryClient.invalidateQueries({ queryKey: ["deals"] });
    },
  });
  return (
    <>
      <Button
        small
        reasonId={disabledReasonId}
        data-testid="reopen-open"
        onClick={() => setOpen(true)}
      >
        {t("deal.reopen")}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy="reopen-title"
      >
        <p className="t-sub" id="reopen-title">
          {t("deal.reopenPick")}
        </p>
        <div
          style={{
            display: "flex",
            gap: "var(--space-2)",
            flexWrap: "wrap",
            margin: "var(--space-3) 0",
          }}
        >
          {openStages.map((s) => (
            <Button
              key={s.id}
              small
              aria-pressed={stageId === s.id}
              data-testid={`reopen-stage-${s.id}`}
              onClick={() => setStageId(s.id)}
            >
              {s.name}
            </Button>
          ))}
        </div>
        {reopen.isError && (
          <p className="t-caption" style={{ color: "var(--danger)" }}>
            {problemMessageOf(reopen.error, t)}
          </p>
        )}
        <div className="actions">
          <Button small onClick={() => setOpen(false)}>
            {t("deals.cancel")}
          </Button>
          <Button
            small
            variant="primary"
            data-testid="reopen-confirm"
            disabled={!stageId || reopen.isPending}
            onClick={() => {
              if (stageId) {
                reopen.mutate({ toStageId: stageId, version: dealVersion });
              }
            }}
          >
            {t("deal.reopenConfirm")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

// The edit form's project fields, in the two readings a stored project has.
// Named rather than inlined so `DealActions` stays under the complexity ceiling,
// and so the masked case reads as one decision.
function editProjectFields(
  t: (key: MessageKey) => string,
  opts: Readonly<{
    masked: readonly string[];
    openProjects: readonly Project[];
    currentProject?: { id: string; label: string };
    company?: string;
  }>,
): CreateField[] {
  if (opts.masked.includes("project_id")) {
    // One entry, naming the field as withheld — the same reading the company
    // and partner pickers carry, for the same reason. Dropping the field said
    // nothing at all, and a form with no project row reads as a deal on no
    // project rather than one this reader was not shown. `mapDealUpdate`
    // leaves the field out of the patch either way, and neither a project to
    // pick nor the "start a new one" option is offered: both would re-point
    // the deal off a project the reader never saw.
    return [
      {
        key: "project_id",
        label: "deal.project",
        type: "select",
        options: [{ value: "", label: t("deal.projectWithheld") }],
      },
    ];
  }
  return dealProjectFields(
    t,
    opts.openProjects,
    opts.currentProject,
    opts.company,
  );
}

// The two people surfaces under the deal's overview.
//
// The seats are in the RAIL as context — who these people are. The map is in
// the column because it is a working surface: it draws how the deal is threaded
// and where the cover is missing, which is what a reader acts on.
//
// The stakeholders panel is where a seat is ADDED, changed and removed. The
// rail's seats and the map both read the coverage view, which carries no
// relationship id and so can carry no verb — a deal's stakeholders were
// readable on three surfaces and writable on none of them, reachable only from
// whichever person happened to already be linked. It is the generic
// relationships panel under a deal scope, not a second one: create, edit and
// remove have one implementation for every kind.
//
// Named rather than inlined so DealScreen's render callback stays under the
// complexity ceiling.
function DealPeoplePanels({
  dealId,
  overlay,
}: Readonly<{
  dealId: string;
  overlay: boolean;
}>) {
  // Out in overlay mode, where the deal is a mirror with no native row for a
  // relationship to point at. The committee map is not here any more: it is
  // one half of the reading's reference pair, beside the offers.
  if (overlay) {
    return null;
  }
  return (
    <div style={{ marginTop: "var(--space-4)" }}>
      <RelationshipsTab scope={{ deal_id: dealId }} />
    </div>
  );
}

// This deal's VERBS — split out of DealScreen's render so the record-view
// callback stays readably small. An archived deal is read-only (no
// edit/archive/advance path exists server-side for a non-live row), so its
// verbs render REFUSED rather than missing: the page's one sentence about the
// archive says why, and each of them points at it (STATE-4a). A missing control
// says nothing about the deal, while a refused one names the reason.
//
// They used to ride the record view's BADGES slot, which is where a record says
// what it IS rather than what can be done to it — so the deal page passed
// `actionsInline` with no `actions` to place, and four buttons sat in the row
// meant for a status and a project chip. Edit leads because it is the verb a
// reader reaches for; the three whose consequence has to be read before they
// are pressed go behind the overflow.
// The shared Email verb every record header carries. Not in overlay, where
// the mirror owns the deal's mail.
function DealEmailVerb({
  deal,
  overlay,
  disabledReasonId,
}: Readonly<{ deal: Deal; overlay: boolean; disabledReasonId?: string }>) {
  // The same coverage read the readings band and the coverage card already
  // make, served from one cache entry — so asking here costs no request. Off
  // in overlay for the reason the hook documents: a mirrored deal's coverage
  // cannot be assembled, and a doomed fetch reads as "nobody is on this deal".
  const coverage = useDealCoverage(deal.id, !overlay);
  const recordAddress = useDealRecipientAddress(coverage);
  if (overlay) {
    return null;
  }
  return (
    <RecordEmailVerb
      entityType="deal"
      entityId={deal.id}
      // Who a FIRST message on this deal goes to: the champion, else somebody
      // the deal is actually in conversation with, else the first seat. Only
      // ever an offer — the composer fills an empty To field once and never
      // over what the reader typed.
      recordAddress={recordAddress}
      disabledReasonId={disabledReasonId}
    />
  );
}

function DealActions({
  deal,
  orgs,
  meId,
  openStages,
  archivedReasonId,
}: Readonly<{
  deal: Deal;
  orgs: { id: string; display_name: string }[];
  meId: string;
  openStages: Stage[];
  // The id of the page's sentence about this deal being archived. Every verb
  // the archive refuses points at that one element instead of printing the
  // same line four times.
  archivedReasonId: string;
}>) {
  const t = useT();
  const cf = useObjectCustomFields("deal");
  // Reads the same cached partner list the deals list built, so opening Edit
  // costs no extra request.
  const partnerOptions = usePartnerOptions(orgs);
  const masked = deal.masked_fields ?? [];
  // The company this deal names, resolvable whether or not the picker's capped
  // page reached it. The page answers first; only a company it does not carry
  // is read by id, through the SAME cache entry the subtitle's own reference
  // already fills, so the common case costs nothing.
  const companyOnPage = orgs.find((org) => org.id === deal.organization_id);
  const companyById = useEntityName(
    "organization",
    companyOnPage ? null : deal.organization_id,
  );
  const currentCompany = deal.organization_id
    ? {
        id: deal.organization_id,
        // The raw id is the floor rather than the aim: ugly, and still better
        // than a blank picker whose save clears the company nobody touched.
        label:
          companyOnPage?.display_name ??
          companyById.name ??
          deal.organization_id,
      }
    : undefined;
  // The seam serves update and archive for a mirrored deal (write-back
  // projects onto the incumbent, overlay/provider_writes.go), so Edit and
  // Archive render in overlay too. Reopen and share stay hidden: reopen
  // dials advance under the hood, which the seam refuses outright (a mirror
  // deal carries no native pipeline/stage, OVA-MAP-6), and a record grant
  // probes the native deal row (auth.EnsureLinkTarget), which a mirror deal
  // has no row in, so the grant 404s — overlay visibility is governed by
  // mirror_visibility, which record_grant does not feed.
  const overlay = useSorMode() === "overlay";
  // One fact refuses every write below, so it is named once. Undefined while
  // the deal is live, which is what leaves the verbs pressable.
  const refusedByArchive = deal.archived_at ? archivedReasonId : undefined;
  // This deal's company, so the picker offers the projects that company is on —
  // as customer, partner or subcontractor. The server decides which; asking for
  // every project and filtering here on organization_id would show only the
  // ones it is the CUSTOMER of.
  const openProjects = useProjectsOfCompany(deal.organization_id ?? undefined);
  const projectById = useEntityName("project", deal.project_id);
  const currentProject = deal.project_id
    ? { id: deal.project_id, label: projectById.name ?? deal.project_id }
    : undefined;
  // What the form OPENS on, named once because two things need the same answer:
  // the form seeds its controls from it, and the patch is a diff against it.
  // Two spellings of "the record as the form read it" would drift, and the one
  // that drifts decides whether an untouched field is sent as a change.
  const seeded = { ...dealEditRecord(deal), ...cf.recordSlice(deal) };
  return (
    <>
      <DealEmailVerb
        deal={deal}
        overlay={overlay}
        disabledReasonId={refusedByArchive}
      />
      <EditAction<Deal>
        disabledReasonId={refusedByArchive}
        label={t("deal.edit")}
        savedMessage={(saved) => t("record.saveDone", { name: saved.name })}
        notice={overlay ? t("overlay.partialWriteBack") : undefined}
        fields={[
          ...dealEditFields(t, {
            orgs,
            partnerOptions,
            attributedPartner: attributedPartner(deal, orgs),
            currentCompany,
            masked,
            me: meId,
            currentOwner: deal.owner_id ?? null,
            // EMPTY, not a default. `dealEditFields` only uses this to put the
            // record's own currency at the head of the option list, and a deal
            // nobody has priced has none to put there.
            currency: deal.currency ?? "",
          }),
          ...editProjectFields(t, {
            masked,
            openProjects,
            currentProject,
            company: deal.organization_id ?? undefined,
          }),
          ...cf.formFields,
        ]}
        record={seeded}
        update={async (values, _rows, opened) => {
          // The company the form SUBMITS, not the one the deal had: a
          // project started here belongs to the company the save names.
          const submitted = stringValues(values);
          const projectId = await resolveDealProject(
            submitted,
            submitted.organization_id?.trim() || null,
            t,
          );
          const { data, error } = await api.PATCH("/deals/{id}", {
            params: {
              path: { id: deal.id },
              ...ifMatch(requireVersion(opened?.version)),
            },
            body: {
              ...mapDealUpdate(
                { ...values, project_id: projectId ?? "" },
                // The reading the form opened on, not the live one: `seeded`
                // is rebuilt on every render, so a refetch mid-edit would make
                // somebody else's change read as this person's.
                opened ?? seeded,
                masked,
              ),
              // The other half of the same body, diffed the same way and
              // against the same baseline. A snapshot here reproduced the
              // reported defect exactly: `cf_*` columns are clearable through
              // no path, so one empty custom field refused every save.
              ...cf.toPatch(values, opened ?? {}),
            },
          });
          if (error) {
            throwProblem(error);
          }
          return data;
        }}
        invalidate="deals"
        recordKey="deal"
      />
      {/* Behind the overflow, all three: archiving a deal, handing a link to
          somebody outside the workspace, and reopening a closed one are verbs
          whose consequence a reader has to read before pressing, so each of
          them wants a whole line rather than a place in a row. */}
      <OverflowMenu label={t("record.moreActions")}>
        <ArchiveAction
          disabledReasonId={refusedByArchive}
          label={t("deal.archive")}
          confirmText={t("deal.archiveConfirm")}
          archivedMessage={t("record.archiveDone", { name: deal.name })}
          archive={async () => {
            const { data, error } = await api.DELETE("/deals/{id}", {
              params: { path: { id: deal.id } },
            });
            if (error) {
              throwProblem(error);
            }
            return data;
          }}
          invalidate="deals"
          recordKey="deal"
          onArchived={() => navigate({ screen: "deals" })}
        />
        {!overlay && (
          <ShareAction
            recordType="deal"
            recordId={deal.id}
            disabledReasonId={refusedByArchive}
          />
        )}
        {/* Reopen answers a CLOSED deal, so an open one has no reason to be
            told about it — absent, not refused. An archived closed deal keeps
            it, refused: the reader came asking whether this can come back. */}
        {!overlay && (deal.status === "won" || deal.status === "lost") && (
          <ReopenAction
            dealId={deal.id}
            dealVersion={deal.version}
            openStages={openStages}
            disabledReasonId={refusedByArchive}
          />
        )}
      </OverflowMenu>
    </>
  );
}

type Approval = components["schemas"]["Approval"];

// The live 🟡 confirm-first staging queue for this deal — split out of
// DealScreen's render for the same readability reason as DealActions above.
function DealApprovals({
  approvals,
  decide,
}: Readonly<{
  approvals: Approval[];
  decide: (input: {
    approvalId: string;
    verdict: "approve" | "reject";
  }) => void;
}>) {
  const t = useT();
  const tierMap = useAgentTierMap();
  const viewerId = useViewerId();
  if (approvals.length === 0) {
    return null;
  }
  return (
    <Card
      title={t("deal.pendingApprovals")}
      style={{ marginBottom: "var(--space-4)" }}
    >
      {approvals.map((approval) => (
        <div
          key={approval.id}
          className="staging-card"
          style={{ marginBottom: 8 }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <AutonomyDot tier={approvalDotTier(approval.kind, tierMap)} />
            {/* The same two facts the approvals inbox states, said the same
                way. Printed off the wire they read `advance_deal` and
                `agent:capture` — the vocabulary the API speaks, on a page
                whose reader never sees the API. */}
            <span className="t-label">
              {approvalKindLabel(approval.kind, t)}
            </span>
            <ProvenanceTag
              provenance={provenanceOf(approval.proposed_by, viewerId)}
            />
          </div>
          <div className="approval-gate">
            <Button
              variant="primary"
              small
              onClick={() =>
                decide({ approvalId: approval.id, verdict: "approve" })
              }
            >
              {t("trust.accept")}
            </Button>
            <Button
              small
              onClick={() =>
                decide({ approvalId: approval.id, verdict: "reject" })
              }
            >
              {t("trust.dismiss")}
            </Button>
          </div>
        </div>
      ))}
    </Card>
  );
}

// Exported so a render test can reach the refusal state directly. It is not as
// self-contained as FxLine — it reads the system-of-record mode, so it needs a
// query client around it — but whether the New-offer control is refused depends
// on its props alone.
export function OffersPanel({
  offers,
  creating,
  locale,
  dealCurrency,
  onCreate,
}: Readonly<{
  offers: Offer[] | undefined;
  creating: boolean;
  locale: Locale;
  // An offer is written in the DEAL's currency, so a deal nobody has priced
  // has nothing to write one in. Null refuses the control and says why rather
  // than creating an offer denominated in a currency the code chose.
  dealCurrency: string | null;
  onCreate: (currency: string) => void;
}>) {
  const t = useT();
  // Offers are read (and created) against a mirrored deal — the list read 404s
  // and creation would write, both refused in overlay. Show the honest
  // unavailable state instead of an empty panel with a New-offer button.
  const overlay = useSorMode() === "overlay";
  if (overlay) {
    return (
      <Card title={t("deal.offers")} style={{ marginBottom: "var(--space-4)" }}>
        <OverlayUnavailable />
      </Card>
    );
  }
  return (
    <Card
      title={t("deal.offers")}
      actions={
        <Button
          small
          // `reason` disables the control AND points at the explanation. Passing
          // `disabled` beside it would cancel the refusal it sets, so the
          // in-flight case stays on `disabled` and the state case on `reason`.
          // An empty code is as absent as a null one — `formatMoneyOrAbsent`
          // already treats it that way, and Intl throws on it.
          disabled={Boolean(dealCurrency) && creating}
          reason={dealCurrency ? undefined : t("deal.offerNeedsCurrency")}
          onClick={() => {
            if (dealCurrency) {
              onCreate(dealCurrency);
            }
          }}
        >
          {t("deal.newOffer")}
        </Button>
      }
      style={{ marginBottom: "var(--space-4)" }}
    >
      {offers &&
        (offers.length > 0 ? (
          <DataTable
            label={t("deal.offers")}
            columns={[
              {
                key: "offer_number",
                header: t("deal.offerNumber"),
                render: (offer: Offer) => offer.offer_number,
              },
              {
                key: "revision",
                header: t("deal.offerRevision"),
                render: (offer: Offer) => String(offer.revision),
              },
              {
                key: "status",
                header: t("lead.status"),
                render: (offer: Offer) => <Badge>{offer.status}</Badge>,
              },
              {
                key: "gross",
                header: t("deals.amount"),
                render: (offer: Offer) => (
                  <span className="t-mono">
                    {formatMoney(offer.gross_minor, offer.currency, locale)}
                  </span>
                ),
              },
            ]}
            rows={offers}
            rowKey={(offer) => offer.id}
            onRowClick={(offer) => navigate({ screen: "offers", id: offer.id })}
          />
        ) : (
          <EmptyState>{t("deal.offersEmpty")}</EmptyState>
        ))}
    </Card>
  );
}

const DEAL_TABS = ["overview", "files", "history"] as const;
type DealTab = (typeof DEAL_TABS)[number];

// The pipeline's stages as ladder rungs: what is behind the deal, where it
// stands, and the ways out.
//
// `position` orders the pipeline and is what the trail is read from — a stage
// earlier in the pipeline than the deal's own has been passed. A deal whose
// stage the pipeline cannot name (an overlay mirror carrying the incumbent's
// id, a stage archived out from under it) leaves every rung unpassed rather
// than guessing a position, because a trail drawn from a guess says the deal
// went through stages it may never have seen.
function dealStageSteps({
  deal,
  stages,
  refused,
  onAdvance,
}: Readonly<{
  deal: Deal;
  stages: readonly Stage[];
  refused: boolean;
  onAdvance: (toStage: Stage) => void;
}>): StageStep[] {
  const here = stages.find((stage) => stage.id === deal.stage_id);
  return stages.map((stage) => ({
    key: stage.id,
    label: stage.name,
    done: here !== undefined && stage.position < here.position,
    current: stage.id === deal.stage_id,
    // Won and lost are the two ways out rather than two more rungs.
    terminal: stage.semantic !== "open",
    disabled: refused,
    onPick: () => onAdvance(stage),
  }));
}

// The deal 360's "overview" pane, split out of DealScreen so the tab switch
// doesn't push the render-prop closure over the cognitive-complexity budget.
// Every prop here is a value already resolved by DealScreen — no new
// fetches, no behavior change from the pre-tab layout.
// What the identity line reads off a deal. Every field is optional because
// every one of them is a fact a deal can lack or a reader can be refused.
type DealIdentity = Pick<
  Deal,
  | "amount_minor"
  | "currency"
  | "stage_id"
  | "owner_id"
  | "organization_id"
  | "partner_org_id"
  | "partner_attribution"
  | "masked_fields"
>;

/**
 * The one line of facts under a deal's name: what it is worth, where it sits
 * on the board, whose deal it is, and — when one brought it — which partner.
 *
 * It is the design system's `IdentityLine`, the same row the account and the
 * contact draw under their own names, so the three records read the same way.
 * These facts used to stand in a labelled box beside the verbs instead
 * (`DealFacts`), which made the deal the only record whose head answered "what
 * is this" in a different shape from every other — and cost the verbs the room
 * they need beside a name at the record rung.
 *
 * The partner was editable in the form and rendered nowhere, so a deal that a
 * partner sourced looked identical to one we won alone. That is the fact the
 * commission is computed from, and a figure a partner is paid on has to be
 * visible on the record it came from.
 *
 * Each reference goes through EntityRef, which resolves the name and links to
 * the record — and withholds both when the reader may not open it, which is
 * why the ids are not printed as a fallback. A withheld fact NAMES the field
 * it withholds: on a line of joined facts a bare mask says only "something
 * here is hidden", and the amount, the company and the partner are three
 * different things to be refused.
 */
export function DealIdentityLine({
  deal,
  stages,
  locale,
}: Readonly<{
  // The facts this line draws and no more. A presentational row does not need
  // a whole `Deal` to say what one is worth, and asking for one makes every
  // story and test that draws the line assemble a record it does not read.
  deal: DealIdentity;
  // The stages the PAGE already sorted for the board, not a second read.
  stages: readonly { id: string; name: string }[];
  locale: Locale;
}>) {
  const t = useT();
  // Only asked for when there is an owner to name: an unowned deal needs no
  // roster read to say so.
  const roster = useRoster("user", Boolean(deal.owner_id));
  const partial = useRosterPartial("user", Boolean(deal.owner_id));
  const masked = deal.masked_fields ?? [];
  // An em dash rather than the stage id: a deal in overlay mode carries no
  // native pipeline row, and printing a UUID where a stage name goes reads as
  // a fault.
  const stage = stages.find((candidate) => candidate.id === deal.stage_id);
  return (
    <IdentityMeta>
      <IdentityLine>
        {masked.includes("organization_id") ? (
          <IdentityFact>
            {t("create.organization")} <FieldGuard mode="masked" />
          </IdentityFact>
        ) : (
          deal.organization_id && (
            <IdentityFact>
              <EntityRef kind="organization" id={deal.organization_id} />
            </IdentityFact>
          )
        )}
        <IdentityFact>
          {/* A masked amount NAMES the field, like every other refusal on this
              line: a lone lock among joined facts says only that something
              here is hidden, and "no value recorded" and "you may not see the
              value" are different statements about a deal. */}
          {masked.includes("amount_minor") ? (
            <>
              {t("deals.amount")} <FieldGuard mode="masked" />
            </>
          ) : (
            dealAmount(deal, locale)
          )}
        </IdentityFact>
        <IdentityFact>{stage?.name ?? "—"}</IdentityFact>
        <IdentityFact quiet>
          {t("list.owner")}:{" "}
          {rosterOwnerName(
            deal.owner_id,
            roster,
            partial,
            t,
            t("co.pulse.unowned"),
          )}
        </IdentityFact>
        {masked.includes("partner_org_id") ? (
          // No attribution word here: what the partner did is withheld WITH
          // the partner, so naming one would decide what a partner nobody
          // could see is owed.
          <IdentityFact>
            {t("deal.partnerOrg")} <FieldGuard mode="masked" />
          </IdentityFact>
        ) : (
          deal.partner_org_id && (
            <IdentityFact>
              {/* Sourced and influenced are paid differently, so the line says
                  which one rather than a neutral "partner: X" that hides the
                  distinction the commission turns on. */}
              {t(
                deal.partner_attribution === "influenced"
                  ? "deal.partnerInfluenced"
                  : "deal.partnerSourced",
              )}{" "}
              <EntityRef kind="organization" id={deal.partner_org_id} />
            </IdentityFact>
          )
        )}
      </IdentityLine>
    </IdentityMeta>
  );
}

// The value, or an em dash when the deal carries none. The masked case is the
// caller's, because a refusal on this line is written as the field's name
// beside the mark rather than as the mark alone.
function dealAmount(deal: DealIdentity, locale: Locale): ReactNode {
  if (deal.amount_minor == null || !deal.currency) {
    return "—";
  }
  return formatMoney(deal.amount_minor, deal.currency, locale);
}

// Deal360 leads the page. It is absent on an overlay-backed deal: the briefing
// is written from records this installation holds, and a mirrored deal's
// timeline is the incumbent's rather than ours.
function DealLead({
  dealId,
  dealName,
  overlay,
  pulse,
  spine,
}: Readonly<{
  dealId: string;
  dealName: string;
  overlay: boolean;
  pulse: ReactNode | undefined;
  spine: ReactNode;
}>) {
  if (overlay) {
    return null;
  }
  return (
    <DealStatusCardPanel
      dealId={dealId}
      dealName={dealName}
      pulse={pulse}
      spine={spine}
    />
  );
}

/**
 * The deal's tags, drawn by the SHARED panel.
 *
 * Same three questions the server asks before it writes: the object grant,
 * this record's own editability, and — inside the panel — whether the caller
 * may see the vocabulary at all.
 */
function DealTagsSection({ deal }: Readonly<{ deal: Deal }>) {
  // useCanWriteRecord, not useCan: applying a tag writes to the deal, so the
  // verb owes the seat ceiling and this row's own `writable` alongside the
  // object grant — a grant by itself would offer the picker on a colleague's
  // deal, and to a read seat the server refuses before RBAC is consulted.
  const canUpdate = useCanWriteRecord("deal", deal);
  const overlay = useSorMode() === "overlay";
  return (
    <TagsPanel
      entityType="deal"
      entityID={deal.id}
      canEdit={canUpdate && !deal.archived_at && !overlay}
    />
  );
}

function DealOverviewPane({
  deal,
  stages,
  dealApprovals,
  onDecide,
  offers,
  creatingOffer,
  locale,
  baseCurrency,
  onCreateOffer,
  overlay,
  onAdvance,
  advancing,
  advanceRefused,
  pulse,
  spine,
  coverage,
  onOpenHistory,
}: Readonly<{
  deal: Deal;
  stages: Stage[];
  dealApprovals: Approval[];
  onDecide: (input: {
    approvalId: string;
    verdict: "approve" | "reject";
  }) => void;
  offers: Offer[] | undefined;
  creatingOffer: boolean;
  locale: Locale;
  baseCurrency: string | null;
  onCreateOffer: (currency: string) => void;
  overlay: boolean;
  onAdvance: (toStage: Stage) => void;
  /** One advance at a time: a second click while the first is in flight would
   * send a second write pinned to the same version, and the loser reads as a
   * conflict the reader never caused. */
  advancing: boolean;
  /** Where this deal cannot be moved at all — archived (restore it first), or
   * mirrored from an incumbent that refuses the write. */
  advanceRefused: boolean;
  // Whose move it is, in one sentence, drawn under the call inside the
  // reading — it is the sentence the call rests on, not a header fact.
  pulse: ReactNode | undefined;
  // The deal's story as a thread, drawn under the call inside the reading.
  spine: ReactNode;
  // Who is on the deal, for the reading's reference pair beside the offers.
  coverage: ReturnType<typeof useDealCoverage>;
  // Where the momentum reading's door goes: the history tab, which the page
  // owns.
  onOpenHistory: () => void;
}>) {
  const t = useT();
  return (
    // The same stack every record's overview reads down, with its rhythm.
    <div className="record-stack">
      {/* The readings open the overview, as they do on every record page.
          Not in overlay, where the mirror carries no offers or seats. */}
      {!overlay && (
        <DealStrip
          deal={deal}
          offers={offers}
          coverage={coverage.coverage}
          coverageWithheld={coverage.withheld}
          onOpenHistory={onOpenHistory}
        />
      )}
      {deal.fx_rate_to_base != null && (
        <FxLine
          amountMinor={deal.amount_minor ?? null}
          baseCurrency={baseCurrency}
          fxRateToBase={deal.fx_rate_to_base}
          fxRateDate={deal.fx_rate_date ?? null}
          locale={locale}
        />
      )}
      {/* A won deal with no project, on a company with exactly one open
          project, is offered that project once. Nothing else here asks. */}
      {!overlay && <StartDeliveryPrompt deal={deal} />}
      {/* Where the deal is now is a fact, not a choice, so the current stage
          stays a marker. Every other stage is the move to it — which is what
          makes a deal closable from its own page rather than only by dragging
          its card on the board. */}
      {stages.length > 0 && (
        <StageLadder
          label={t("deals.stage")}
          steps={dealStageSteps({
            deal,
            stages,
            refused: advancing || advanceRefused,
            onAdvance,
          })}
        />
      )}
      {/* ONE READING, IN PARTS, below the stage bar on purpose: a reader
          takes in WHERE the deal is before they read the account of how it got
          there. The call with the deal's thread, the move the briefing names,
          the brief itself, and under them the two sections a reader consults
          rather than reads — what is on the table, and who is in the room.
          The buying committee sits beside the offers rather than at the foot
          of the page, because the two are the deal's two sides. */}
      <RecordReading>
        <DealLead
          dealId={deal.id}
          dealName={deal.name}
          overlay={overlay}
          pulse={pulse}
          spine={spine}
        />
        <DealApprovals approvals={dealApprovals} decide={onDecide} />
        <RecordReadingPair>
          <OffersPanel
            offers={offers}
            creating={creatingOffer}
            locale={locale}
            dealCurrency={deal.currency ?? null}
            onCreate={onCreateOffer}
          />
          <DealCommitteeMap
            coverage={coverage.coverage}
            withheld={coverage.withheld}
            pending={coverage.pending}
            overlay={overlay}
          />
        </RecordReadingPair>
      </RecordReading>
      <CustomFieldsCard object="deal" record={deal} />
      <RecordContextPanel entityType="deal" id={deal.id} />
      <LogActivity entityType="deal" entityId={deal.id} />
    </div>
  );
}

// The page's ONE sentence about this deal being archived, said across the
// whole header rather than repeated beside each of the four verbs the archive
// refuses. Nothing at all while the deal is live — `undefined` rather than an
// element that renders null, because RecordView reserves the band's space for
// anything it is handed, and a page that always kept the gap would read as a
// record with something to say about itself and nothing said.
// The sentence under the call: whose move it is.
//
// Absent in overlay mode for the reason the readings are: whose move it is, is
// read from this installation's own timeline, and a mirrored deal's is the
// incumbent's.
function dealPulse({
  card,
  timeline,
  overlay,
}: Readonly<{
  card?: DealStatusCard;
  timeline: readonly Activity[];
  overlay: boolean;
}>): ReactNode | undefined {
  if (overlay) {
    return undefined;
  }
  return <DealPulse card={card} timeline={timeline} />;
}

// The band under the header: the deal's four readings, and the archived notice
// when there is one.
//
// RecordView reserves the band's space for anything it is handed, so this
// answers `undefined` rather than a null-rendering element when there is
// nothing to say — which is only ever the case in overlay mode, where the
// readings are assembled from records this installation does not hold.
function dealBand({
  deal,
  reasonId,
  t,
}: Readonly<{
  deal: Deal;
  reasonId: string;
  t: ReturnType<typeof useT>;
}>): ReactNode | undefined {
  if (deal.archived_at == null) {
    return undefined;
  }
  return (
    <p id={reasonId} className="t-caption">
      {t("deal.archivedReadOnly")}
    </p>
  );
}

export function DealScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const details = usePageAside();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const queryClient = useQueryClient();
  // Minted here because the band that carries the sentence and the verbs that
  // point at it are two different slots of the same header.
  const archivedReasonId = useId();
  const [tab, setTab] = useState<DealTab>("overview");
  const [pending, setPending] = useState<PendingAdvance | null>(null);
  const advance = useAdvanceDeal();
  const dealQuery = useQuery({
    queryKey: ["deal", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/deals/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const pipelineQuery = usePipeline(dealQuery.data?.pipeline_id);
  // One shared singleton read (the ["installation-settings"] key), not a
  // per-deal request: the FX line has to name the base currency it converted
  // into, and nothing on the deal itself carries it.
  const baseCurrency = useInstallationSettings().data?.base_currency ?? null;
  const me = useMe();
  const viewerId = useViewerId();
  // Overlay serves a read-only mirror: entity-scoped activity reads (timeline)
  // and the deal's stakeholders/offers sub-resources 422/404, and offer
  // creation would write to a mirrored deal. Gate all of it on this.
  const overlay = useSorMode() === "overlay";
  // Asked here rather than inside the aside, because an element is truthy
  // whatever it renders: a slot filled with a component that draws nothing
  // still reserves the aside column and its landmark.
  const orgs = useQuery({
    queryKey: ["organizations"],
    queryFn: async () => {
      const { data, error } = await api.GET("/organizations", {
        params: { query: { limit: 50 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  // The shared pending-approvals hook, not a second query on the same key: a
  // private queryFn under ["approvals","pending"] takes over the cache entry the
  // Inbox and the rail badge read, and this one stopped at the first page and
  // kept expired rows — so a visit here could silently cap the badge at 50 and
  // count approvals nobody can act on.
  const approvalsQuery = usePendingApprovals();
  // Both already fetched by children — the status card by Deal360, coverage by
  // the signal chips and the coverage card. Read here so the header's sentence
  // and the readings band share those cache entries rather than adding two
  // requests for facts the page already holds.
  const statusQuery = useDealStatusCard(id);
  const coverageRead = useDealCoverage(id, !overlay);
  // The pane's content, or nothing while it is folded: an aside handed to the
  // view reserves its column, so a closed pane hands it none.
  const dealContext = (deal: Deal) =>
    details.open ? (
      <DealContext deal={deal} coverage={coverageRead} overlay={overlay} />
    ) : undefined;
  const [timelineFilters, setTimelineFilters] = useTimelineFilters(id);
  const timelineQuery = useRecordTimeline("deal", id, {
    filters: timelineFilters,
  });
  // The thread under the call reads the WHOLE history, not the page the filter
  // strip narrowed: a filter is a view of the timeline tab, and a call that
  // said "no reply since" because the reader had hidden emails would be false.
  // The two share one query whenever no filter is set.
  const threadQuery = useRecordTimeline("deal", id);
  const [openEmail, setOpenEmail] = useOpenEmail();
  const rawTimelineEntries = activityTimeline(
    timelineQuery.activities,
    viewerId,
    (activity) => (
      <TimelineActions activity={activity} entityType="deal" entityId={id} />
    ),
  );
  const timelineEntries = withEmailOpener(rawTimelineEntries, setOpenEmail);
  const offersQuery = useQuery({
    queryKey: ["deal-offers", id],
    enabled: !overlay,
    queryFn: async () => {
      const { data, error } = await api.GET("/deals/{id}/offers", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const createOffer = useMutation({
    mutationFn: async (currency: string) => {
      const { data, error } = await api.POST("/deals/{id}/offers", {
        params: { path: { id } },
        body: { currency, source: "manual" },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (offer: Offer) => {
      navigate({ screen: "offers", id: offer.id });
    },
  });
  const decide = useMutation({
    mutationFn: async (input: {
      approvalId: string;
      verdict: "approve" | "reject";
    }) => {
      const path =
        input.verdict === "approve"
          ? "/approvals/{id}/approve"
          : "/approvals/{id}/reject";
      const { error } = await api.POST(path, {
        params: { path: { id: input.approvalId } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["approvals", "pending"] }),
  });

  return (
    <div className="wrap">
      <QueryGate query={dealQuery} pendingLabel={t("nav.deals")}>
        {(deal) => {
          const stages = [...(pipelineQuery.data?.stages ?? [])].sort(
            (a, b) => a.position - b.position,
          );
          const openStages = stages.filter(
            (stage) => stage.semantic === "open",
          );
          const dealApprovals = (approvalsQuery.data?.data ?? []).filter(
            (approval) => approval.target_entity_id === deal.id,
          );
          return (
            <RecordView
              // Context first: who these people are, before the verbs that act
              // on them. The seats moved out of the main column when the
              // readings band started counting them — the same two facts were
              // reaching a reader three times on one screen. The pane is the
              // one every record page draws, with the same fold and the same
              // memory of it.
              aside={dealContext(deal)}
              name={deal.name}
              pulse={
                <DealIdentityLine deal={deal} stages={stages} locale={locale} />
              }
              zone={recordZone}
              actionsInline
              badges={
                <>
                  <Badge tone={dealStatusTone(deal.status)}>
                    {deal.status}
                  </Badge>
                  <DealProjectChip deal={deal} />
                </>
              }
              actions={
                <>
                  <DealActions
                    deal={deal}
                    orgs={orgs.data?.data ?? []}
                    meId={me.data?.user.id ?? ""}
                    openStages={openStages}
                    archivedReasonId={archivedReasonId}
                  />
                </>
              }
              band={dealBand({ deal, reasonId: archivedReasonId, t })}
              timeline={timelineEntries}
              timelineGroups={groupChronology(
                timelineEntries,
                timelineQuery.hasNextPage,
              )}
              timelineHeader={
                overlay ? undefined : (
                  <TimelineFilterBar
                    value={timelineFilters}
                    onChange={setTimelineFilters}
                  />
                )
              }
              timelineFooter={
                <>
                  <LoadMoreButton query={timelineQuery} />
                  {/* One drawer over the deal, beside the timeline it opens
                      from. */}
                  <OpenEmailDrawer
                    activityId={openEmail}
                    zone={recordZone}
                    onClose={() => setOpenEmail(null)}
                  />
                </>
              }
              timelineNotice={timelineZoneNotice(
                { overlay, pending: timelineQuery.isPending },
                t,
              )}
            >
              {/* The same strip every record carries: a place a reader
                  navigates, drawn as a rule with the open body underlined. */}
              <RecordTabs
                options={DEAL_TABS}
                value={tab}
                onChange={setTab}
                labels={{
                  overview: t("tab.overview"),
                  files: t("tab.documents"),
                  history: t("tab.history"),
                }}
                // The switch for the deal's details column, at the end of the
                // tab row: it chooses what the page shows BESIDE the work, so
                // it stands with the controls that choose what the work column
                // shows. In the head it sat among the deal's own verbs — write,
                // edit, the overflow — and read as one more thing to do to the
                // record rather than as a way to see more of it.
                trailing={<PageAsideToggle />}
              />
              {tab === "overview" && (
                <DealOverviewPane
                  deal={deal}
                  stages={stages}
                  dealApprovals={dealApprovals}
                  onDecide={(input) => decide.mutate(input)}
                  offers={offersQuery.data?.data}
                  creatingOffer={createOffer.isPending}
                  locale={locale}
                  baseCurrency={baseCurrency}
                  onCreateOffer={(currency) => createOffer.mutate(currency)}
                  overlay={overlay}
                  advancing={advance.isPending}
                  pulse={dealPulse({
                    card: statusQuery.data,
                    timeline: timelineQuery.activities,
                    overlay,
                  })}
                  spine={
                    <TimelineThread
                      thread={threadQuery}
                      commercial={{ next_close_on: deal.expected_close_date }}
                    />
                  }
                  coverage={coverageRead}
                  onOpenHistory={() => setTab("history")}
                  // An archived deal is not moved through the pipeline, and
                  // the mirror answers an advance with unsupported_by_sor —
                  // a control that can only fail is worse than none.
                  //
                  // A CLOSED deal is refused here too, but for a different
                  // reason: reopening is its own deliberate action, with a
                  // dialog that says the close date and the frozen rate are
                  // being cleared. A stepper button that reopened silently
                  // would be a second, quieter door to the same write.
                  advanceRefused={
                    deal.archived_at != null ||
                    overlay ||
                    deal.status !== "open"
                  }
                  onAdvance={(toStage) => {
                    // The version this record was drawn from, exactly as the
                    // board pins the version its card was drawn from: the
                    // write names the deal as the reader saw it, so a change
                    // made elsewhere meanwhile fails loud.
                    const input = {
                      dealId: deal.id,
                      version: deal.version,
                      toStage,
                    };
                    if (toStage.semantic === "open") {
                      advance.mutate(input);
                    } else {
                      setPending(input);
                    }
                  }}
                />
              )}
              {tab === "overview" && (
                <DealPeoplePanels dealId={deal.id} overlay={overlay} />
              )}
              {tab === "files" && !overlay && <DealFiles dealId={deal.id} />}
              {tab === "files" && overlay && <OverlayUnavailable />}
              {tab === "history" && !overlay && (
                <RecordHistoryTab
                  kind="deal"
                  id={deal.id}
                  currency={deal.currency}
                  restore={{
                    version: deal.version,
                    onRestored: () =>
                      invalidateRecord(queryClient, "deal", deal.id),
                  }}
                />
              )}
              {tab === "history" && overlay && <OverlayUnavailable />}
              {advance.isError && (
                <p
                  className="t-caption"
                  style={{
                    color: "var(--danger)",
                    marginTop: "var(--space-2)",
                  }}
                >
                  {problemMessageOf(advance.error, t)}
                </p>
              )}
              <ConfirmAdvanceModal
                pending={pending}
                onClose={() => setPending(null)}
                onConfirm={(input) =>
                  advance.mutateAsync(input).then(
                    () => null,
                    (error: unknown) => error,
                  )
                }
              />
            </RecordView>
          );
        }}
      </QueryGate>
    </div>
  );
}

// The deal's context, for the details pane: the seats first, then how the deal
// is filed, then the deal rooms and the mail card. DealSeats is present in
// overlay mode too, stating the refusal — dropping it there took the seats
// away silently, which reads as "nobody is on this deal"; the rooms and the
// mail aside stay out because they are actions rather than a withheld fact.
function DealContext({
  deal,
  coverage,
  overlay,
}: Readonly<{
  deal: Deal;
  coverage: ReturnType<typeof useDealCoverage>;
  overlay: boolean;
}>) {
  return (
    <>
      <DealSeats
        coverage={coverage.coverage}
        withheld={coverage.withheld}
        pending={coverage.pending}
        overlay={overlay}
      />
      <DealTagsSection deal={deal} />
      {!overlay && (
        <>
          <DealRoomAside dealId={deal.id} dealName={deal.name} />
          <DealEmailAside dealId={deal.id} />
        </>
      )}
    </>
  );
}
