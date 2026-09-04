// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What is going wrong on this lead's team.
//
// The board beside this answers "who is carrying what" and routes to a person.
// It cannot answer "what is going wrong": three counts per teammate cannot say
// that one customer has waited past the target while another rep's queue is
// merely long. This is the rows, worst first.
//
// EVERY ROW SHOWS ITS BASIS. The server decides each exception against a stated
// threshold — the lead-response policy's own state for a breach, never a number
// invented for the reading — and that basis is drawn beside the row. A lead
// disputing a line can see the rule rather than the verdict alone, which is the
// difference between a page they trust and one they stop opening.

import { DataTable, Disclosure } from "../design-system/atoms";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { type TeamException, useTeamExceptions } from "./worklist.queries";

/**
 * The exceptions, with the way into whoever answers for each.
 *
 * `enabled` gates the read at the tier the server gates it at: a rep asking
 * earns a 403, and a panel that rendered that as an error would tell them a
 * surface exists which is not theirs.
 */
export function TeamExceptionsPanel({
  enabled,
  onOwner,
}: Readonly<{ enabled: boolean; onOwner: (id: string) => void }>) {
  const t = useT();
  const exceptions = useTeamExceptions(enabled);
  if (!enabled) {
    return null;
  }
  const state = exceptions.isPending
    ? "loading"
    : exceptions.isError
      ? "failed"
      : "ready";
  return (
    <Disclosure summary={t("worklist.exceptions.title")}>
      <SurfaceState
        state={state}
        emptyLabel={t("worklist.exceptions.empty")}
        loadingLabel={t("worklist.exceptions.loading")}
        detail={{ onRetry: () => void exceptions.refetch() }}
      >
        {/* The rows, where the read gave any. A body without them is not a
            clear team — it is a response this panel cannot read — and mapping
            over the absence would take the whole page down with a type error
            rather than saying nothing here. */}
        {exceptions.data?.exceptions && (
          <>
            <DataTable
              label={t("worklist.exceptions.title")}
              rows={exceptions.data?.exceptions ?? []}
              rowKey={(row) => `${row.kind}-${row.subject.id}`}
              // Every row opens the queue of whoever answers for it, which is
              // the intervention this page routes to. A row nobody holds opens
              // the unassigned scope instead: the work is real and somebody has
              // to take it.
              //
              // Read off the OWNER'S KIND, not off the id. A `user` row whose
              // id this caller may not resolve still has somebody carrying it,
              // and sending the manager to the unassigned scope for it would
              // route them to a queue the work is not in.
              onRowClick={(row) =>
                row.owner.kind === "user" && row.owner.id
                  ? onOwner(row.owner.id)
                  : onOwner("")
              }
              columns={[
                {
                  key: "kind",
                  header: t("worklist.exceptions.condition"),
                  render: (row: TeamException) =>
                    t(`worklist.exceptions.kind.${row.kind}`),
                },
                {
                  key: "subject",
                  header: t("worklist.exceptions.subject"),
                  render: (row: TeamException) =>
                    row.subject.label ?? row.subject.id,
                },
                {
                  key: "owner",
                  header: t("worklist.exceptions.owner"),
                  // Three answers, because there are three facts and the wire
                  // tells them apart: a name, somebody this caller may not
                  // name, and nobody at all.
                  //
                  // Falling back to "Nobody yet" on a missing LABEL conflated
                  // the last two. An exception owned by a real person whose
                  // name the reader cannot resolve was reported as unassigned
                  // work — which is not a display nicety: a lead reads that as
                  // "this is going nowhere" and takes it, when a teammate is
                  // already carrying it.
                  //
                  // Never the raw id either: a uuid in front of a lead is the
                  // defect the label exists to prevent.
                  render: (row: TeamException) =>
                    row.owner.kind === "unassigned"
                      ? t("worklist.exceptions.nobody")
                      : (row.owner.label ??
                        t("worklist.exceptions.ownerWithheld")),
                },
                {
                  key: "threshold",
                  header: t("worklist.exceptions.basis"),
                  render: (row: TeamException) => row.threshold,
                },
              ]}
            />
            {/* A bounded page is not a clear team. The server says when it read
                to its own bound, and a lead who took this list for the whole
                of it would stop looking exactly where the rest begins. */}
            {exceptions.data?.truncated && (
              <p className="t-caption">{t("worklist.exceptions.truncated")}</p>
            )}
          </>
        )}
      </SurfaceState>
    </Disclosure>
  );
}
