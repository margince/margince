// The leads list: the queue a rep works from, as a table or a board. The
// page a row opens lives in leads.tsx; the presentation both share sits in
// leadpresentation.tsx.

import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { usePageName } from "../../app/pagemeta";
import { useRecordOwnerOptions } from "../../components/assigneepicker";
import { CreateAction } from "../../components/create";
import { useObjectCustomFields } from "../../components/customfields.form";
import { useRosterHint, useRosterPartial } from "../../components/entityref";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "../../components/listquery";
import {
  mineEmptyNote,
  standardViews,
  useOwnerTagChips,
} from "../../components/recordlist";
import { RefusedFiltersNote } from "../../components/refusedfiltersnote";
import { SegmentedControl } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { QueryGate } from "../../design-system/querystates";
import { useOwnToast } from "../../design-system/toast";
import { industryOptions } from "../../format/industry";
import { leadIdentityName } from "../../format/leadname";
import { useLocale, useT } from "../../i18n";
import {
  originFilterOptions,
  originPickOptions,
  useLeadOrigins,
} from "../../lib/leadorigins";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { useMe } from "../../lib/session";
import { readStored, STORAGE_KEYS, writeStored } from "../../lib/storage";
import { listQueryParams } from "../../lib/tagfilter";
import { replaceDial, useUrlParams } from "../../lib/urlstate";
import { SaveViewAction, useSavedViewTabs } from "../lists/savedviews";
import { LeadBulkBar } from "./leadbulk";
import { useLeadSettings } from "./leadhandling";
import { LEAD_STATUS_FILTER_OPTIONS, LeadBoard } from "./leadpresentation";
import { leadColumns } from "./leads.columns";
import {
  leadCompanyChips,
  namedRefusedDials,
  refusedDialNames,
  useRefusalOf,
  useWithheldLeadDials,
} from "./leads.companydials";
import { leadIdentityFields } from "./leads.contactoffers";
import "./leads.css";
import { leadPartnerCreateFields, useLeadPartnerOptions } from "./leadpartner";
import { createLead, UNASSIGNED_OWNER } from "./leads.list.queries";

export { mapLeadBody, UNASSIGNED_OWNER } from "./leads.list.queries";

export type Lead = components["schemas"]["Lead"];

const leadStatusFilterOptions = LEAD_STATUS_FILTER_OPTIONS;

// The score bands a reader triages by. `min_score` is a floor, so each band
// names the bottom of a range, not a bucket. "60+" and "80+" overlap, because
// that is what the parameter means.
const LEAD_SCORE_BANDS = [
  { value: "80", label: "lead.filterScoreHot" },
  { value: "60", label: "lead.filterScoreWarm" },
  { value: "40", label: "lead.filterScoreCool" },
] as const;

const LEAD_SLA_STATES = [
  { value: "breached", label: "lead.sla.breached" },
  { value: "at_risk", label: "lead.sla.atRisk" },
  { value: "within_target", label: "lead.sla.withinTarget" },
] as const;

export async function fetchLeadsPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Lead>> {
  const data = unwrap(
    await api.GET("/leads", {
      params: {
        query: {
          q: query.q || undefined,
          sort: query.sort || undefined,
          include_archived: query.includeArchived || undefined,
          cursor: cursor || undefined,
          limit: listFetchLimit(query.perPage),
          ...listQueryParams(query.filters),
        },
      },
    }),
  );
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
      total: data.page.total,
    },
  };
}

export function LeadsScreen() {
  const t = useT();
  const me = useMe();
  return (
    // The page's own root, OUTSIDE the gate: inside it, only the loaded screen
    // carried the gutter and the pending bars drew against the scroller's edge.
    // The queue's classes ride it rather than an element of their own, because
    // `.wrap:has(> .lt)` is what gives a list screen its full height and a div
    // between the two would take that away.
    <div className="wrap lead-surface lead-queue">
      <QueryGate query={me} pendingLabel={t("nav.leads")}>
        {(session) => (
          <LeadsWorkbench
            viewerId={session.user.id}
            // A seat scoped past its own records opens on every lead — they run
            // the queue; a seat scoped to its own opens on those.
            //
            // The opening FILTER, not a permission: every seat holding the lead
            // grant may read any lead (auth/tableclass.go), so this decides
            // which view the queue starts in and nothing about what it may
            // fetch. The row scope the server computed, though, rather than the
            // role keys it came from — those were a second reading of the same
            // policy, and a custom role, or a seeded role whose grants an
            // operator edits, opens the wrong view while the server answers
            // correctly.
            //
            // Written as the POSITIVE test, so an absent authorization block
            // opens on "mine". `!== "own"` would read a missing answer as
            // permission to open on everything, which is the wrong direction to
            // be wrong in — and the block is optional on this response.
            opensOnAll={
              session.authorization?.row_scope === "team" ||
              session.authorization?.row_scope === "all"
            }
          />
        )}
      </QueryGate>
    </div>
  );
}

// The queue's own dial: board or table. Not a wire parameter, because /leads
// takes neither — it decides how the same rows are drawn.
const LEAD_VIEW_PARAM = "view";

const LEAD_SCREEN_DIALS: readonly string[] = [LEAD_VIEW_PARAM];

function LeadsWorkbench({
  viewerId,
  opensOnAll,
}: Readonly<{ viewerId: string; opensOnAll: boolean }>) {
  const ownerChips = useOwnerTagChips();
  const pageName = usePageName("leads");
  const savedViews = useSavedViewTabs("leads");
  const assignable = useRecordOwnerOptions();
  const ownerHint = useRosterHint(
    assignable.roster,
    useRosterPartial("user", true),
  );
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const leadSettings = useLeadSettings();
  const slaOn = leadSettings.data?.first_response_enabled === true;
  // Bulk selection, by lead id; cleared after any bulk run, since the rows
  // and their versions have moved.
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const cf = useObjectCustomFields("lead", "create");
  const withheld = useWithheldLeadDials();
  const listQuery = useListQuery<Lead>({
    key: "leads",
    initialSort: "",
    initialFilters: opensOnAll ? {} : { owner_id: viewerId },
    fetchPage: fetchLeadsPage,
    // A refused dial the list can name is not asked for; it is named instead.
    enabled: (query) => namedRefusedDials(query, withheld).length === 0,
    // Board or table is this screen's own, so the codec must not read it as a
    // lead filter: unheld it was spread onto `GET /leads?view=board`, counted
    // as a narrowing, and cleared by "clear filters" — which flipped the board
    // back to a table.
    screenDials: LEAD_SCREEN_DIALS,
  });
  const { refused, state } = useRefusalOf(listQuery, withheld);
  const refusedNames = refusedDialNames(refused, t);
  // The segregation notice, said once under the header: it explains a rule,
  // and a rule read once stays read until its reader signs out.
  const [noteDismissed, setNoteDismissed] = useState(
    () => readStored(STORAGE_KEYS.segregationNoteDismissed) === "1",
  );
  // What the last bulk write did to a row the current view then stopped
  // showing — a successful assign out of "Mine" must never look like nothing
  // happened (or like a failure).
  const assignedAway = useOwnToast({ leavesWithCaller: true });
  const showingMine = state.query.filters.owner_id === viewerId;
  // Only ids the list currently holds count as selected: a row that left the
  // result set (refetched away, paged out, filtered out) must not linger as
  // an invisible selection nobody can clear.
  const selectedRows = state.rows.filter((lead) => selected.has(lead.id));
  const liveSelection = new Set(selectedRows.map((lead) => lead.id));
  // The board/table choice is a dial like the filters beside it, so it lives in
  // the address: a reader can link to the board, and it survives a reload. It
  // is the screen's OWN name rather than a wire one, because which of the two
  // is drawn changes nothing about which leads exist — the same split the deals
  // screen makes.
  const [params] = useUrlParams();
  const view: "table" | "board" =
    params.get(LEAD_VIEW_PARAM) === "board" ? "board" : "table";
  const setView = (next: "table" | "board") =>
    replaceDial(LEAD_VIEW_PARAM, next === "board" ? next : undefined);
  const origins = useLeadOrigins();
  const partners = useLeadPartnerOptions();
  const ownerOptions = [
    { value: viewerId, label: t("lead.assignToMe") },
    { value: UNASSIGNED_OWNER, label: t("lead.unassigned") },
    ...assignable.options.filter((option) => option.value !== viewerId),
  ];

  return (
    // A fragment, not a div: the queue's own stack is on the page root above
    // the gate (`LeadsScreen`), and an element here would sit between `.wrap`
    // and the `.lt` whose full height depends on being its direct child.
    <>
      {!noteDismissed && (
        <Callout
          kind="standing"
          title={t("lead.segregationTitle")}
          dismiss={{
            label: t("lead.segregationDismiss"),
            onDismiss: () => {
              writeStored(STORAGE_KEYS.segregationNoteDismissed, "1");
              setNoteDismissed(true);
            },
          }}
        >
          {t("lead.segregation")}
        </Callout>
      )}
      <ListTable
        title={pageName}
        emptyNote={
          refused.length > 0 ? (
            <RefusedFiltersNote
              message={t("lead.filterRefused", { dial: refusedNames })}
              dials={refused}
              names={refusedNames}
            />
          ) : (
            mineEmptyNote({ t, state, viewerId, unit: "unit.leads" })
          )
        }
        // The board renders INSIDE the surface, so the search, the chips and
        // the saved views stay above it. A board that replaced the surface
        // took the filter bar with it, leaving the reader looking at a
        // narrowed answer with no way to see or change what narrowed it.
        body={
          view === "board" && refused.length === 0 ? (
            <LeadBoard
              rows={state.rows}
              onMoved={() => state.refetch()}
              hasMore={state.hasMore}
              loadMore={state.loadMore}
            />
          ) : undefined
        }
        bodyOwnsPaging={view === "board" && refused.length === 0}
        state={state}
        unit="unit.leads"
        action={
          <CreateAction
            label={t("create.lead")}
            invalidate="leads"
            screen="leads"
            create={(values) => createLead(values, cf.toBody(values), t)}
            // A contact holding the lead's email or profile opens as the
            // contact: a new opportunity with them is a deal on that contact.
            resolveExisting={(code, id) =>
              code === "contact_holds_email"
                ? { screen: "contacts", id }
                : { screen: "leads", id }
            }
            fields={[
              ...leadIdentityFields,
              {
                key: "industry",
                label: "create.industry",
                type: "select",
                options: industryOptions(t, locale),
              },
              {
                key: "owner_id",
                label: "lead.ownerLabel",
                type: "select",
                required: true,
                options: ownerOptions,
                hint: ownerHint,
              },
              {
                key: "origin",
                label: "lead.origin",
                type: "select",
                // Not required: a lead nobody has classified scores neutral.
                // The active administered list, in the administrator's order.
                options: [
                  { value: "", label: t("lead.originNone") },
                  ...originPickOptions(origins.data, null),
                ],
              },
              { key: "origin_detail", label: "lead.originDetail" },
              ...leadPartnerCreateFields(partners, t),
              ...cf.formFields,
            ]}
          />
        }
        columns={leadColumns(t, locale, recordZone, origins.data, partners)}
        rowKey={(lead) => lead.id}
        selection={{
          selected: liveSelection,
          // A closed lead takes no writes (STATE-4a): no checkbox, no verb.
          selectable: (lead) => !lead.archived_at,
          onToggle: (lead) =>
            setSelected((prev) => {
              const next = new Set(prev);
              if (next.has(lead.id)) {
                next.delete(lead.id);
              } else {
                next.add(lead.id);
              }
              return next;
            }),
          label: (lead) =>
            t("lead.bulkSelectRow", {
              // The id, not the word: this is an accessible NAME, and every
              // unnamed row would otherwise announce as the same control.
              name: leadIdentityName(lead) || lead.id,
            }),
          bar: (
            <LeadBulkBar
              leads={selectedRows}
              // The rows that went through leave the selection; the ones that
              // refused stay in it, named, so the reader can retry them once
              // the list has refetched their versions.
              onDone={(outcomes, action) => {
                setSelected(
                  new Set(outcomes.filter((o) => o.error).map((o) => o.id)),
                );
                // Each run says its own thing; a sentence about the last one
                // must not stand beside this one's rows.
                assignedAway.withdraw();
                const moved = outcomes.filter((o) => !o.error);
                if (
                  action.kind === "assign" &&
                  showingMine &&
                  action.ownerId !== viewerId &&
                  moved.length > 0
                ) {
                  // The verb goes through `action`: the region draws it as one
                  // control on one ground, not a `Button` hand-placed on the
                  // toast's dark plate, and withdraws the message once pressed.
                  assignedAway.show(
                    t("lead.assignedAway", {
                      names: moved.map((o) => o.name).join(", "),
                      owner: action.ownerName,
                    }),
                    {
                      action: {
                        kind: "open",
                        label: t("list.showAll"),
                        onAct: () =>
                          state.setQuery((q) => {
                            const { owner_id: _mine, ...rest } = q.filters;
                            return { ...q, filters: rest };
                          }),
                      },
                    },
                  );
                }
              }}
            />
          ),
        }}
        rowRoute={(lead) => ({ screen: "leads", id: lead.id })}
        chips={[
          {
            key: "status",
            label: "lead.filterStatus",
            allLabel: "lead.filterStatusAll",
            options: leadStatusFilterOptions.map((option) => ({ ...option })),
          },
          {
            key: "min_score",
            label: "lead.filterScore",
            allLabel: "lead.filterScoreAll",
            options: LEAD_SCORE_BANDS.map((band) => ({ ...band })),
          },
          ...(slaOn
            ? [
                {
                  key: "sla_state",
                  label: "lead.filterSla" as const,
                  allLabel: "lead.filterSlaAll" as const,
                  options: LEAD_SLA_STATES.map((state) => ({ ...state })),
                },
              ]
            : []),
          {
            key: "origin",
            label: "lead.filterOrigin",
            allLabel: "lead.filterOriginAll",
            options: originFilterOptions(
              origins.data,
              t("lead.filterOriginRetired"),
            ),
          },
        ]}
        // The one ownership dial every record list carries (DM-VOCAB-OWN-1):
        // mine, my team's, the unowned queue.
        dataChips={[...ownerChips, ...leadCompanyChips(t, locale, withheld)]}
        views={[
          ...standardViews(viewerId, { sort: "", mineFirst: !opensOnAll }),
          // The unassigned queue as a VIEW, not only a chip. It was reachable
          // by opening the owner dial and picking a value, which is a thing
          // you find if you already know it is there; a lead nobody owns is
          // the one a queue exists to surface.
          //
          // Oldest first, deliberately against the other views' work-queue
          // order: what makes an unassigned lead urgent is how long it has sat
          // there with nobody answering it, and the newest arrival is the one
          // that can wait.
          {
            label: "lead.viewUnassigned",
            sort: "created_at",
            filters: { unassigned: "true" },
          },
          {
            label: "lead.viewNewUnassigned",
            sort: "created_at",
            filters: { status: "new", unassigned: "true" },
          },
          { label: "lead.viewNew", sort: "", filters: { status: "new" } },
          {
            label: "lead.viewNeedsFollowUp",
            sort: "",
            filters: { status: "contacted" },
          },
          {
            label: "lead.viewEngaged",
            sort: "",
            filters: { status: "engaged" },
          },
          {
            label: "list.viewHot",
            sort: "-score",
            filters: { min_score: "80" },
          },
          // The overdue queue (formulas §18.1) exists only while the
          // installation tracks a first-response target: off, the view would
          // be a tab that can never hold a row.
          ...(slaOn
            ? [
                {
                  label: "list.viewOverdue" as const,
                  sort: "-score",
                  filters: { sla_state: "breached" },
                },
              ]
            : []),
        ]}
        // The reader's own saved narrowings, beside the presets above. Leads
        // was the one record list without them, while the contract has
        // carried `leads` in the saved-view vocabulary all along.
        dataViews={savedViews}
        tools={
          // Board or table is how the SAME rows are drawn, so it belongs with
          // the drawing dials rather than above the surface — the slot the
          // deals screen's pipeline picker already uses.
          <SegmentedControl
            options={["table", "board"] as const}
            value={view}
            onChange={setView}
            labels={{
              table: t("deals.viewTable"),
              board: t("deals.viewBoard"),
            }}
          />
        }
        saveView={<SaveViewAction resource="leads" query={state.query} />}
      />
    </>
  );
}
