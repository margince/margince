import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { usePageName } from "../../app/pagemeta";
import { searchCompanyTargets } from "../../components/companyform";
import { CreateAction } from "../../components/create";
import {
  type ObjectCustomFields,
  useObjectCustomFields,
} from "../../components/customfields.form";
import { useAppliedCompanyName } from "../../components/dealdials";
import {
  type ListState,
  ListTable,
  useTagChips,
} from "../../components/listquery";
import { useOwnerChips } from "../../components/ownerdials";
import { mineEmptyNote, standardViews } from "../../components/recordlist";
import { ErrorLine } from "../../design-system/errorline";
import { currencyOptions } from "../../format/currencychoices";
import { formatNumber } from "../../format/format";
import { translatePlural, useLocale, useT } from "../../i18n";
import { useAcquisitionSources } from "../../lib/acquisitionsources.queries";
import {
  totalsRefusedBecause,
  totalsWithheldBecause,
  useDeals,
  useStageTotals,
} from "../../lib/deals.queries";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import { usePipelines } from "../../lib/pipelines.queries";
import { throwProblem, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { useViewerId } from "../../lib/session";
import { SaveViewAction, useSavedViewTabs } from "../lists/savedviews";
import { LegalReviewQueueLink } from "../privacy/legalreviews";
import { useOwnerPicker } from "../records/recordreferences";
import {
  type ClosedDeal,
  CloseReviewOffer,
  closedDealOf,
} from "./deal360/closereviewoffer";
import {
  advanceErrorBehind,
  ConfirmAdvanceModal,
  type PendingAdvance,
} from "./deal360/confirmadvance";
import { dealSurfaceChips } from "./deal360/dealfilterchips";
import {
  dealProjectFields,
  resolveDealProject,
  useProjectsOfCompany,
} from "./dealproject";
import type { Deal, Pipeline, Stage } from "./deals";
import {
  DealBoardBody,
  useAdvanceDeal,
  useBoardInteractions,
} from "./deals.board";
import { dealColumns } from "./deals.columns";
import { partnerFields, usePartnerOptions } from "./deals.fields";
import {
  DealViewTools,
  dealFilterChips,
  dealRowSelection,
  savableDealQuery,
  useDealScreenDials,
} from "./deals.listtools";
import { mapDealCreate } from "./deals.write";

// The create form, with the one question it has to ask the server: which
// projects the company named in the OPEN FORM is on.
//
// Its own component because that question is the form's and not the screen's.
// The company comes from the form's own values. So the state that holds it and
// the query that follows it belong beside the form, not among the screen's dials.
function DealCreateAction({
  pipeline,
  cf,
  openStages,
  partnerOptions,
  startOpen,
}: Readonly<{
  pipeline?: Pipeline;
  // Read at screen level and handed down, so the schema request runs beside the
  // screen's own. It does not wait for the pipelines this form is gated on.
  cf: ObjectCustomFields;
  openStages: Stage[];
  partnerOptions: { value: string; label: string }[];
  startOpen: boolean;
}>) {
  const t = useT();
  const leading = useLeadingCurrencies();
  // The picker asks the server for one company's projects, so it is keyed on
  // the company the open form names. A create form has no record to take it
  // from, and only the server knows which projects a company is on. The form
  // publishes its answers (CreateAction's onValuesChange) and the query follows.
  const [formCompany, setFormCompany] = useState("");
  const openProjects = useProjectsOfCompany(formCompany || undefined);
  const projectOwner = useOwnerPicker(null);

  const createDeal = async (
    values: Record<string, string>,
    _rows: unknown,
    idempotencyKey: string,
  ) => {
    if (!pipeline) {
      throwProblem(null);
    }
    // A project asked for on the form is born first, so the deal names it.
    const projectId = await resolveDealProject(
      values,
      values.company_id?.trim() || null,
      t,
      idempotencyKey,
    );
    return unwrap(
      await api.POST("/deals", {
        params: { header: { "Idempotency-Key": idempotencyKey } },
        body: {
          ...mapDealCreate(
            { ...values, project_id: projectId ?? "" },
            pipeline.id,
            leading[0],
          ),
          ...cf.toBody(values),
        },
      }),
      t,
    );
  };

  return (
    <CreateAction
      label={t("create.deal")}
      invalidate="deals"
      screen="deals"
      create={createDeal}
      startOpen={startOpen}
      onValuesChange={(values) => setFormCompany(values.company_id ?? "")}
      fields={[
        { key: "name", label: "create.dealName", required: true },
        {
          key: "amount",
          label: "create.amount",
          type: "number",
          step: "any",
          min: "0",
          wireField: "amount_minor",
        },
        {
          key: "currency",
          label: "create.currency",
          type: "select",
          required: true,
          options: currencyOptions(leading),
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
        // Searched: the screen's own page of companies names board cards and
        // stops short of a large workspace.
        {
          key: "company_id",
          label: "create.relatedCompany",
          searchTargets: searchCompanyTargets,
        },
        // The body of work this deal is about, chosen or started here: a
        // project begins during the deal, in its initiative phase.
        // Narrowed by the server, for the company the form names now. The
        // query above re-reads when the reader changes it, so the picker
        // repopulates instead of going empty.
        ...dealProjectFields(
          t,
          openProjects,
          undefined,
          formCompany,
          projectOwner,
        ),
        // A deal brought by a partner is attributed at birth, not by editing
        // it afterwards. The win that pays them can come before anybody
        // thinks to revisit the record.
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

export function DealsScreen({
  startCreating = false,
}: Readonly<{ startCreating?: boolean }>) {
  const t = useT();
  const pageName = usePageName("deals");
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const cf = useObjectCustomFields("deal", "create");
  const pipelinesQuery = usePipelines();
  const ownerChips = useOwnerChips();
  const viewerId = useViewerId();
  const savedViews = useSavedViewTabs("deals");
  const {
    query,
    setQuery,
    setPipelineId,
    effectivePipeline,
    dealFilters,
    view,
    setView,
  } = useDealScreenDials({ pipelines: pipelinesQuery.data });
  const dealsQuery = useDeals(dealFilters);
  // The board's column totals: a per-stage server aggregate over every
  // matching deal, not only the capped page useDeals fetches. Built from the
  // same filter dials, so cards and totals agree on which deals are in view.
  const stageTotalsQuery = useStageTotals(dealFilters);
  // The same answer the query keys its `enabled` on, so the column explains the
  // absence by the rule that caused it.
  const totalsWithheld =
    totalsWithheldBecause(dealFilters) ??
    totalsRefusedBecause(stageTotalsQuery.data);
  const [pending, setPending] = useState<PendingAdvance | null>(null);
  // The deal that just closed, while the review is on offer for it. Separate
  // from `pending`, which is the question before the close; this is the offer
  // after one landed, and the close dialog is already gone by then.
  const [closed, setClosed] = useState<ClosedDeal | null>(null);
  // Bulk selection, by deal id. Cleared after any bulk run except for the rows
  // that refused, since every other row's version has moved.
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());

  const advance = useAdvanceDeal();

  const stages = effectivePipeline?.stages ?? [];
  const stageName = new Map(stages.map((stage) => [stage.id, stage.name]));
  // Every page walked so far, in one list. The board draws its columns from
  // this and the table renders it directly, so both surfaces grow together as
  // the reader asks for more.
  const loadedDeals = (dealsQuery.data?.pages ?? []).flatMap(
    (page) => page.data,
  );
  // Only ids the list holds now count as selected. A row that left the result
  // set (refetched away, filtered out, archived by this run) must not linger
  // as an invisible selection nobody can clear.
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

  const companiesQuery = useQuery({
    queryKey: ["companies"],
    queryFn: async () => {
      return unwrap(
        await api.GET("/companies", {
          params: { query: { limit: 50 } },
        }),
      );
    },
  });

  const partnerOptions = usePartnerOptions(companiesQuery.data?.data ?? []);
  // The channel catalog for the filter chip. Shared cache with the edit form's
  // own read, so opening one after the other costs a single request.
  const acquisitionSources = useAcquisitionSources().data;

  // Open-stage targets only: a deal is born open (INV-CLOSE-PAST twin rule);
  // won/lost are reached through the confirmed advance, never at create.
  const openStages = stages.filter((stage) => stage.semantic === "open");

  const { openDeal, cardDragHandlers, columnDropHandlers } =
    useBoardInteractions({ stages, loadedDeals, advance, setPending });

  // Shared between the board and the table surface: whichever view is
  // showing, the action lives in the surface's own header, not a wrap sibling.
  const createAction = openStages.length > 0 && (
    <DealCreateAction
      pipeline={effectivePipeline}
      cf={cf}
      openStages={openStages}
      partnerOptions={partnerOptions}
      startOpen={startCreating}
    />
  );

  const pipelineId = effectivePipeline?.id ?? "";
  // The board/table switch and the pipeline picker are shared, so the reader
  // sees the same dials either way.
  const tools = (
    <DealViewTools
      view={view}
      setView={setView}
      pipelines={pipelinesQuery.data ?? []}
      pipelineId={pipelineId}
      setPipelineId={setPipelineId}
      setQuery={setQuery}
    />
  );
  // Saving is not shared. A view holds a sort as well as its filters. The
  // board offers no way to see or change one: its order is the pipeline's
  // stage order. Saving from there would pin an ordering the reader never
  // chose into a view they will restore on the table.
  const saveView = view === "table" && (
    <SaveViewAction
      resource="deals"
      query={savableDealQuery(dealsListState.query, pipelineId)}
    />
  );
  const companyName = useAppliedCompanyName(query.filters.company_id);
  const dealChips = dealFilterChips(stages, t, companyName);
  const tagChips = useTagChips();
  const rowSelection = dealRowSelection({
    view,
    liveSelection,
    selectedRows,
    stages,
    setSelected,
    t,
  });

  // The board is the surface's alternate body, not a surface of its own. The
  // saved views, the chips and the archived toggle all describe the one query
  // both views read. A board that replaced the surface would take them along,
  // leaving a narrowed pipeline with nothing saying what narrowed it.
  //
  // It carries its own continuation control, because `bodyOwnsPaging` withholds
  // the surface's. That belongs to the paged grid this body does not draw, and
  // the board holds every card loaded so far at once. Its count still goes to
  // the surface's head (`bodyCount`). A reader looks for how much is here beside
  // the page's name; under the toolbar it would read as a caption on the dials.
  const boardBody =
    view === "board" ? (
      <DealBoardBody
        dealsQuery={dealsQuery}
        pipelinesQuery={pipelinesQuery}
        effectivePipeline={effectivePipeline}
        dealFilters={dealFilters}
        loadedDeals={loadedDeals}
        stageTotalsQuery={stageTotalsQuery}
        totalsWithheld={totalsWithheld}
        companies={companiesQuery.data?.data ?? []}
        companiesSettled={!companiesQuery.isPending}
        openDeal={openDeal}
        cardDragHandlers={cardDragHandlers}
        columnDropHandlers={columnDropHandlers}
      />
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
        action={
          <>
            <LegalReviewQueueLink />
            {createAction}
          </>
        }
        tools={tools}
        saveView={saveView}
        body={boardBody}
        bodyOwnsPaging={view === "board"}
        bodyCount={
          view === "board" &&
          dealsQuery.data &&
          translatePlural(locale, "board.count", loadedDeals.length, {
            count: formatNumber(loadedDeals.length, locale),
          })
        }
        // The pipeline picker is screen state, not a filter, so switching it
        // changes every row without touching `filters`. Naming it here is
        // what puts the reader back on page 1.
        scopeKey={effectivePipeline?.id ?? ""}
        dataChips={[...ownerChips, ...dealChips, ...tagChips]}
        dataViews={savedViews}
        selection={rowSelection}
        chips={dealSurfaceChips({
          partnerOptions,
          partnerApplied: query.filters.partner_company_id,
          acquisitionSources,
          retiredSuffix: t("deal.acquisitionRetired"),
        })}
        views={[...standardViews(viewerId, { sort: "" })]}
        emptyNote={mineEmptyNote({
          t,
          state: dealsListState,
          viewerId,
          unit: "unit.deals",
        })}
      />
      <ErrorLine error={advanceErrorBehind(advance.error, pending !== null)} />
      <ConfirmAdvanceModal
        pending={pending}
        onClose={() => setPending(null)}
        onConfirm={(input) => advance.mutateAsync(input)}
        onClosed={(deal, reason) => setClosed(closedDealOf(deal, reason))}
      />
      {/* Offered the moment a deal closes, from the list as from the record
          page: the reader who just said why is the one who can answer the
          rest. */}
      <CloseReviewOffer closed={closed} onDismiss={() => setClosed(null)} />
    </div>
  );
}
