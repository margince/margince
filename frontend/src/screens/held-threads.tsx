import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, DataTable, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { EmailReference } from "../design-system/emailreference";
import { OpenEmailDrawer } from "../design-system/openemaildrawer";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useThreadAudience } from "./audienceservice";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { useOpenEmail } from "./openemail";

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

// Why a thread is held, in the reader's words. A kind absent from the map is
// one the server learned to say and this screen has not — it falls back to the
// raw token rather than rendering nothing, so a new kind arrives as something
// to name instead of a blank cell.
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
      const { data, error } = await api.GET("/capture/held-threads");
      if (error) throwProblem(error);
      return data;
    },
  });

  return (
    <Panel title={t("heldThreads.title")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("heldThreads.sub")}</p>
        <QueryGate query={query}>
          {(list) =>
            list.data.length === 0 ? (
              // Nothing held is a READING, not an absent list: "my mailbox is
              // withholding nothing right now" is exactly what an owner opens
              // this card to confirm.
              <EmptyState>{t("heldThreads.empty")}</EmptyState>
            ) : (
              <>
                <BacklogCallout rows={list.data} />
                <HeldThreadTable rows={list.data} />
              </>
            )
          }
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function HeldThreadTable({ rows }: Readonly<{ rows: HeldThread[] }>) {
  const t = useT();
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
  return (
    <>
      <DataTable<HeldThread>
        label={t("heldThreads.title")}
        rows={rows}
        rowKey={(row) => row.thread_key}
        columns={[
          {
            key: "subject",
            header: t("heldThreads.colThread"),
            render: (row) =>
              // A thread whose opening message was erased has no subject to
              // show. It is still held, and saying so beats an empty cell that
              // reads as a message sent with a blank subject line.
              // Three states, not two: a subject, a message that carries
              // none, and no message at all. Collapsing the last two would tell
              // a reader their evidence was destroyed when it is sitting there
              // unnamed.
              row.has_message ? (
                <ThreadSubject row={row} onOpen={setOpenEmail} />
              ) : (
                <span className="t-meta">{t("heldThreads.noSubject")}</span>
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
              row.occurred_at ? (
                formatDateTime(row.occurred_at, locale, zone)
              ) : (
                <span className="t-meta">—</span>
              ),
          },
          {
            key: "actions",
            header: t("heldThreads.colActions"),
            render: (row) => (
              <Button
                small
                // A verdict outlives the message it was raised about, and the
                // release endpoint works on the seat's MESSAGES on the thread —
                // with none left it answers not-found. Offering the verb anyway
                // would be a control whose only outcome is an error, so it says
                // why instead. The row stays listed: the verdict still governs
                // what a later message on this thread inherits.
                reason={
                  row.has_message ? undefined : t("heldThreads.nothingToShare")
                }
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
      {release.isError && (
        <Callout tone="danger" live="alert">
          {problemMessageOf(release.error, t)}
        </Callout>
      )}
      {stillHeld.map(([threadKey, owners]) => (
        <Callout key={threadKey} tone="info">
          {t("heldThreads.heldByOthers", {
            count: formatNumber(owners, locale),
          })}
        </Callout>
      ))}
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
    <Callout tone="warn" live="status">
      {t("heldThreads.backlogStalled", {
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
  const { locale } = useLocale();
  if (row.pending) {
    return (
      <span className="cell-stack">
        <Badge tone="warn">{t("heldThreads.pending")}</Badge>
        <span className="t-meta">
          {t("heldThreads.attempts", {
            count: formatNumber(row.attempts, locale),
          })}
        </span>
      </span>
    );
  }
  const kindKey = row.kind ? kindLabel[row.kind] : undefined;
  return (
    <span className="cell-stack">
      <Badge>{kindKey ? t(kindKey) : (row.kind ?? row.status)}</Badge>
    </span>
  );
}

/**
 * A held thread's subject, as a citation of the message it was raised about.
 *
 * EmailReference rather than this file's own button: naming a message and
 * offering to open it is exactly what a citation is, and the hand-rolled
 * version here had grown its own copies of the blank-subject fallback, the
 * withheld reading and the not-openable branch — three rules that must agree
 * with every other citation in the product and had no way of doing so.
 *
 * No date, because the table has a When column of its own. A reference that
 * printed one would put the same timestamp twice in one row.
 *
 * Two flags, because there are three states and the caller must not collapse
 * them. `has_message` is whether the message still EXISTS, read without the
 * content gate; `activity_id` is read from the gated join, so the presence of
 * that field IS the permission. A message that exists and carries no id is one
 * this reader may not read — withheld — and one that carries an id but no
 * subject was simply sent with a blank subject line, which reads as "No
 * subject" and still opens.
 *
 * Only a thread whose message still exists reaches here; the caller draws the
 * erased case, which is a statement about the ledger rather than a citation.
 */
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
