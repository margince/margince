import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CellStack } from "../design-system/cellstack";
import { DataTable } from "../design-system/datatable";
import { EmailReference } from "../design-system/emailreference";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useThreadAudience } from "./audienceservice";
import { problemMessageOf, QueryGate, unwrap } from "./common";
import { EntityRef } from "./entityref";
import { useOpenEmail } from "./openemail";
import { VerdictPassNote } from "./verdictpass";
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
type HeldThreadList = components["schemas"]["HeldThreadListResponse"];

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

export function HeldThreadsCard() {
  const t = useT();
  const query = useQuery({
    queryKey: ["held-threads"],
    queryFn: async () => {
      return unwrap(await api.GET("/capture/held-threads"));
    },
  });

  return (
    <Panel title={t("heldThreads.title")}>
      <PanelBody>
        <PanelIntro>{t("heldThreads.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("heldThreads.title")}>
        {(list) =>
          list.data.length === 0 ? (
            // Nothing held is a reading, not an absent list: "my mailbox is
            // withholding nothing right now" is what an owner opens this to see.
            <PanelBody>
              <EmptyState>{t("heldThreads.empty")}</EmptyState>
            </PanelBody>
          ) : (
            <>
              <HeldThreadNotices list={list} />
              <HeldThreadTable rows={list.data} />
            </>
          )
        }
      </QueryGate>
    </Panel>
  );
}

function HeldThreadNotices({ list }: Readonly<{ list: HeldThreadList }>) {
  const stalled = list.data.some(
    (row) => row.pending && row.attempts >= backlogStallAttempts,
  );
  // Only where a row waits on the pass. A thread judged personnel waits for
  // nothing, so naming the next pass would promise a change.
  const waiting = list.data.some((row) => row.pending);
  if (!stalled && !waiting) {
    return null;
  }
  return (
    <PanelBody className="form-stack">
      <BacklogCallout rows={list.data} />
      {waiting && (
        <VerdictPassNote clock={list.thread_verdict} subject="threads" />
      )}
    </PanelBody>
  );
}

function HeldThreadTable({ rows }: Readonly<{ rows: HeldThread[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = viewerZone();
  const toast = useToast();
  // Which thread another owner still holds, so a release that changed nothing
  // says so instead of looking broken. Keyed by thread, because two rows can be
  // released before either answer arrives.
  const [heldByOthers, setHeldByOthers] = useState<Record<string, number>>({});

  // The same write the timeline's own control performs, through the same
  // service. It was spelled twice before — here and in the action cluster,
  // byte for byte — and what differs is only what a release refreshes and
  // that this queue says so out loud.
  const release = useThreadAudience({
    invalidate: () => [["held-threads"]],
    onSettled: ({ threadKey, outcome }) => {
      if (outcome && !outcome.shared) {
        // Somebody else imported this message too and has not released their
        // own contribution. Saying which is the difference between a control
        // that looks broken and one that reports what happened.
        setHeldByOthers((held) => ({
          ...held,
          [threadKey]: outcome.held_by_others ?? 0,
        }));
        return;
      }
      setHeldByOthers((held) => {
        const { [threadKey]: _released, ...rest } = held;
        return rest;
      });
      toast.show(t("heldThreads.released"));
    },
  });

  const stillHeld = Object.entries(heldByOthers);
  // The message a held row opens. This table is the one place a reader meets
  // these threads, so the drawer belongs to it.
  const [openEmail, setOpenEmail] = useOpenEmail();
  const base = useId();
  const erasedId = (row: HeldThread) => `${base}-${row.thread_key}`;
  const subjectId = (row: HeldThread) => `${base}-subject-${row.thread_key}`;
  return (
    <>
      <DataTable<HeldThread>
        bleed
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
              // Three states, not two: a subject, a message that carries none,
              // and no message at all, which still governs later mail.
              <span className="held-threads-subject" id={subjectId(row)}>
                {row.has_message ? (
                  <ThreadSubject row={row} onOpen={setOpenEmail} />
                ) : (
                  <CellStack>
                    <span>{t("heldThreads.noSubject")}</span>
                    <span className="t-caption" id={erasedId(row)}>
                      {t("heldThreads.nothingToShare")}
                    </span>
                  </CellStack>
                )}
              </span>
            ),
          },
          {
            key: "parties",
            header: t("heldThreads.colParties"),
            grow: true,
            render: (row) => <ThreadParticipants row={row} limit={1} />,
            folded: (row) =>
              row.participants?.length ? (
                <ThreadParticipants row={row} />
              ) : undefined,
          },
          {
            key: "why",
            header: t("heldThreads.colWhy"),
            render: (row) => <WhyCell row={row} />,
          },
          {
            key: "when",
            header: t("heldThreads.colWhen"),
            align: "end",
            render: (row) =>
              row.occurred_at
                ? formatDateTime(row.occurred_at, locale, zone)
                : "—",
          },
          {
            key: "actions",
            header: t("table.actions"),
            headerHidden: true,
            align: "end",
            fold: "end",
            render: (row) => (
              <Button
                // The release endpoint works on the seat's messages on the
                // thread, so with none left it could only answer not-found.
                reasonId={row.has_message ? undefined : erasedId(row)}
                aria-describedby={subjectId(row)}
                pending={
                  release.isPending &&
                  release.variables?.threadKey === row.thread_key
                }
                onClick={() =>
                  release.mutate({ threadKey: row.thread_key, share: true })
                }
              >
                {t("heldThreads.release")}
              </Button>
            ),
          },
        ]}
      />
      {(release.isError || stillHeld.length > 0) && (
        <PanelBody className="form-stack">
          {release.isError && (
            <Callout
              tone="danger"
              kind="outcome"
              title={t("heldThreads.releaseFailed")}
            >
              {problemMessageOf(release.error, t)}
            </Callout>
          )}
          {/* One notice listing every thread, so releasing several rows adds
              no stack of identical bands. */}
          {stillHeld.length > 0 && (
            <Callout kind="outcome" title={t("heldThreads.stillHeldTitle")}>
              <ul>
                {stillHeld.map(([threadKey, owners]) => (
                  <li key={threadKey}>
                    {plural("heldThreads.heldByOthers", owners, {
                      count: formatNumber(owners, locale),
                    })}
                  </li>
                ))}
              </ul>
            </Callout>
          )}
        </PanelBody>
      )}
      {/* One drawer over the table, at its level rather than inside a row:
          two mounted dialogs would be two `aria-modal` elements. */}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
      />
    </>
  );
}

// backlogStallAttempts is when a pending thread has been asked about as often as
// it ever will be.
//
// The ledger claims a row only while `attempts < 2` (PendingMaxAttempts), so 2
// is the ceiling and a threshold above it can never be crossed — the first
// version of this used 3 and rendered nothing, ever. A thread sitting at the
// ceiling and still pending is one the lane has stopped answering: a healthy
// pass would have resolved it, and a spent one would have retired it to `unsure`
// and taken it out of this list.
const backlogStallAttempts = 2;

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
function BacklogCallout({ rows }: Readonly<{ rows: HeldThread[] }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const stalled = rows.filter(
    (row) => row.pending && row.attempts >= backlogStallAttempts,
  );
  if (stalled.length === 0) {
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
      {plural("heldThreads.backlogStalled", stalled.length, {
        count: formatNumber(stalled.length, locale),
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
// Beside the subject the first address stands for the rest, which are counted.
// Folded, the line has the row's width for every address it was given.
function ThreadParticipants({
  row,
  limit,
}: Readonly<{ row: HeldThread; limit?: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const given = row.participants ?? [];
  if (given.length === 0) {
    return null;
  }
  const shown = given.slice(0, limit);
  const more = (row.participant_count ?? given.length) - shown.length;
  return (
    <span
      className={
        limit === undefined
          ? "held-threads-parties-all"
          : "held-threads-parties"
      }
    >
      <span className="held-threads-parties-list">
        {shown.map((party, at) => (
          <span key={party.address}>
            {at > 0 && ", "}
            {party.contact_id ? (
              <EntityRef
                kind="contact"
                id={party.contact_id}
                name={party.address}
              />
            ) : (
              party.address
            )}
          </span>
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
