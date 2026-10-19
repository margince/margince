// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { SectionHeader } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { QueryStates, WriteRefused } from "../../design-system/querystates";
import { SurfaceState } from "../../design-system/surfacestate";
import { viewerZone } from "../../format/timezone";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { isVersionSkewOf, problemMessageOf, unwrap } from "../../lib/problem";
import "./scheduledsends.css";
import { QueueSkewNotice } from "./scheduledsends.notices";
// The queue behind "send later", and the reason it has to exist: a rep who can
// schedule a message must be able to reach it again. Without this page the
// endpoints, the store and the fire path all existed and nothing in the client
// called them, so "send later" was a one-way door — the message a rep scheduled
// by mistake went out.
//
// It is the SENDER's own list, not the workspace's (ADR-0104/A155): an unsent
// body and its blind-copy list are not workspace-readable the way a sent
// activity is, and the server lists only what the caller scheduled. So there is
// no owner column here and no sharing affordance — the page is one contact's.
import { SendRow } from "./scheduledsends.row";

export type ScheduledSend = components["schemas"]["ScheduledSend"];

export type Status = ScheduledSend["status"];

/** The route this screen answers on, spelled where the screen lives. */
export const SCHEDULED_SCREEN = "scheduled" as const;

/**
 * The three groups a rep reads this page in, which are NOT the five wire
 * statuses.
 *
 * `held` is first because it is the only group that is waiting on a contact: a
 * gate refused at fire, or the moment passed while nothing was running, and the
 * message will not send itself. `waiting` is the queue proper. `closed` is
 * everything that is no longer going to change on its own — released, sent,
 * cancelled — kept on the page because a rep who cancelled a message wants to
 * see that it is cancelled rather than that it is gone.
 */
const GROUPS = ["held", "waiting", "closed"] as const;

type Group = (typeof GROUPS)[number];

function groupOf(status: Status): Group {
  if (status === "held") return "held";
  if (status === "scheduled") return "waiting";
  return "closed";
}

const GROUP_TITLE: Record<Group, MessageKey> = {
  held: "sched.group.held",
  waiting: "sched.group.waiting",
  closed: "sched.group.closed",
};

// Each group says what there is none OF in its own words, because "there is
// none" is the one state that has to be specific: a shared sentence under three
// headings tells the reader the same thing three times and none of it about the
// heading it sits under.
//
// A rep with nothing scheduled at all never reads any of them — the page prints
// ONE empty state instead of three, which is the difference between an inbox
// that is clear and a page that looks broken.
const GROUP_EMPTY: Record<Group, MessageKey> = {
  held: "sched.group.heldEmpty",
  waiting: "sched.group.waitingEmpty",
  closed: "sched.group.closedEmpty",
};

// The whole write, row identity included. The `mutationFn` takes it as a
// VARIABLE and never closes over the render's own state: the click belongs to
// the committed render, so what it passes cannot be older than the control the
// reader pressed, while what it closes over can be — and here the stale value
// would be the `If-Match` version, so the reader would be told somebody else
// moved the message when nobody did.
export type Move = Readonly<{ id: string; version: number; at: string }>;

type Withdraw = Readonly<{ id: string }>;

/**
 * The caller's own queue, read once.
 *
 * No status filter on the request: the three groups this page reads in ARE the
 * reading, and one unfiltered read fills all of them — asking per group would be
 * three requests to answer one question.
 *
 * Exported because the Tasks page carries this queue's front door and needs the
 * same answer to decide whether to offer it. One cache entry, one request, both
 * consumers — the shape `useRoster` uses for the same reason.
 */
export function useScheduledSends() {
  const t = useT();
  return useQuery({
    queryKey: ["scheduled-sends"],
    queryFn: async () => {
      return unwrap(await api.GET("/scheduled-sends", {}), t);
    },
    staleTime: 60_000,
  });
}

export function ScheduledSendsScreen() {
  const t = useT();
  const queryClient = useQueryClient();
  // A moment a rep chose is a personal one, so the page reads in the viewer's
  // own zone unless the send names another (see Moment).
  const readerZone = viewerZone();
  const [withdrawing, setWithdrawing] = useState<ScheduledSend | null>(null);

  const query = useScheduledSends();

  const move = useMutation({
    mutationFn: async ({ id, version, at }: Move) => {
      unwrap(
        await api.PATCH("/scheduled-sends/{id}", {
          // Through `ifMatch`, not a hand-written header: it is the one spelling of
          // the precondition. `src/api/if-match-coverage.test.ts` reads the AST for
          // that call, so a second spelling reads to the gate as no precondition.
          params: { path: { id }, ...ifMatch(version) },
          // The zone travels as a name beside the instant. So a message scheduled
          // across a DST boundary arrives at the wall time the rep meant, not an
          // hour either side.
          body: { scheduled_at: at, scheduled_tz: readerZone },
        }),
        t,
      );
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["scheduled-sends"] }),
  });

  const withdraw = useMutation({
    mutationFn: async ({ id }: Withdraw) => {
      unwrap(
        await api.POST("/scheduled-sends/{id}/cancel", {
          params: { path: { id } },
        }),
        t,
      );
    },
    onSuccess: () => {
      setWithdrawing(null);
      queryClient.invalidateQueries({ queryKey: ["scheduled-sends"] });
    },
  });

  // A 409 on either write says one thing: the row on screen is not the row on
  // the server. It already fired, somebody withdrew it, or another surface moved
  // it first — and all three want the same next step, which is to read the list
  // again rather than to retry a write against a version that is gone. It is
  // told apart from every other failure because "reload and look" is useless
  // advice for a 500 and the only useful advice here.
  const skew =
    isVersionSkewOf(move.error) || isVersionSkewOf(withdraw.error)
      ? t("sched.skew")
      : null;
  const writeError =
    skew === null
      ? ((move.isError ? problemMessageOf(move.error, t) : null) ??
        (withdraw.isError ? problemMessageOf(withdraw.error, t) : null))
      : null;

  return (
    // No heading of its own: the shell's PageTitle names this page from
    // OFF_RAIL_TITLE_KEYS and prints its subtitle from PAGE_SUB_KEYS, and a
    // second h1 here would leave a screen reader picking between two page
    // titles.
    <div className="wrap">
      {skew && (
        <QueueSkewNotice message={skew} onReload={() => void query.refetch()} />
      )}
      <WriteRefused titleKey="sched.writeFailed" message={writeError} />
      <QueryStates query={query} pendingLabel={t("nav.scheduled")}>
        {query.data && query.data.length === 0 ? (
          // One sentence for the whole page, not one per group: a rep who has
          // never scheduled anything is not reading three findings, and three
          // empty blocks make a page that is simply clear look broken.
          <SurfaceState
            state="empty"
            emptyLabel={t("sched.empty")}
            loadingLabel={t("nav.scheduled")}
          >
            {null}
          </SurfaceState>
        ) : (
          query.data && (
            <div className="arrive-stack">
              {GROUPS.map((group) => {
                const rows = query.data.filter(
                  (send) => groupOf(send.status) === group,
                );
                const title = t(GROUP_TITLE[group]);
                return (
                  <section key={group} aria-label={title}>
                    <SectionHeader title={title} />
                    <SurfaceState
                      loadingLabel={title}
                      label={title}
                      state={rows.length === 0 ? "empty" : "ready"}
                      emptyLabel={t(GROUP_EMPTY[group])}
                    >
                      {rows.map((send) => (
                        <SendRow
                          key={send.id}
                          send={send}
                          readerZone={readerZone}
                          movePending={move.isPending}
                          onMove={(input) => move.mutate(input)}
                          onWithdraw={setWithdrawing}
                        />
                      ))}
                    </SurfaceState>
                  </section>
                );
              })}
            </div>
          )
        )}
      </QueryStates>
      {/* Withdrawing is confirmed and moving is not, and the difference is what
          each one costs to undo: a moved message can be moved back, while a
          withdrawn one is gone and has to be written again from nothing — the
          approval it carried does not survive it. */}
      <ConfirmModal
        open={withdrawing !== null}
        onClose={() => setWithdrawing(null)}
        title={t("sched.withdrawTitle")}
        confirmLabel={t("sched.withdrawConfirm")}
        confirmVariant="danger"
        pending={withdraw.isPending}
        error={withdraw.isError ? problemMessageOf(withdraw.error, t) : null}
        onConfirm={() => {
          if (withdrawing) {
            withdraw.mutate({ id: withdrawing.id });
          }
        }}
      >
        <p>
          {t("sched.withdrawBody", { subject: withdrawing?.subject ?? "" })}
        </p>
      </ConfirmModal>
    </div>
  );
}
