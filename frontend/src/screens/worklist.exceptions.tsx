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
//
// A TITLED PANEL, open, in the team board's own chrome — Panel, PanelBody,
// SurfaceState, table. It stood behind a disclosure, and a table of exceptions
// folded away reads as a block that failed to load rather than as one waiting
// to be asked for: this is a reading a lead came to the page FOR, and the
// first thing they would do with the control is open it.

import { DataTable } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useViewerId } from "./common";
import { AFTER_THE_DAY } from "./worklist.layout";
import { listReadState } from "./worklist.listread";
import { TakeOwnershipControl } from "./worklist.manager";
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
  // The whole body is a child rather than this function's own, so that a
  // reader below the tier MOUNTS NOTHING. Hooks cannot sit under an early
  // return, so keeping the reads here would have made a rep's page ask who
  // they are on behalf of a panel they are not shown — a request whose only
  // purpose was to fill a control that never rendered.
  return enabled ? <TeamExceptions onOwner={onOwner} /> : null;
}

function TeamExceptions({
  onOwner,
}: Readonly<{ onOwner: (id: string) => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const viewerId = useViewerId();
  const exceptions = useTeamExceptions(true);
  const rows = exceptions.data?.exceptions;
  // Off the LIST, not off the query's flags alone. A clear team is the healthy
  // answer and the contract says the list is empty then, so a state read from
  // `isPending`/`isError` called that `ready` and drew a table's header row
  // over no rows — five column names and a silence, where "nothing on the team
  // needs you right now" is the answer a lead came for.
  const state = listReadState(exceptions, rows);
  return (
    <Panel
      className={AFTER_THE_DAY}
      title={t("worklist.exceptions.title")}
      // HOW MANY need this lead, in the band that belongs to the whole panel.
      //
      // Only over a read that ANSWERED, and only over one that reached its own
      // end. `truncated` means the server stopped early, so the length is a
      // floor — and a floor printed as a count is a wrong number in the one
      // direction this surface must not get wrong: a lead told "4" over a
      // figure that is really 4-or-more will not go looking. A failed refetch
      // over a cached list is the same wrong number, since the rows survive it:
      // the band went on reporting the last good count beside a body saying the
      // team could not be read. The caveat in the body is what they get
      // instead.
      //
      // `ready` already means there is a non-empty list, so `rows` here narrows
      // the type rather than deciding anything.
      footer={
        state === "ready" && rows && !exceptions.data?.truncated
          ? t("worklist.exceptions.count", {
              count: formatNumber(rows.length, locale),
            })
          : undefined
      }
    >
      {/* The table carries its own cell padding but not the panel's inset, so
          it sits in a `PanelBody` with the truncation caveat rather than
          full-bleed against the panel's own edges — the arrangement the team
          board beside it already draws. */}
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("worklist.exceptions.empty")}
          loadingLabel={t("worklist.exceptions.loading")}
          detail={{ onRetry: () => void exceptions.refetch() }}
        >
          {/* `ready` already means there are rows — the state above is derived
              from the list — so this narrows the type rather than deciding
              anything. A response carrying no list at all resolved to
              `unavailable` and never reaches here: mapping over the absence
              would take the whole page down with a type error, and calling it a
              clear team would be the refusal reported as good news. */}
          {rows && (
            <>
              <DataTable
                label={t("worklist.exceptions.title")}
                rows={rows}
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
                  {
                    key: "take",
                    header: t("worklist.exceptions.intervene"),
                    // The press must not ALSO open the owner's queue. Every row
                    // navigates on click, so a button inside one fires both: the
                    // handover runs and the page walks away from its own
                    // confirmation, which reads as a control that did something
                    // unrelated to what it said.
                    //
                    // The control stops it on its own buttons rather than under a
                    // wrapper: a handler on a static element is invisible to a
                    // keyboard and the a11y lint rejects it, and the buttons are
                    // already the interactive things the event comes from.
                    render: (row: TeamException) => (
                      <TakeOwnershipControl
                        subject={row.subject}
                        viewerId={viewerId ?? ""}
                        insideAClickableRow
                      />
                    ),
                  },
                ]}
              />
              {/* A bounded page is not a clear team. The server says when it read
                to its own bound, and a lead who took this list for the whole
                of it would stop looking exactly where the rest begins. */}
              {exceptions.data?.truncated && (
                <p className="t-caption">
                  {t("worklist.exceptions.truncated")}
                </p>
              )}
            </>
          )}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}
