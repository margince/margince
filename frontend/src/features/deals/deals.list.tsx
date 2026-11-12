import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { usePageName } from "../../app/pagemeta";
import { BULK_MAX_ITEMS } from "../../components/bulkchangeapi";
import { searchCompanyTargets } from "../../components/companyform";
import { CreateAction } from "../../components/create";
import { currencyField } from "../../components/currencyfield";
import {
  type ObjectCustomFields,
  useObjectCustomFields,
} from "../../components/customfields.form";
import {
  type ListState,
  ListTable,
  useTagChips,
} from "../../components/listquery";
import { useOwnerChips } from "../../components/ownerdials";
import { projectOwnerField } from "../../components/projectownerfield";
import { mineEmptyNote, standardViews } from "../../components/recordlist";
import { useOwnerPicker } from "../../components/recordowners";
import { RefusedFiltersNote } from "../../components/refusedfiltersnote";
import { ErrorLine } from "../../design-system/errorline";
import { useRowSelection } from "../../design-system/listselection";
import { useLocale, useT } from "../../i18n";
import { useAcquisitionSources } from "../../lib/acquisitionsources.queries";
import { useCan } from "../../lib/capability";
import { maskRefusedDialOf } from "../../lib/dealranges";
import {
  totalsRefusedBecause,
  totalsWithheldBecause,
} from "../../lib/deals.queries";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import { usePipelines } from "../../lib/pipelines.queries";
import { throwProblem, unwrap } from "../../lib/problem";
import { useReReadMeWhen } from "../../lib/refusedfilters";
import { useViewerId } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import {
  SavedViewsFailure,
  SaveViewAction,
  useSavedViewTabs,
} from "../lists/savedviews";
import { LegalReviewQueueLink } from "../privacy/legalreviews";
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
import { acquisitionOptions } from "./deal360/dealcommercialfields";
import {
  dealProjectFields,
  resolveDealProject,
  useProjectsOfCompany,
} from "./dealproject";
import type { Deal, Pipeline, Stage } from "./deals";
import {
  BoardHeadCount,
  DealBoardBody,
  useAdvanceDeal,
  useBoardInteractions,
} from "./deals.board";
import { dealColumns } from "./deals.columns";
import {
  partnerFields,
  useNoPartnersYet,
  usePartnerOptions,
} from "./deals.fields";
import {
  dealRowSelection,
  dealViewDials,
  savableDealQuery,
  useDealChips,
  useDealScreenDials,
} from "./deals.listtools";
import { useSurfaceReads } from "./deals.surfacereads";
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
  const { locale } = useLocale();
  const noPartnersYet = useNoPartnersYet();
  const acquisitionSources = useAcquisitionSources().data;
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
      listCreate={{ open: startOpen }}
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
        currencyField(leading, locale),
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
        // Who works the deal, where it came from and the partner who brought
        // it are set at birth, not by editing afterwards: a deal left without
        // its source stays without it, and the win that pays a partner can
        // come before anybody revisits the record.
        ...projectOwnerField(t, projectOwner, {
          key: "owner_id",
          label: "list.owner",
          mode: "create",
        }),
        {
          key: "acquisition_source",
          label: "deal.acquisitionSource",
          type: "select",
          options: acquisitionOptions(t, acquisitionSources ?? [], null),
        },
        ...partnerFields(t, partnerOptions, { noneYet: noPartnersYet }),
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
  const readerZone = useViewerZone();
  const cf = useObjectCustomFields("deal", "create");
  const pipelinesQuery = usePipelines();
  const ownerChips = useOwnerChips();
  const viewerId = useViewerId();
  const savedViews = useSavedViewTabs("deals");
  const {
    query,
    setQuery,
    baseCurrency,
    withheld,
    setPipelineId,
    effectivePipeline,
    dealFilters,
    view,
    setView,
    refusedDials,
  } = useDealScreenDials({ pipelines: pipelinesQuery.data });
  const stages = effectivePipeline?.stages ?? [];
  // The board names its cards' companies from this page, and a reader without
  // company.read is refused it.
  const readsCompanies = useCan("company", "read");
  const companiesQuery = useQuery({
    queryKey: ["companies"],
    enabled: readsCompanies,
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
  const { dealChips, rangeChips, surfaceChips, nameRefusal } = useDealChips({
    stages,
    query,
    withheld,
    baseCurrency,
    partnerOptions,
    acquisitionSources,
  });
  // A refused dial the list can name is named, and no wider list is asked for.
  const roleRefusal = nameRefusal(refusedDials);
  const { dealsQuery, stageColumns, stageTotalsQuery, readError, listRead } =
    useSurfaceReads(
      dealFilters,
      stages,
      view,
      Boolean(roleRefusal),
      pipelinesQuery,
      query.perPage,
    );
  // A dial the role's mask refused, which a current snapshot would have
  // withheld. Reading /me again withholds it and the list reloads without it.
  const maskRefused = maskRefusedDialOf(readError);
  useReReadMeWhen(Boolean(maskRefused));
  const refusal = nameRefusal([...refusedDials, maskRefused]);
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

  const advance = useAdvanceDeal();

  const stageName = new Map(stages.map((stage) => [stage.id, stage.name]));
  const loadedDeals = (dealsQuery.data?.pages ?? []).flatMap(
    (page) => page.data,
  );
  // Cleared after any bulk run except for the rows that refused, since every
  // other row's version has moved.
  const chosen = useRowSelection({
    rows: loadedDeals,
    keyOf: (deal) => deal.id,
    limit: BULK_MAX_ITEMS,
  });

  const dealsListState: ListState<Deal> = {
    rows: loadedDeals,
    query,
    setQuery,
    isPending: listRead.isPending,
    isError: listRead.isError,
    error: listRead.error,
    refetch: () => listRead.refetch(),
    hasMore: dealsQuery.hasNextPage,
    loadMore: () => {
      dealsQuery.fetchNextPage();
    },
  };

  // Open-stage targets only: a deal is born open (INV-CLOSE-PAST twin rule);
  // won/lost are reached through the confirmed advance, never at create.
  const openStages = stages.filter((stage) => stage.semantic === "open");

  const { openDeal, cardDragHandlers, columnDropHandlers } =
    useBoardInteractions({
      stages,
      loadedDeals: stageColumns.deals,
      advance,
      setPending,
    });

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
  const { layout, viewRows } = dealViewDials({
    view,
    setView,
    pipelines: pipelinesQuery.data ?? [],
    pipelineId,
    setPipelineId,
    setQuery,
    t,
  });
  // Saving is not shared. A view holds a sort as well as its filters. The
  // board offers no way to see or change one: its order is the pipeline's
  // stage order. Saving from there would pin an ordering the reader never
  // chose into a view they will restore on the table.
  const saveView = view === "table" && (
    <SaveViewAction
      resource="deals"
      state={{ query: savableDealQuery(dealsListState.query, pipelineId) }}
    />
  );
  const listState: ListState<Deal> = refusal
    ? {
        ...dealsListState,
        rows: [],
        isPending: false,
        isError: false,
        hasMore: false,
      }
    : dealsListState;
  const tagChips = useTagChips();
  const rowSelection = dealRowSelection({
    view,
    chosen,
    stages,
    t,
  });

  // The board is the surface's body, so the dials that narrow it stay in view.
  // Each column pages itself, so `bodyOwnsPaging` withholds the grid's pager.
  const boardBody =
    view === "board" && !refusal ? (
      <DealBoardBody
        stageColumns={stageColumns}
        pipelinesQuery={pipelinesQuery}
        effectivePipeline={effectivePipeline}
        dealFilters={dealFilters}
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
        state={listState}
        unit="deals.unit"
        columns={dealColumns(t, locale, readerZone, stageName)}
        rowKey={(deal) => deal.id}
        rowRoute={(deal) => ({ screen: "deals", id: deal.id })}
        action={
          <>
            <LegalReviewQueueLink />
            {createAction}
          </>
        }
        layout={layout}
        viewRows={viewRows}
        saveView={saveView}
        notice={<SavedViewsFailure resource="deals" />}
        body={boardBody}
        bodyOwnsPaging={boardBody !== undefined}
        countScope={effectivePipeline?.name}
        bodyCount={
          boardBody && (
            <BoardHeadCount
              pipeline={effectivePipeline}
              stageColumns={stageColumns}
              stageTotalsQuery={stageTotalsQuery}
            />
          )
        }
        // The pipeline picker is screen state, not a filter, so switching it
        // changes every row without touching `filters`. Naming it here is
        // what puts the reader back on page 1.
        scopeKey={effectivePipeline?.id ?? ""}
        dataChips={[...ownerChips, ...dealChips, ...tagChips, ...rangeChips]}
        dataViews={savedViews}
        selection={rowSelection}
        chips={surfaceChips}
        views={[...standardViews(viewerId, { sort: "" })]}
        emptyNote={
          refusal ? (
            <RefusedFiltersNote
              message={t("deals.filterRefused", { dial: refusal.names })}
              dials={refusal.dials}
              names={refusal.names}
            />
          ) : (
            mineEmptyNote({
              t,
              state: dealsListState,
              viewerId,
              unit: "unit.deals",
            })
          )
        }
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
