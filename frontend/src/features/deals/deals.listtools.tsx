import { type Dispatch, type SetStateAction, useMemo } from "react";
import {
  admittedDealFilters,
  useAppliedCompanyName,
} from "../../components/dealdials";
import { searchCompanies } from "../../components/filterreference";
import {
  type FilterSpec,
  LIST_PAGE_SIZES,
  type ListQuery,
  listPageOf,
  listQueryFromParams,
  mergeScreenDials,
  paramsFromListQuery,
  withListPage,
  withoutScreenDials,
} from "../../components/listquery";
import { SegmentedControl } from "../../design-system/atoms";
import type { ListChip } from "../../design-system/listsurface";
import type { ListSelection } from "../../design-system/listtable";
import { Select } from "../../design-system/select";
import { type Translator, useLocale, useT } from "../../i18n";
import type { AcquisitionSource } from "../../lib/acquisitionsources.queries";
import { useCan } from "../../lib/capability";
import {
  roleRefusedDealDials,
  withheldDealDials,
  withoutWithheldDials,
} from "../../lib/dealranges";
import {
  DEAL_SCREEN_DIALS,
  type DealFilters,
  PIPELINE_PARAM,
  VIEW_PARAM,
} from "../../lib/deals.queries";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import { useRecordZone } from "../../lib/recordzone";
import { heldRefusedDials } from "../../lib/refusedfilters";
import { useMe } from "../../lib/session";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { currentParams, replaceDial, useUrlParams } from "../../lib/urlstate";
import { dealSurfaceChips } from "./deal360/dealfilterchips";
import { DealBulkBar } from "./dealbulk";
import type { Deal, Pipeline, Stage } from "./deals";
import { dealRangeChips } from "./deals.rangefilters";

// Bespoke selects for the filters whose option labels are runtime strings
// (pipeline/stage/company names). A chip's option label is a MessageKey, so
// these three cannot go through ListTable's chips. Each writes into the
// ListQuery.filters bag the table's chips read. It deletes the key on a blank
// choice, so the two stay in one coherent query state.
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

// Whether the reader has narrowed this list themselves.
//
// The question `SaveViewAction` asks before it offers to save. It is asked here
// because the pipeline is folded into the saved query without making the query
// look narrowed. A pipeline is always selected, so counting it would offer to
// save the default list.
function narrowsTheDealList(query: ListQuery): boolean {
  return (
    Boolean(query.q) ||
    Boolean(query.sort) ||
    query.includeArchived ||
    Object.values(query.filters).some(Boolean)
  );
}

// The stage and company filters. The stage list is loaded whole already (a
// pipeline has few stages), so it stays a fixed chip. The company filter
// searches rather than listing (see searchCompanies in filterreference.ts).
// Both are still filters, so they read as the same chip as every other one
// instead of as a native select sitting among them.
export function dealFilterChips(
  stages: Stage[],
  t: ReturnType<typeof useT>,
  companyName: string | undefined,
): ListChip[] {
  return [
    {
      key: "stage_id",
      label: t("deals.stage"),
      allLabel: t("deals.filterStageAll"),
      options: stages.map((stage) => ({ value: stage.id, label: stage.name })),
    },
    {
      key: "company_id",
      label: t("create.relatedCompany"),
      allLabel: t("deals.filterCompanyAll"),
      options: [],
      search: searchCompanies,
      appliedName: companyName,
    },
  ];
}

// How the deals are shown rather than which ones: the board/table switch and
// the pipeline being looked at. Both sit on the right with the table's own
// display controls.
export function DealViewTools({
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
            // A stage belongs to one pipeline, so switching pipeline strands any
            // stage_id filter. The chip blanks out, but useDeals would still
            // forward the old id and filter a foreign stage (0 rows).
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
            {pipelines[0]?.name}
          </span>
        )
      )}
    </>
  );
}

// Every dial on this screen, read from and written to the ADDRESS.
//
// A hook of its own, because this screen hand-rolls what
// components/listquery.tsx does for every other list. It drives a board as
// well as a table. It uses the same codec, so a narrowed list's URL has one
// answer. The hand-rolled part is worth reading in one piece.
//
// `pipeline_id` and `view` sit beside the query's own dials. The first is
// already a wire parameter name. The second is the one dial here about drawing
// rather than about which deals exist.
export function useDealScreenDials({
  pipelines,
}: Readonly<{ pipelines?: Pipeline[] }>) {
  const [params, setParams] = useUrlParams();
  const opening = useMemo<ListQuery>(
    () => ({
      q: "",
      sort: "",
      includeArchived: false,
      filters: {},
      // The deal list reads its own fixed page (the board is capped at 100 and
      // documented as such). So this is the shape's default, not a dial the
      // footer offers.
      perPage: LIST_PAGE_SIZES[0],
    }),
    [],
  );
  const baseCurrency = useInstallationSettings().data?.base_currency;
  const readsCompanies = useCan("company", "read");
  const refused = useMe().data?.authorization?.refused_deal_filters;
  const withheld = useMemo(
    () =>
      withheldDealDials({
        baseCurrency,
        readsCompanies,
        refused: refused ?? [],
      }),
    [baseCurrency, readsCompanies, refused],
  );
  const { query, refusedDials } = useMemo(() => {
    const read = listQueryFromParams(
      withoutScreenDials(params, DEAL_SCREEN_DIALS),
      opening,
      true,
    );
    const admitted = admittedDealFilters(read.filters);
    const roleRefused = roleRefusedDealDials({
      readsCompanies,
      refused: refused ?? [],
    });
    // The role's refusals stay in the query to be named or refused by the
    // server; the rest of the withheld dials are dropped and apply once readable.
    const quiet = new Set([...withheld].filter((d) => !roleRefused.has(d)));
    const filters = withoutWithheldDials(admitted, quiet);
    return {
      query: filters === read.filters ? read : { ...read, filters },
      refusedDials: heldRefusedDials(admitted, roleRefused),
    };
  }, [params, opening, withheld, readsCompanies, refused]);
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
        // The rendered page is carried across rather than dropped, as
        // useListQuery carries it. The codec does not compute it, so rebuilding
        // the address from the query alone would drop the page from a link a
        // reader was sent. A narrowing write still resets it: the table's own
        // reset fires next.
        withListPage(paramsFromListQuery(next, opening), listPageOf(live)),
        live,
        DEAL_SCREEN_DIALS,
      ),
    );
  };
  const pipelineId = params.get(PIPELINE_PARAM) ?? "";
  const setPipelineId = (next: string) => replaceDial(PIPELINE_PARAM, next);
  const effectivePipeline: Pipeline | undefined =
    pipelines?.find((p) => p.id === pipelineId) ??
    pipelines?.find((p) => p.is_default) ??
    pipelines?.[0];
  const dealFilters: DealFilters = {
    pipelineId: effectivePipeline?.id ?? "",
    q: query.q,
    sort: query.sort,
    includeArchived: query.includeArchived,
    filters: query.filters,
  };

  const view: "board" | "table" =
    params.get(VIEW_PARAM) === "table" ? "table" : "board";
  const setView = (next: "board" | "table") =>
    replaceDial(VIEW_PARAM, next === "table" ? next : undefined);

  return {
    query,
    refusedDials,
    setQuery,
    baseCurrency,
    withheld,
    pipelineId,
    setPipelineId,
    effectivePipeline,
    dealFilters,
    view,
    setView,
  };
}

// The bulk-selection contract the table is handed, and the board is not.
//
// The row checkboxes live inside the grid's identity cell, and the board draws
// no rows to put one in. Offered while the board is showing, the bulk bar would
// name rows the reader can neither see nor deselect.
export function dealRowSelection({
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
        // A closed or archived deal takes no bulk write. Archiving it is done
        // or meaningless. Moving it between open stages would be the unannounced
        // reopen the stepper already refuses.
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
        label: (deal) => t("bulk.selectRow", { name: deal.name }),
        bar: (
          <DealBulkBar
            deals={selectedRows}
            stages={stages}
            // The rows that went through leave the selection. The ones that
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
// The pipeline goes in as a filter: it is the strongest dial on this screen,
// and it lives in its own state, outside `query`. Left out, a view saved on one
// pipeline would restore against whichever pipeline was showing. That is a
// different list under the saved name.
//
// Folded in only once the reader has narrowed something else. A pipeline is
// always selected. Folding it in every time would make every list look narrowed
// and offer to save the default view, the clutter SaveViewAction's check prevents.
export function savableDealQuery(
  query: ListQuery,
  pipelineId: string,
): ListQuery {
  return narrowsTheDealList(query)
    ? { ...query, filters: { ...query.filters, pipeline_id: pipelineId } }
    : query;
}

// The refused dials a chip names, with those names. A dial no chip names is
// left to the list's own refusal; a stripped partner dial still finds its chip.
function refusedDealLabel(
  dials: readonly (string | undefined)[],
  {
    dataChips,
    surfaceOf,
    t,
  }: Readonly<{
    dataChips: readonly ListChip[];
    surfaceOf: (partnerApplied: string | undefined) => readonly FilterSpec[];
    t: Translator;
  }>,
): { dials: string[]; names: string } | undefined {
  const held = dials.filter((dial): dial is string => Boolean(dial));
  const chips = surfaceOf(held.find((dial) => dial === "partner_company_id"));
  const named = held.flatMap((dial) => {
    const chip = dataChips.find((each) => each.key === dial);
    if (chip) return [{ dial, name: chip.label }];
    const spec = chips.find((each) => each.key === dial);
    return spec ? [{ dial, name: t(spec.label) }] : [];
  });
  if (named.length === 0) return undefined;
  return {
    dials: named.map((each) => each.dial),
    names: [...new Set(named.map((each) => each.name))].join(", "),
  };
}

// The filter chips this list offers, and the names it gives a refused dial. A
// withheld dial gets no chip, yet its chip still names it when it is refused.
export function useDealChips({
  stages,
  query,
  withheld,
  baseCurrency,
  partnerOptions,
  acquisitionSources,
}: Readonly<{
  stages: Stage[];
  query: ListQuery;
  withheld: ReadonlySet<string>;
  baseCurrency: string | undefined;
  partnerOptions: { value: string; label: string }[];
  acquisitionSources: AcquisitionSource[] | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  // A company this reader may not search for is not looked up either.
  const companyName = useAppliedCompanyName(
    withheld.has("company_id") ? undefined : query.filters.company_id,
  );
  const leading = useLeadingCurrencies();
  const offered = (chip: { key: string }) => !withheld.has(chip.key);
  const everyDealChip = dealFilterChips(stages, t, companyName);
  const rangeOf = (held: ReadonlySet<string>) =>
    dealRangeChips({
      words: { t, locale, zone },
      filters: query.filters,
      baseCurrency,
      withheld: held,
      leading,
    });
  const surfaceOf = (partnerApplied: string | undefined) =>
    dealSurfaceChips({
      partnerOptions,
      partnerApplied,
      acquisitionSources,
      retiredSuffix: t("deal.acquisitionRetired"),
    });
  return {
    dealChips: everyDealChip.filter(offered),
    rangeChips: rangeOf(withheld),
    surfaceChips: surfaceOf(query.filters.partner_company_id).filter(offered),
    nameRefusal: (dials: readonly (string | undefined)[]) =>
      refusedDealLabel(dials, {
        dataChips: [...everyDealChip, ...rangeOf(new Set())],
        surfaceOf,
        t,
      }),
  };
}
