// The leads list: the queue a rep works from, as a table or a board. The
// page a row opens lives in leads.tsx; the presentation both share sits in
// leadpresentation.tsx.
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { currentParams, useUrlParams } from "../app/urlstate";
import { Badge, Button, SegmentedControl } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useToast } from "../design-system/toast";
import { formatDateAbbrev, formatNumber } from "../format/format";
import { leadIdentityName } from "../format/leadname";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  ProblemError,
  QueryGate,
  throwProblem,
  useMe,
  useSorMode,
} from "./common";
import { CreateAction, type CreateField } from "./create";
import { useObjectCustomFields } from "./customfields.form";
import { useRoster } from "./entityref";
import { LeadBulkBar } from "./leadbulk";
import {
  LEAD_STATUS_FILTER_OPTIONS,
  LeadBoard,
  SlaBadge,
  StatusBadge,
  scoreFactorLabel,
  scoreTone,
  terminalBadge,
} from "./leadpresentation";
import {
  sourceFilterOptions,
  sourceLabelFor,
  sourcePickOptions,
  useLeadSettings,
  useLeadSources,
} from "./leadsources";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
  useOwnerChips,
} from "./listquery";
import {
  createdColumn,
  lastActivityColumn,
  mineEmptyNote,
  ownerColumn,
  standardViews,
} from "./recordlist";
import { SaveViewAction, useSavedViewTabs } from "./savedviews";
import "./leads.css";

type Lead = components["schemas"]["Lead"];
type CreateLeadRequest = components["schemas"]["CreateLeadRequest"];

async function fetchLeadsPage(
  query: ListQuery,
  cursor: string | null,
): Promise<ListPage<Lead>> {
  const { data, error } = await api.GET("/leads", {
    params: {
      query: {
        q: query.q || undefined,
        sort: query.sort || undefined,
        include_archived: query.includeArchived || undefined,
        cursor: cursor || undefined,
        limit: listFetchLimit(query.perPage),
        ...query.filters,
      },
    },
  });
  if (error) {
    // A LIST read's honest-error path only needs a message to render — the
    // dedupe "view existing" link is a create/update-only concern.
    throwProblem(error);
  }
  return {
    data: data.data,
    page: {
      next_cursor: data.page.next_cursor ?? null,
      has_more: data.page.has_more,
    },
  };
}

// Builds the create-lead request body: scalar fields trim to undefined when
// blank (never sent rather than sent empty). Lead email is a single string —
// not a repeatable list — so no rows channel is used here.
export function mapLeadBody(values: Record<string, string>): CreateLeadRequest {
  return {
    full_name: values.full_name?.trim() || undefined,
    email: values.email?.trim() || undefined,
    linkedin_url: values.linkedin_url?.trim() || undefined,
    title: values.title?.trim() || undefined,
    company_name: values.company_name?.trim() || undefined,
    owner_id:
      values.owner_id === UNASSIGNED_OWNER
        ? undefined
        : values.owner_id?.trim() || undefined,
    status: "new",
    source: values.source?.trim() || "manual",
  };
}

// The owner picker's "nobody yet" choice. A sentinel rather than the empty
// string because the field is required — the writer chooses the queue, they
// do not skip the question. The server reads an omitted owner_id as exactly
// that: an unassigned lead for routing or a claim to pick up.
export const UNASSIGNED_OWNER = "unassigned";

const leadCreateFields: CreateField[] = [
  { key: "full_name", label: "create.fullName", required: true },
  { key: "email", label: "create.email", type: "email" },
  { key: "linkedin_url", label: "create.linkedinUrl" },
  { key: "title", label: "create.personTitle" },
  { key: "company_name", label: "create.companyName" },
];

const leadStatusFilterOptions = LEAD_STATUS_FILTER_OPTIONS;

// The score bands a reader triages by. `min_score` is a floor, so each band
// names the bottom of a range rather than a bucket — "60+" and "80+" overlap
// on purpose, because that is what the parameter means.
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

async function createLead(
  values: Record<string, string>,
  customFields: Record<string, unknown>,
  t: (key: MessageKey) => string,
): Promise<Lead> {
  const body = mapLeadBody(values);
  const probes = [body.email, body.linkedin_url].filter(
    (value): value is string => Boolean(value),
  );
  for (const probe of probes) {
    const { data: matches, error: probeError } = await api.GET("/leads", {
      params: { query: { q: probe, limit: 10 } },
    });
    if (probeError) throwProblem(probeError, t);
    const normalized = probe.toLowerCase().replace(/\/$/, "");
    const existing = matches.data.find(
      (lead) =>
        lead.email?.toLowerCase() === normalized ||
        lead.linkedin_url?.toLowerCase().replace(/\/$/, "") === normalized,
    );
    if (existing) {
      throw new ProblemError(
        {
          code: "duplicate_lead",
          detail: t("lead.duplicateFound"),
          details: { existing_id: existing.id },
        },
        t,
      );
    }
  }
  const { data, error } = await api.POST("/leads", {
    body: { ...body, ...customFields },
  });
  if (error) {
    throwProblem(error, t);
  }
  return data;
}

export function LeadsScreen() {
  const me = useMe();
  return (
    // The page's own root, OUTSIDE the gate: inside it, only the loaded screen
    // carried the gutter and the pending bars drew against the scroller's edge.
    // The queue's classes ride it rather than an element of their own, because
    // `.wrap:has(> .lt)` is what gives a list screen its full height and a div
    // between the two would take that away.
    <div className="wrap lead-surface lead-queue">
      <QueryGate query={me}>
        {(session) => (
          <LeadsWorkbench
            viewerId={session.user.id}
            // An admin or manager opens on every lead — they run the queue; a
            // rep opens on their own.
            opensOnAll={session.roles.some(
              (role) =>
                role === "admin" || role === "manager" || role === "management",
            )}
          />
        )}
      </QueryGate>
    </div>
  );
}

// The segregation notice (AC-leads-1), said once under the header and
// dismissible per browser: it explains a rule, and a rule read once stays
// read.
const SEGREGATION_NOTE_KEY = "margince.leads.segregationNoteDismissed";

// The queue's own dial: board or table. Not a wire parameter, because /leads
// takes neither — it decides how the same rows are drawn.
const LEAD_VIEW_PARAM = "view";
const LEAD_SCREEN_DIALS: readonly string[] = [LEAD_VIEW_PARAM];

function LeadsWorkbench({
  viewerId,
  opensOnAll,
}: Readonly<{ viewerId: string; opensOnAll: boolean }>) {
  const ownerChips = useOwnerChips();
  const savedViews = useSavedViewTabs("leads");
  const roster = useRoster("user", true);
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const overlay = useSorMode() === "overlay";
  const leadSettings = useLeadSettings();
  const slaOn = leadSettings.data?.first_response_enabled === true;
  // Bulk selection, by lead id; cleared after any bulk run, since the rows
  // and their versions have moved.
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const cf = useObjectCustomFields("lead");
  const state = useListQuery<Lead>({
    key: "leads",
    initialSort: "",
    initialFilters: opensOnAll ? {} : { owner_id: viewerId },
    fetchPage: fetchLeadsPage,
    // Board or table is this screen's own, so the codec must not read it as a
    // lead filter: unheld it was spread onto `GET /leads?view=board`, counted
    // as a narrowing, and cleared by "clear filters" — which flipped the board
    // back to a table.
    screenDials: LEAD_SCREEN_DIALS,
  });
  const [noteDismissed, setNoteDismissed] = useState(
    () => window.localStorage.getItem(SEGREGATION_NOTE_KEY) === "1",
  );
  // What the last bulk write did to a row the current view then stopped
  // showing — a successful assign out of "Mine" must never look like nothing
  // happened (or like a failure).
  const toast = useToast();
  const showingMine = state.query.filters.owner_id === viewerId;
  // Only ids the list currently holds count as selected: a row that left the
  // result set (refetched away, paged out, filtered out) must not linger as
  // an invisible selection nobody can clear.
  const selectedRows = state.rows.filter((lead) => selected.has(lead.id));
  const liveSelection = new Set(selectedRows.map((lead) => lead.id));
  // The board writes status, which the mirror refuses (a lead's lifecycle is
  // not a field write-back), so overlay gets the table and no toggle.
  // The board/table choice is a dial like the filters beside it, so it lives in
  // the address: a reader can link to the board, and it survives a reload. It
  // is the screen's OWN name rather than a wire one, because which of the two
  // is drawn changes nothing about which leads exist — the same split the deals
  // screen makes.
  const [params, setParams] = useUrlParams();
  const view: "table" | "board" =
    params.get(LEAD_VIEW_PARAM) === "board" ? "board" : "table";
  const setView = (next: "table" | "board") => {
    const dials = new Map(currentParams());
    if (next === "board") {
      dials.set(LEAD_VIEW_PARAM, next);
    } else {
      dials.delete(LEAD_VIEW_PARAM);
    }
    setParams(dials);
  };
  const sources = useLeadSources();
  const ownerOptions = [
    { value: viewerId, label: t("lead.assignToMe") },
    { value: UNASSIGNED_OWNER, label: t("lead.unassigned") },
    ...(roster.data ?? [])
      .filter((entry) => !("is_agent" in entry && entry.is_agent))
      .filter((entry) => entry.id !== viewerId)
      .map((entry) => ({
        value: entry.id,
        label:
          ("display_name" in entry ? entry.display_name : null) ?? entry.id,
      })),
  ];

  return (
    // A fragment, not a div: the queue's own stack is on the page root above
    // the gate (`LeadsScreen`), and an element here would sit between `.wrap`
    // and the `.lt` whose full height depends on being its direct child.
    <>
      {!noteDismissed && (
        <Callout tone="info">
          {t("lead.segregation")}{" "}
          <Button
            small
            data-testid="lead-segregation-dismiss"
            onClick={() => {
              window.localStorage.setItem(SEGREGATION_NOTE_KEY, "1");
              setNoteDismissed(true);
            }}
          >
            {t("lead.segregationDismiss")}
          </Button>
        </Callout>
      )}
      <ListTable
        emptyNote={mineEmptyNote({ t, state, viewerId, unit: "unit.leads" })}
        // The board renders INSIDE the surface, so the search, the chips and
        // the saved views stay above it. A board that replaced the surface
        // took the filter bar with it, leaving the reader looking at a
        // narrowed answer with no way to see or change what narrowed it.
        body={
          view === "board" && !overlay ? (
            <LeadBoard
              rows={state.rows}
              onMoved={() => state.refetch()}
              hasMore={state.hasMore}
              loadMore={state.loadMore}
            />
          ) : undefined
        }
        bodyOwnsPaging={view === "board" && !overlay}
        state={state}
        unit="unit.leads"
        action={
          <CreateAction
            label={t("create.lead")}
            invalidate="leads"
            screen="leads"
            create={(values) => createLead(values, cf.toBody(values), t)}
            resolveExisting={(_code, id) => ({ screen: "leads", id })}
            fields={[
              ...leadCreateFields,
              {
                key: "owner_id",
                label: "lead.ownerLabel",
                type: "select",
                required: true,
                options: ownerOptions,
              },
              {
                key: "source",
                label: "lead.source",
                type: "select",
                required: true,
                // The active administered list, in the administrator's order;
                // "Created manually" is the default because it is first.
                options: sourcePickOptions(sources.data?.data, null, t),
              },
              ...cf.formFields,
            ]}
          />
        }
        columns={[
          {
            key: "name",
            header: t("people.name"),
            cell: (lead: Lead) => {
              const terminal = terminalBadge(lead.status);
              return (
                <span>
                  <strong>{leadIdentityName(lead) || t("lead.unnamed")}</strong>
                  {lead.company_name && (
                    <span className="t-caption"> · {lead.company_name}</span>
                  )}
                  {terminal && (
                    <Badge tone={terminal.tone}>{t(terminal.label)}</Badge>
                  )}
                </span>
              );
            },
            // `full_name` is in the server's lead sort vocabulary, so the
            // header is live and the attribute joins the sort menu — the same
            // A–Z route the contacts list offers.
            sort: "full_name",
            fixed: true,
          },
          {
            key: "score",
            header: t("lead.score"),
            cell: (lead: Lead) => (
              <span
                style={{
                  display: "flex",
                  gap: "var(--space-1)",
                  flexWrap: "wrap",
                }}
              >
                <Badge tone={scoreTone(lead.score)}>
                  {formatNumber(lead.score, locale)}
                </Badge>
                <span className="t-caption">
                  {lead.score_reason
                    ? scoreFactorLabel(lead.score_reason, t)
                    : t("lead.scoreNoSignals")}
                </span>
              </span>
            ),
            sort: "score",
            numeric: true,
          },
          {
            key: "status",
            header: t("lead.status"),
            cell: (lead: Lead) => (
              <span
                style={{
                  display: "inline-flex",
                  gap: "var(--space-1)",
                  alignItems: "center",
                }}
              >
                <StatusBadge status={lead.status} />
                <SlaBadge state={lead.sla_state} />
              </span>
            ),
          },
          {
            key: "nextTask",
            header: t("lead.nextTask"),
            cell: (lead: Lead) => (
              <span className="t-caption">
                {lead.next_task_subject ?? t("lead.noNextTask")}
                {lead.open_task_count
                  ? ` · ${t("lead.openTaskCount", {
                      count: formatNumber(lead.open_task_count, locale),
                    })}`
                  : ""}
                {lead.next_task_due_at
                  ? ` · ${formatDateAbbrev(lead.next_task_due_at, locale, viewerZone())}`
                  : ""}
              </span>
            ),
          },
          lastActivityColumn<Lead>(t, locale, recordZone),
          {
            key: "source",
            header: t("lead.source"),
            cell: (lead: Lead) => (
              <span className="t-caption">
                {sourceLabelFor(lead, sources.data?.data, t)}
              </span>
            ),
          },
          ownerColumn<Lead>(t),
          createdColumn<Lead>(t, locale, recordZone),
        ]}
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
                toast.dismiss();
                const moved = outcomes.filter((o) => !o.error);
                if (
                  action.kind === "assign" &&
                  showingMine &&
                  action.ownerId !== viewerId &&
                  moved.length > 0
                ) {
                  // The verb goes through `action` rather than into the
                  // message: the region draws it, so it is one control on one
                  // ground rather than a `Button` hand-placed on the toast's
                  // dark plate, and the region withdraws the message once it
                  // has been pressed.
                  toast.show(
                    t("lead.assignedAway", {
                      names: moved.map((o) => o.name).join(", "),
                      owner: action.ownerName,
                    }),
                    {
                      action: {
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
            key: "source",
            label: "lead.filterSource",
            allLabel: "lead.filterSourceAll",
            options: sourceFilterOptions(sources.data, t),
          },
        ]}
        // The one ownership dial every record list carries (DM-VOCAB-OWN-1):
        // mine, my team's, the unowned queue.
        dataChips={ownerChips}
        views={[
          ...standardViews(viewerId, { sort: "", mineFirst: !opensOnAll }),
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
          { label: "list.viewAZ", sort: "full_name" },
        ]}
        // The reader's own saved narrowings, beside the presets above. Leads
        // was the one record list without them, while the contract has
        // carried `leads` in the saved-view vocabulary all along.
        dataViews={savedViews}
        tools={
          <>
            {/* Board or table is how the SAME rows are drawn, so it belongs
                with the drawing dials rather than above the surface — the
                slot the deals screen's pipeline picker already uses. The
                mirror refuses the board's status write, so overlay gets the
                table and no toggle. */}
            {!overlay && (
              <SegmentedControl
                options={["table", "board"] as const}
                value={view}
                onChange={setView}
                labels={{
                  table: t("deals.viewTable"),
                  board: t("deals.viewBoard"),
                }}
              />
            )}
            <SaveViewAction resource="leads" query={state.query} />
          </>
        }
      />
    </>
  );
}
