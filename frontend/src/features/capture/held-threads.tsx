import { keepPreviousData, useInfiniteQuery } from "@tanstack/react-query";
import { Fragment, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import {
  Badge,
  Button,
  EmptyState,
  SearchField,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { CellStack } from "../../design-system/cellstack";
import { DataTable } from "../../design-system/datatable";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { useSettledValue } from "../../design-system/debouncedsearch";
import { EmailReference } from "../../design-system/emailreference";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { formatActivityWhen } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { startOfDayInZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useOpenEmail } from "../../lib/openemail";
import { unwrap } from "../../lib/problem";
import { useUrlParams } from "../../lib/urlstate";
import { useViewerZone } from "../../lib/viewerzone";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import {
  HELD_STATUSES,
  type HeldStatus,
  heldStatusLabel,
} from "./held-threads.counts";

import "./held-threads.css";

// What this mailbox is holding back from the team, and the one press that
// releases a thread.
//
// An owner could already do this from a record's timeline, one thread at a
// time. What they could not do is ask "what am I holding" — and during a
// classifier outage that is the only question worth asking, because every new
// thread lands pending and stays there until the model answers again.
//
// The caller's own mailbox and nobody else's. The endpoint has no admin view
// for the same reason this card has no filter by colleague: the rows name
// threads judged legal, personnel or personal, so a shared list would disclose
// exactly what holding them prevents.

type HeldThread = components["schemas"]["HeldThread"];

/** What the list is narrowed to. Every filter runs on the server. */
export type HeldFilters = Readonly<{
  q?: string;
  status?: HeldStatus;
  from?: string;
  to?: string;
  reason?: string;
  contact_id?: string;
}>;

// Why a thread is held, in the reader's words. A kind absent from the map is
// one the server learned to say and this screen has not.
const kindLabel: Record<string, MessageKey> = {
  legal: "heldThreads.kind.legal",
  financial_corporate: "heldThreads.kind.financialCorporate",
  personnel: "heldThreads.kind.personnel",
  personal: "heldThreads.kind.personal",
  security_incident: "heldThreads.kind.securityIncident",
  explicitly_confidential: "heldThreads.kind.explicitlyConfidential",
};

/** One seat's held threads, a page at a time, narrowed on the server. */
export function useHeldThreads(filters: HeldFilters = {}) {
  // One key for one question: an empty search is no search, so the card and
  // the unfiltered drawer share a read.
  const narrowed = { ...filters, q: filters.q || undefined };
  return useInfiniteQuery({
    queryKey: ["held-threads", narrowed],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/capture/held-threads", {
          params: {
            query: {
              cursor: pageParam,
              ...narrowed,
            },
          },
        }),
      ),
    // A server older than paging sent the whole list and no page.
    getNextPageParam: (last) => last.page?.next_cursor ?? undefined,
    // The drawer opens over the card's own read, so opening it is not a
    // second read of the same list.
    staleTime: 30_000,
    // A new search keeps the rows it replaces until its answer lands, so the
    // table never reads as empty while the server is still looking.
    placeholderData: keepPreviousData,
  });
}

/** The held list with its search and status filter, read on the server. */
export function HeldThreads() {
  const t = useT();
  const zone = useViewerZone();
  const [params] = useUrlParams();
  const contact = params.get("contact_id") ?? params.get("contact");
  const [typed, setTyped] = useState("");
  const [status, setStatus] = useState<HeldStatus | "">("");
  const [reason, setReason] = useState("");
  const [from, setFrom] = useState<ISODate | "">("");
  const [to, setTo] = useState<ISODate | "">("");
  const [browse, setBrowse] = useState(false);
  const q = useSettledValue(typed.trim());
  const active = Boolean(
    q || status || reason || from || to || contact || browse,
  );
  const held = useHeldThreads({
    q,
    status: status || undefined,
    reason: reason || undefined,
    contact_id: contact,
    from: from
      ? new Date(startOfDayInZone(from, zone)).toISOString()
      : undefined,
    to: to ? new Date(startOfDayInZone(to, zone, 1)).toISOString() : undefined,
  });
  const first = held.data?.pages[0];
  const rows = [
    ...new Map(
      (held.data?.pages ?? [])
        .flatMap((page) => page.data)
        .filter((row) => row.has_message && row.activity_id)
        .map((row) => [row.thread_key, row]),
    ).values(),
  ];
  return (
    <div className="form-stack">
      <p>{t("heldThreads.sub")}</p>
      <SearchField
        aria-label={t("heldThreads.search")}
        placeholder={t("heldThreads.search")}
        value={typed}
        onChange={(event) => setTyped(event.target.value)}
      />
      <div className="held-threads-filters">
        <Select
          aria-label={t("heldThreads.statusFilter")}
          value={status}
          options={[
            { value: "", label: t("heldThreads.statusAll") },
            ...HELD_STATUSES.map((value) => ({
              value,
              label: t(heldStatusLabel[value]),
            })),
          ]}
          onChange={(value) =>
            setStatus(HELD_STATUSES.find((status) => status === value) ?? "")
          }
        />
        <Select
          aria-label={t("heldThreads.reasonFilter")}
          value={reason}
          options={[
            { value: "", label: t("heldThreads.anyReason") },
            ...Object.entries(kindLabel).map(([value, key]) => ({
              value,
              label: t(key),
            })),
          ]}
          onChange={setReason}
        />
        <DateInput
          aria-label={t("heldThreads.from")}
          value={from}
          onChange={(event) => setFrom(event.target.value)}
        />
        <DateInput
          aria-label={t("heldThreads.to")}
          value={to}
          onChange={(event) => setTo(event.target.value)}
        />
      </div>
      {first && <BacklogCallout stalled={first.counts.stalled} />}
      {held.isError && <ErrorLine error={held.error} />}
      {!active ? (
        <Button onClick={() => setBrowse(true)}>
          {t("heldThreads.browse")}
        </Button>
      ) : rows.length === 0 ? (
        <EmptyState>{t("heldThreads.noMatch")}</EmptyState>
      ) : (
        <HeldThreadTable rows={rows} />
      )}
      {active && held.hasNextPage && (
        <Button
          pending={held.isFetchingNextPage}
          onClick={() => void held.fetchNextPage()}
        >
          {t("heldThreads.loadMore")}
        </Button>
      )}
    </div>
  );
}

function HeldThreadTable({ rows }: Readonly<{ rows: HeldThread[] }>) {
  const zone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const [openEmail, setOpenEmail] = useOpenEmail();
  return (
    <>
      <DataTable
        fold
        label={t("heldThreads.title")}
        rows={rows}
        rowKey={(row) => row.thread_key}
        columns={[
          {
            key: "subject",
            header: t("heldThreads.colThread"),
            grow: true,
            render: (row) => (
              <CellStack>
                <ThreadSubject row={row} onOpen={setOpenEmail} />
                <ThreadParticipants row={row} beside />
              </CellStack>
            ),
          },
          {
            key: "why",
            header: t("heldThreads.colWhy"),
            render: (row) => <WhyCell row={row} />,
          },
          {
            key: "when",
            header: t("heldThreads.colWhen"),
            render: (row) =>
              row.occurred_at
                ? formatActivityWhen(
                    row.occurred_at,
                    row.occurred_precision ?? "instant",
                    locale,
                    zone,
                  )
                : "—",
          },
          {
            key: "actions",
            header: t("table.actions"),
            headerHidden: true,
            fold: "end",
            render: (row) => (
              <Button
                disabled={!row.activity_id}
                onClick={() => row.activity_id && setOpenEmail(row.activity_id)}
              >
                {t("heldThreads.preview")}
              </Button>
            ),
          },
        ]}
      />
      <OpenEmailDrawer
        activityId={openEmail}
        zone={zone}
        onClose={() => setOpenEmail(null)}
      />
    </>
  );
}

/**
 * Whether the classifier is answering at all, derived from the rows already
 * fetched rather than read from a lane the reader may not query.
 *
 * `/ai/health` is the obvious source and the wrong one: it is admin-only, and
 * the closed RBAC object set carries no AI-runtime entry, so opening it to reps
 * is a closed-set change plus a product decision. This is also the truer answer
 * for the reader in front of it — their own backlog, not a global lane that may
 * be healthy while their mail sits.
 */
function BacklogCallout({ stalled }: Readonly<{ stalled: number }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (stalled === 0) {
    // Pending but not yet at the ceiling is ordinary latency, and saying "your
    // mail is stuck" about it would cry wolf every time a thread arrives.
    return null;
  }
  return (
    <Callout
      tone="warning"
      kind="event"
      title={t("heldThreads.backlogStalledTitle")}
    >
      {plural("heldThreads.backlogStalled", stalled, {
        count: formatNumber(stalled, locale),
      })}
    </Callout>
  );
}

// Why this thread is held: the classifier's conclusion, or the fact that it has
// not reached one yet.
//
// Pending is drawn as its own badge rather than as a missing kind. During an
// outage every new thread is pending, and a column of blanks would read as a
// classifier that judged nothing rather than one that has not answered.
function WhyCell({ row }: Readonly<{ row: HeldThread }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const asked = plural("heldThreads.attempts", row.attempts, {
    count: formatNumber(row.attempts, locale),
  });
  if (row.pending) {
    return (
      <CellStack>
        {/* `info`: pending below the ceiling is ordinary latency, which the
            stalled-backlog callout above already refuses to cry wolf about. */}
        <Badge tone="info">{t("heldThreads.pending")}</Badge>
        <span className="t-caption">{asked}</span>
      </CellStack>
    );
  }
  if (row.status === "unsure") {
    return (
      <CellStack>
        <Badge>{t("heldThreads.kind.unsure")}</Badge>
        <span className="t-caption">{asked}</span>
      </CellStack>
    );
  }
  const kindKey = row.kind ? kindLabel[row.kind] : undefined;
  return <Badge>{t(kindKey ?? "heldThreads.kind.other")}</Badge>;
}

// A held thread's subject as a citation, with no date beside the Arrived column.
// `activity_id` is gated, so an existing message without one is withheld.
function ThreadSubject({
  row,
  onOpen,
}: Readonly<{ row: HeldThread; onOpen: (activityId: string) => void }>) {
  const activityId = row.activity_id;
  return (
    <EmailReference
      subject={row.subject}
      withheld={!activityId}
      onOpen={activityId ? () => onOpen(activityId) : undefined}
    />
  );
}

// Who the thread is with, so a reader can tell one held thread from another.
// Beside the subject it is one line cut short; folded, a line of its own.
function ThreadParticipants({
  row,
  beside = false,
}: Readonly<{ row: HeldThread; beside?: boolean }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const shown = row.participants ?? [];
  if (shown.length === 0) {
    return null;
  }
  const more = (row.participant_count ?? shown.length) - shown.length;
  return (
    <span
      className={
        beside ? "t-caption held-threads-parties" : "held-threads-parties-all"
      }
    >
      <span className="held-threads-parties-list">
        {shown.map((party, at) => (
          <Fragment key={party.address}>
            {at > 0 && " "}
            <span className="held-threads-party">
              {party.contact_id ? (
                <EntityRef
                  kind="contact"
                  id={party.contact_id}
                  name={party.address}
                />
              ) : (
                party.address
              )}
              {at < shown.length - 1 && ","}
            </span>
          </Fragment>
        ))}
      </span>
      {more > 0 && (
        <span className="held-threads-parties-more">
          {plural("heldThreads.moreParticipants", more, {
            count: formatNumber(more, locale),
          })}
        </span>
      )}
    </span>
  );
}
