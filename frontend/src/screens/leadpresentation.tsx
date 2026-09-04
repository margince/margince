import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { navigate } from "../app/router";
import { Badge, Button } from "../design-system/atoms";
import {
  type BoardColumn,
  type BoardRecord,
  PipelineBoard,
} from "../design-system/composed";
import { formatNumber } from "../format/format";
import { leadIdentityName } from "../format/leadname";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import {
  LEAD_STATUS_COUNTS_KEY,
  leadTerminalKey,
  leadWriteKeys,
} from "./leadkeys";
import { DisqualifyDialog } from "./leads.disqualify";
import { QualifyDialog } from "./leads.qualify";
import { sourceLabelFor } from "./leadsources";

type Lead = components["schemas"]["Lead"];

export function scoreTone(score: number): "success" | "warn" | undefined {
  if (score >= 60) return "success";
  if (score >= 40) return "warn";
  return undefined;
}

export const LEAD_STATUS_FILTER_OPTIONS = [
  { value: "new", label: "lead.statusNew" },
  { value: "contacted", label: "lead.statusContacted" },
  { value: "engaged", label: "lead.statusEngaged" },
  { value: "promoted", label: "lead.statusPromoted" },
  { value: "disqualified", label: "lead.statusDisqualified" },
] as const;

/**
 * The catalogue key for a status, shared by the badge and the record page's
 * readings strip. Exported because the strip states the SAME word the badge
 * does — a second spelling here is how one lead comes to read "Qualified" in
 * a pill and "promoted" in the slot beside it.
 */
export function leadStatusLabel(status: Lead["status"]): MessageKey | null {
  return (
    LEAD_STATUS_FILTER_OPTIONS.find((option) => option.value === status)
      ?.label ?? null
  );
}

// The ladder's colours: a new lead is quiet, contact is in motion, engaged
// is the signal worth noticing; the terminal pair is muted.
function statusTone(status: Lead["status"]): "accent" | "success" | undefined {
  switch (status) {
    case "contacted":
      return "accent";
    case "engaged":
      return "success";
    default:
      return undefined;
  }
}

export function StatusBadge({ status }: Readonly<{ status: Lead["status"] }>) {
  const t = useT();
  const label = leadStatusLabel(status);
  return <Badge tone={statusTone(status)}>{label ? t(label) : status}</Badge>;
}

export function SlaBadge({ state }: Readonly<{ state: Lead["sla_state"] }>) {
  const t = useT();
  if (state === "breached") {
    return <Badge tone="danger">{t("lead.sla.breached")}</Badge>;
  }
  if (state === "at_risk") {
    return <Badge tone="warn">{t("lead.sla.atRisk")}</Badge>;
  }
  return null;
}

const LEAD_BOARD_STAGES = [
  { stage: "new", label: "lead.statusNew" },
  { stage: "contacted", label: "lead.statusContacted" },
  { stage: "engaged", label: "lead.statusEngaged" },
] as const;

// The two ends of the pipeline. A lead reaching either is ARCHIVED, which is
// why they are not in the list above: the board's own page excludes archived
// rows, so these columns have no cards to count and their figures come from
// the leads-by-status report instead.
//
// They are not statuses a card can simply be MOVED to. Qualifying promotes the
// lead into a person (and maybe a deal) and disqualifying records a reason, so
// each drop opens the dialog that collects what the transition needs — the
// server refuses a bare status PATCH into either, and rightly.
const LEAD_TERMINAL_STAGES = [
  { stage: "promoted", label: "lead.statusPromoted", dialog: "qualify" },
  {
    stage: "disqualified",
    label: "lead.statusDisqualified",
    dialog: "disqualify",
  },
] as const;

type TerminalDialog = (typeof LEAD_TERMINAL_STAGES)[number]["dialog"];

// How many leads each status holds, INCLUDING the archived terminal ones.
//
// The board's own page cannot answer this: it excludes archived rows, so the
// two terminal columns would read zero however many leads had passed through
// them. The report is the one lead read that counts them (leads-by-status).
//
// Keyed under ["leads"] so the invalidation every lead mutation already fires
// refreshes these figures with the cards — a qualified lead has to leave the
// open count and arrive in the terminal one in the same beat, or the board
// shows a card that is gone and a count that never moved.
function useLeadStatusCounts() {
  return useQuery({
    queryKey: LEAD_STATUS_COUNTS_KEY,
    queryFn: async () => {
      const { data, error } = await api.POST("/reports/{report}", {
        params: { path: { report: "leads-by-status" } },
        body: {
          group_by: ["status"],
          aggregates: [{ fn: "count", as: "leads" }],
        },
      });
      if (error) throwProblem(error);
      const counts = new Map<string, number>();
      for (const row of data.rows) {
        const status = row.status;
        const leads = row.leads;
        if (typeof status === "string" && typeof leads === "number") {
          counts.set(status, leads);
        }
      }
      return counts;
    },
  });
}

// The leads sitting in ONE terminal column, fetched only while it is open.
//
// A collapsed column states a figure and needs no rows; an expanded one that
// showed none would be a column claiming twelve leads and displaying nothing.
// So the rows are fetched when the reader opens it and not before — these are
// archived leads, and every other read on this screen is right to hide them.
type TerminalStatus = (typeof LEAD_TERMINAL_STAGES)[number]["stage"];

function useTerminalLeads(status: TerminalStatus, enabled: boolean) {
  return useQuery({
    queryKey: leadTerminalKey(status),
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/leads", {
        params: {
          query: { status, include_archived: true, limit: 50 },
        },
      });
      if (error) throwProblem(error);
      return data.data;
    },
  });
}

function LeadCard({
  lead,
  onOpen,
  dragHandlers,
}: Readonly<{
  lead: Lead;
  onOpen: (lead: Lead) => void;
  dragHandlers?: {
    draggable: true;
    onDragStart: (event: React.DragEvent) => void;
    onDragEnd: () => void;
  };
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <button
      type="button"
      className="deal-card"
      data-lead={lead.id}
      onClick={() => onOpen(lead)}
      {...dragHandlers}
    >
      <span className="deal-name">
        {leadIdentityName(lead) || t("lead.unnamed")}
      </span>
      {lead.company_name && (
        <span className="deal-org">
          <span className="deal-org-name">{lead.company_name}</span>
        </span>
      )}
      <span className="deal-meta">
        <Badge tone={scoreTone(lead.score)}>
          {t("lead.score")}: {formatNumber(lead.score, locale)}
        </Badge>
        <SlaBadge state={lead.sla_state} />
        {lead.title && <span>{lead.title}</span>}
      </span>
      <span className="deal-meta">
        <span>{sourceLabelFor(lead, undefined, t)}</span>
        <span>
          {lead.next_task_subject ?? t("lead.noNextTask")}
          {lead.open_task_count
            ? ` · ${t("lead.openTaskCount", {
                count: formatNumber(lead.open_task_count, locale),
              })}`
            : ""}
        </span>
      </span>
    </button>
  );
}

export function LeadBoard({
  rows,
  onMoved,
  hasMore,
  loadMore,
}: Readonly<{
  rows: Lead[];
  onMoved: () => void;
  hasMore: boolean;
  loadMore: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const dragging = useRef<string | null>(null);
  const lastDragEnd = useRef(0);
  // Collapsed by DEFAULT, and the two of them together. These columns are
  // where leads go to stop being work: a reader opening the board wants the
  // three they still act on, and the terminal pair folded to a count each.
  const [openTerminal, setOpenTerminal] = useState<TerminalStatus | null>(null);
  const [pending, setPending] = useState<{
    lead: Lead;
    dialog: TerminalDialog;
  } | null>(null);
  const counts = useLeadStatusCounts();
  const terminalRows = useTerminalLeads(
    openTerminal ?? "promoted",
    openTerminal !== null,
  );
  const move = useMutation({
    // No single record: one mutation instance serves every card on the board,
    // and which lead is moving is known only at mutate() time, not here.
    mutationKey: ["lead-edit"],
    mutationFn: async (moved: {
      id: string;
      version?: number;
      status: "new" | "contacted" | "engaged";
    }) => {
      const { data, error } = await api.PATCH("/leads/{id}", {
        // Refused rather than sent unpinned: a row the server did not version is
        // one this client can make no concurrency claim about.
        params: {
          path: { id: moved.id },
          ...ifMatch(requireVersion(moved.version)),
        },
        body: { status: moved.status },
      });
      if (error) throwProblem(error, t);
      return data;
    },
    // The moved lead is named on BOTH arms. The board reads
    // `["leads", query]` and the detail page reads the sibling `["lead", id]`,
    // which prefix invalidation does not walk sideways to: naming only the
    // list left a detail page open behind the board showing the status the
    // reader had just dragged away from. The error arm owes the same — a
    // refused move means the row on screen may no longer be what the server
    // holds, which is exactly when a stale detail page misleads.
    onSuccess: (_moved, variables) => {
      for (const key of leadWriteKeys(variables.id)) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      onMoved();
    },
    onError: (_failure, variables) => {
      for (const key of leadWriteKeys(variables.id)) {
        queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });

  const live = rows.filter(
    (lead) =>
      lead.status === "new" ||
      lead.status === "contacted" ||
      lead.status === "engaged",
  );
  const columns: BoardColumn<BoardRecord>[] = LEAD_BOARD_STAGES.map((stage) => {
    const held = live.filter((lead) => lead.status === stage.stage);
    return {
      stage: stage.stage,
      label: t(stage.label),
      count: held.length,
      deals: held.map((lead) => ({ id: lead.id, name: "" })),
    };
  });
  // The terminal pair, whose count comes from the report and whose cards are
  // fetched only for the one that is open.
  const opened = openTerminal !== null ? (terminalRows.data ?? []) : [];
  for (const stage of LEAD_TERMINAL_STAGES) {
    const isOpen = openTerminal === stage.stage;
    const held = isOpen
      ? opened.filter((lead) => lead.status === stage.stage)
      : [];
    columns.push({
      stage: stage.stage,
      label: t(stage.label),
      // The REPORT's figure, not the rows'. A collapsed column has no rows at
      // all, and an open one holds only the page that was fetched — either
      // would understate a column whose whole population is archived.
      count: counts.data?.get(stage.stage) ?? 0,
      collapsed: !isOpen,
      deals: held.map((lead) => ({ id: lead.id, name: "" })),
    });
  }
  const leadsById = new Map(
    [...live, ...opened].map((lead) => [lead.id, lead]),
  );

  // WHERE a dropped card lands, as one decision.
  //
  // A terminal column FIRST. Neither of those transitions is a status change:
  // qualifying promotes the lead into a person and maybe a deal, disqualifying
  // records a reason, and the server refuses a bare status PATCH into either.
  // The drop opens the dialog that collects what the transition needs; only an
  // open stage is a move.
  function dropLeadOn(stage: string, lead: Lead) {
    const terminal = LEAD_TERMINAL_STAGES.find((s) => s.stage === stage);
    if (terminal) {
      if (lead.status !== terminal.stage) {
        setPending({ lead, dialog: terminal.dialog });
      }
      return;
    }
    const target = LEAD_BOARD_STAGES.find((s) => s.stage === stage);
    if (target && lead.status !== target.stage) {
      move.mutate({ id: lead.id, version: lead.version, status: target.stage });
    }
  }

  return (
    <>
      {move.isError && (
        <p className="t-caption" style={{ color: "var(--danger)" }}>
          {problemMessageOf(move.error, t)}
        </p>
      )}
      {rows.length > 0 && live.length === 0 && (
        <p className="t-caption">{t("lead.boardTerminalOnly")}</p>
      )}
      <PipelineBoard
        variant="plain"
        columns={columns}
        countLabel={(count) =>
          t("lead.boardCount", { count: formatNumber(count, locale) })
        }
        renderCard={(card) => {
          const lead = leadsById.get(card.id);
          if (!lead) return null;
          return (
            <LeadCard
              lead={lead}
              onOpen={(opened) => {
                if (Date.now() - lastDragEnd.current > 250) {
                  navigate({ screen: "leads", id: opened.id });
                }
              }}
              dragHandlers={{
                draggable: true,
                onDragStart: (event) => {
                  dragging.current = lead.id;
                  event.dataTransfer.setData("text/plain", lead.id);
                },
                onDragEnd: () => {
                  dragging.current = null;
                  lastDragEnd.current = Date.now();
                },
              }}
            />
          );
        }}
        onToggleColumn={(column) => {
          // Only the terminal pair folds. Clicking an open stage's head must
          // not collapse a column the reader works out of — and finding the
          // stage rather than casting is what keeps the state's type honest.
          const terminal = LEAD_TERMINAL_STAGES.find(
            (stage) => stage.stage === column.stage,
          );
          if (!terminal) return;
          setOpenTerminal((open) =>
            open === terminal.stage ? null : terminal.stage,
          );
        }}
        columnDropHandlers={(column) => ({
          onDragOver: (event) => {
            event.preventDefault();
            event.currentTarget.classList.add("droptarget");
          },
          onDragLeave: (event) => {
            event.currentTarget.classList.remove("droptarget");
          },
          onDrop: (event) => {
            event.preventDefault();
            event.currentTarget.classList.remove("droptarget");
            const id =
              event.dataTransfer.getData("text/plain") || dragging.current;
            dragging.current = null;
            lastDragEnd.current = Date.now();
            const lead = id ? leadsById.get(id) : undefined;
            if (lead) dropLeadOn(column.stage, lead);
          },
        })}
      />
      {hasMore && (
        <Button small onClick={loadMore}>
          {t("list.loadMore")}
        </Button>
      )}
      {/* Keyed by lead so a half-filled deal block for one never carries to
          the next, the same reason the detail screen keys its pair. */}
      {pending?.dialog === "qualify" && (
        <QualifyDialog
          key={`qualify-${pending.lead.id}`}
          lead={pending.lead}
          open
          onClose={() => setPending(null)}
          onQualified={() => {
            setPending(null);
            onMoved();
          }}
        />
      )}
      {pending?.dialog === "disqualify" && (
        <DisqualifyDialog
          key={`disqualify-${pending.lead.id}`}
          lead={pending.lead}
          open
          onClose={() => setPending(null)}
          onDisqualified={() => {
            setPending(null);
            onMoved();
          }}
        />
      )}
    </>
  );
}

export function promoteEligible(lead: Lead): boolean {
  return isOpenStatus(lead.status) && Boolean(lead.email);
}

// The terminal badge a lead status earns (null = live/open, no badge). A lead
// is archived iff it is promoted or disqualified; keying the label off the
// status — not a bare archived_at — is what stops a promoted lead reading
// "Disqualified". Exhaustive over the four statuses: a new value is a compile
// error here, not a silently-unlabelled row.
export function terminalBadge(
  status: Lead["status"],
): { label: MessageKey; tone: "warn" } | null {
  switch (status) {
    case "disqualified":
      return { label: "lead.disqualified", tone: "warn" };
    case "promoted":
      return { label: "record.archived", tone: "warn" };
    case "new":
    case "contacted":
    case "engaged":
      return null;
  }
}

// The open ladder, as the page's open-step predicate reads it.
export const LEAD_OPEN_STATUSES = ["new", "contacted", "engaged"] as const;
type LeadOpenStatus = (typeof LEAD_OPEN_STATUSES)[number];

export function isOpenStatus(status: Lead["status"]): status is LeadOpenStatus {
  return status === "new" || status === "contacted" || status === "engaged";
}

// The message key for one §3 factor name, falling back to the raw name for
// a factor the catalog does not know yet.
export function scoreFactorLabel(
  factor: string,
  t: ReturnType<typeof useT>,
): string {
  const key = `lead.factor.${factor}` as MessageKey;
  const label = t(key);
  return label === key ? factor : label;
}
