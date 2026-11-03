import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import type { ListQuery } from "../../components/listquery";
import { Badge, Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import {
  type BoardColumn,
  type BoardRecord,
  PipelineBoard,
} from "../../design-system/pipelineboard";
import { LoadMoreButton } from "../../design-system/querystates";
import { formatNumber } from "../../format/format";
import { leadIdentityName } from "../../format/leadname";
import { translatePlural, useLocale, useT } from "../../i18n";
import { leadWriteKeys } from "../../lib/leadkeys";
import { originLabelFor, useLeadOrigins } from "../../lib/leadorigins";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import {
  terminalCountsRefusedBecause,
  terminalCountsWithheldBecause,
  useLeadStatusCounts,
  useTerminalLeads,
} from "./leadboard.queries";
import { LeadCompanyName } from "./leadcompany";
import { openTaskCountLabel } from "./leadopentasks";
import type { Lead } from "./leadpresentation";
import { DisqualifyDialog } from "./leads.disqualify";
import { QualifyDialog } from "./leads.qualify";

export function scoreTone(score: number): "success" | "warning" | undefined {
  if (score >= 60) return "success";
  if (score >= 40) return "warning";
  return undefined;
}

export function SlaBadge({ state }: Readonly<{ state: Lead["sla_state"] }>) {
  const t = useT();
  if (state === "breached") {
    return <Badge tone="danger">{t("lead.sla.breached")}</Badge>;
  }
  if (state === "at_risk") {
    return <Badge tone="warning">{t("lead.sla.atRisk")}</Badge>;
  }
  return null;
}

const LEAD_BOARD_STAGES = [
  { stage: "new", label: "lead.statusNew" },
  { stage: "contacted", label: "lead.statusContacted" },
  { stage: "engaged", label: "lead.statusEngaged" },
] as const;

// The two ends of the pipeline, kept out of the list above. A lead reaching
// either is archived, and the board's own page excludes archived rows. So these
// columns have no cards to count, and their figures come from leads-by-status.
//
// A card is not moved to them. Qualifying promotes the lead into a contact (and
// maybe a deal), and disqualifying records a reason. The server refuses a bare
// status PATCH into either, so each drop opens the dialog that collects them.
const LEAD_TERMINAL_STAGES = [
  { stage: "promoted", label: "lead.statusPromoted", dialog: "qualify" },
  {
    stage: "disqualified",
    label: "lead.statusDisqualified",
    dialog: "disqualify",
  },
] as const;

type TerminalDialog = (typeof LEAD_TERMINAL_STAGES)[number]["dialog"];

// The lead a terminal decision is waiting on. `open` outlives the close so the
// copy still names it, and `seq` keys the dialog so a second drop asks clean.
type Ask = { lead: Lead; dialog: TerminalDialog; open: boolean; seq: number };

type TerminalStatus = (typeof LEAD_TERMINAL_STAGES)[number]["stage"];

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
  const origins = useLeadOrigins().data;
  // A div with a stretched opener, as the deal card has. A company link inside
  // a button would be invalid markup and would not take its own click.
  return (
    <div className="deal-card" data-lead={lead.id} {...dragHandlers}>
      <button
        type="button"
        className="deal-name deal-open lead-card-open"
        onClick={() => onOpen(lead)}
      >
        {leadIdentityName(lead) || t("lead.unnamed")}
      </button>
      {(lead.company_id || lead.company_name?.trim()) && (
        <span className="deal-company">
          <span className="deal-company-name deal-company-link">
            <LeadCompanyName lead={lead} />
          </span>
        </span>
      )}
      <span>
        <Badge tone={scoreTone(lead.score)}>
          {t("lead.score")}: {formatNumber(lead.score, locale)}
        </Badge>
        <SlaBadge state={lead.sla_state} />
        {lead.title && <span>{lead.title}</span>}
      </span>
      <span>
        <span>{originLabelFor(lead, origins) ?? t("lead.originUnset")}</span>
        <span>
          {lead.next_task_subject ?? t("lead.noNextTask")}
          {lead.open_task_count
            ? ` · ${openTaskCountLabel(locale, lead.open_task_count)}`
            : ""}
        </span>
      </span>
    </div>
  );
}

export function LeadBoard({
  query,
  rows,
  onMoved,
  hasMore,
  loadMore,
}: Readonly<{
  /** The queue's dials, which the terminal columns' counts and cards follow. */
  query: ListQuery;
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
  // Collapsed by default, and the two of them together. Leads in these columns
  // are no longer work. A reader opening the board wants the three stages they
  // still act on, and the terminal pair folded to a count each.
  const [openTerminal, setOpenTerminal] = useState<TerminalStatus | null>(null);
  const [pending, setPending] = useState<Ask | null>(null);
  const counts = useLeadStatusCounts(query);
  const withheld =
    terminalCountsWithheldBecause(query) ??
    terminalCountsRefusedBecause(counts.data);
  const terminalRows = useTerminalLeads(
    query,
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
      return unwrap(
        await api.PATCH("/leads/{id}", {
          // Refused rather than sent unpinned: a row the server did not version is
          // one this client can make no concurrency claim about.
          params: {
            path: { id: moved.id },
            ...ifMatch(requireVersion(moved.version)),
          },
          body: { status: moved.status },
        }),
        t,
      );
    },
    // The moved lead is named on both arms. The board reads `["leads", query]`
    // and the detail page reads the sibling `["lead", id]`, which prefix
    // invalidation does not reach. Naming only the list would leave a detail
    // page behind the board showing the status the reader dragged away from.
    // On error the row on screen may differ from the server's, so both refresh.
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
  const opened =
    openTerminal !== null
      ? (terminalRows.data?.pages.flatMap((page) => page.data) ?? [])
      : [];
  for (const stage of LEAD_TERMINAL_STAGES) {
    const isOpen = openTerminal === stage.stage;
    const held = isOpen
      ? opened.filter((lead) => lead.status === stage.stage)
      : [];
    columns.push({
      stage: stage.stage,
      label: t(stage.label),
      // The report's figure, not the rows'. A collapsed column has no rows, and
      // an open one holds only the fetched page. Either would understate a
      // column whose whole population is archived.
      count: counts.data?.get(stage.stage) ?? 0,
      sumHidden: withheld !== undefined,
      sumHiddenReason: withheld && t(withheld),
      collapsed: !isOpen,
      deals: held.map((lead) => ({ id: lead.id, name: "" })),
    });
  }
  const leadsById = new Map(
    [...live, ...opened].map((lead) => [lead.id, lead]),
  );

  // Where a dropped card lands, as one decision.
  //
  // A terminal column is checked first, since neither of its transitions is a
  // status change. The server refuses a bare status PATCH into either. The drop
  // opens the dialog that collects what the transition needs; only an open
  // stage is a move.
  function dropLeadOn(stage: string, lead: Lead) {
    // The source has to be a live lead. The cards refuse to drag a terminal
    // one, and this refuses to act on one too. A drop carries an id through a
    // dataTransfer string, so the guard belongs where the decision is taken.
    if (!isOpenStatus(lead.status)) return;
    const terminal = LEAD_TERMINAL_STAGES.find((s) => s.stage === stage);
    if (terminal) {
      // No "is it already there" test: the guard above has already ruled out
      // every terminal status, and the compiler says so.
      const seq = (pending?.seq ?? 0) + 1;
      setPending({ lead, dialog: terminal.dialog, open: true, seq });
      return;
    }
    const target = LEAD_BOARD_STAGES.find((s) => s.stage === stage);
    if (target && lead.status !== target.stage) {
      move.mutate({ id: lead.id, version: lead.version, status: target.stage });
    }
  }

  return (
    <>
      <ErrorLine error={move.error} />
      {rows.length > 0 && live.length === 0 && (
        <p>{t("lead.boardTerminalOnly")}</p>
      )}
      {/* The two reads behind the terminal columns, when they fail. A failed
          report renders as 0 and a failed row read as an empty column, and
          both read as fact. "Nobody was ever disqualified" is a very
          different statement from "we could not ask". */}
      {counts.isError && (
        <ErrorLine
          actions={
            <Button variant="ghost" onClick={() => counts.refetch()}>
              {t("common.retry")}
            </Button>
          }
        >
          {t("lead.boardCountsUnavailable")}
        </ErrorLine>
      )}
      {terminalRows.isError && (
        <ErrorLine
          actions={
            <Button variant="ghost" onClick={() => terminalRows.refetch()}>
              {t("common.retry")}
            </Button>
          }
        >
          {t(
            withheld
              ? "lead.boardTerminalRowsUnavailableNoCount"
              : "lead.boardTerminalRowsUnavailable",
          )}
        </ErrorLine>
      )}
      <PipelineBoard
        variant="plain"
        columns={columns}
        columnExtras={(column) =>
          column.stage === openTerminal ? (
            <LoadMoreButton query={terminalRows} />
          ) : null
        }
        countLabel={(count) =>
          translatePlural(locale, "lead.boardCount", count, {
            count: formatNumber(count, locale),
          })
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
              // A terminal lead does not drag. It is archived, and every
              // destination refuses it. UpdateLead reads live rows only, so a
              // drop on an open stage returns 409, and the other terminal
              // dialog's own mutation rejects it too. Offering a gesture that
              // fails is worse than not offering it.
              dragHandlers={
                isOpenStatus(lead.status)
                  ? {
                      draggable: true,
                      onDragStart: (event) => {
                        dragging.current = lead.id;
                        event.dataTransfer.setData("text/plain", lead.id);
                      },
                      onDragEnd: () => {
                        dragging.current = null;
                        lastDragEnd.current = Date.now();
                      },
                    }
                  : undefined
              }
            />
          );
        }}
        onToggleColumn={(column) => {
          // Only the terminal pair folds. Clicking an open stage's head must
          // not collapse a column the reader works out of. Finding the stage,
          // rather than casting, keeps the state's type correct.
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
      <LoadMoreButton
        query={{
          hasNextPage: hasMore,
          isFetchingNextPage: false,
          fetchNextPage: loadMore,
        }}
      />
      {/* Keyed by lead so a half-filled deal block for one never carries to
          the next, the same reason the detail screen keys its pair. */}
      {pending?.dialog === "qualify" && (
        <QualifyDialog
          key={`qualify-${pending.lead.id}-${pending.seq}`}
          lead={pending.lead}
          open={pending.open}
          onClose={() => setPending({ ...pending, open: false })}
          onQualified={() => {
            setPending({ ...pending, open: false });
            onMoved();
          }}
        />
      )}
      {pending?.dialog === "disqualify" && (
        <DisqualifyDialog
          key={`disqualify-${pending.lead.id}-${pending.seq}`}
          lead={pending.lead}
          open={pending.open}
          onClose={() => setPending({ ...pending, open: false })}
          onDisqualified={() => {
            setPending({ ...pending, open: false });
            onMoved();
          }}
        />
      )}
    </>
  );
}

// The open ladder, as the page's open-step predicate reads it.
export const LEAD_OPEN_STATUSES = ["new", "contacted", "engaged"] as const;

type LeadOpenStatus = (typeof LEAD_OPEN_STATUSES)[number];

export function isOpenStatus(status: Lead["status"]): status is LeadOpenStatus {
  return status === "new" || status === "contacted" || status === "engaged";
}
