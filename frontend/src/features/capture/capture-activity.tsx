// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import "./capture-activity.css";
import { type ReactNode, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  EmptyState,
  SegmentedControl,
  StatCard,
} from "../../design-system/atoms";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../../design-system/panel";
import { LoadMoreButton, QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { useOpenEmail } from "../../lib/openemail";
import { throwProblem } from "../../lib/problem";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { CaptureActivityDrawer } from "./capture-activity-drawer";
import { CaptureMessages } from "./capture-activity-messages";
import { CaptureExclusionsCard } from "./capture-exclusions";
import { ConnectionOutcomesCard } from "./connection-outcomes";
import { VerdictPassNote } from "./verdictpass";

// Settings → Capture activity: what the pipeline did with the reader's own
// messages in the last 24 hours.
//
// Until this existed, every decision the pipeline made was written and none was
// readable: the activity that WAS created has an audit row, and the decisions
// that created nothing were operator breadcrumbs with no member on them. A rep
// whose message never appeared had no way to ask why.
//
// The scope is not a filter the reader picks. Personal rows come from their own
// connections and no grant widens them; the workspace section is shared-channel
// traffic (a bot binding) and takes the capture_trace object. The two are
// different endpoints for that reason, not one endpoint with a parameter.

type CaptureActivity = components["schemas"]["CaptureActivityResponse"];
type TraceEntry = components["schemas"]["CaptureTraceEntry"];

// The five outcomes, in the order a message meets them, so the strip reads as a
// path rather than as a legend sorted by size.
const OUTCOMES = [
  "captured",
  "internal",
  "suppressed",
  "deferred",
  "fault",
] as const;

type Outcome = (typeof OUTCOMES)[number];

const SCOPES = ["mine", "workspace"] as const;
type Scope = (typeof SCOPES)[number];

export function CaptureActivityTab() {
  const t = useT();
  // The workspace section is the gated half. Hidden rather than disabled when
  // the grant is absent: a toggle a reader cannot use is an invitation to ask
  // why, and shared-channel debugging is not part of every seat's job. Their
  // OWN activity — the reason they opened this page — is unaffected.
  const canReadWorkspace = useCan("capture_trace", "read");
  const [scope, setScope] = useState<Scope>("mine");

  return (
    <>
      {/* What waits on the reader comes first: threads held back from the
          team, which an outage makes urgent because every new one lands held.
          The senders the classifier decided on sit in the same card. */}
      <ConnectionOutcomesCard />
      {/* What the reader keeps out. Blocking a sender is a rep's answer to
          what this page asks, not an administrator's posture setting. */}
      <CaptureExclusionsCard />
      <Panel title={t("captureActivity.windowTitle")}>
        <PanelBody>
          <PanelIntro>{t("captureActivity.sub")}</PanelIntro>
        </PanelBody>
        <CaptureActivityWindow
          scope={canReadWorkspace ? scope : "mine"}
          scopeRow={
            canReadWorkspace && (
              <SettingRow
                label={t("captureActivity.scope.label")}
                control={
                  <SegmentedControl<Scope>
                    label={t("captureActivity.scope.label")}
                    value={scope}
                    onChange={setScope}
                    options={SCOPES}
                    labels={{
                      mine: t("captureActivity.scope.mine"),
                      workspace: t("captureActivity.scope.workspace"),
                    }}
                  />
                }
              />
            )
          }
        />
      </Panel>
    </>
  );
}

// One fetch per route, so each response is typed by the route that produced it.
async function fetchWindow(
  path: "/capture/activity",
  cursor: string | undefined,
): Promise<CaptureActivity>;
async function fetchWindow(
  path: "/capture/activity/workspace",
  cursor: string | undefined,
): Promise<CaptureActivity>;
async function fetchWindow(
  path: "/capture/activity" | "/capture/activity/workspace",
  cursor: string | undefined,
): Promise<CaptureActivity> {
  // Each branch names its route as a LITERAL. `api.GET` is typed per path, so
  // the union the overloads accept is not something it can be handed — the
  // narrowing is the reason the branch exists, and writing the literal is what
  // makes the two arms visibly two different calls rather than one repeated.
  const { data, error } =
    path === "/capture/activity/workspace"
      ? await api.GET("/capture/activity/workspace", {
          params: { query: { cursor } },
        })
      : await api.GET("/capture/activity", { params: { query: { cursor } } });
  if (error) throwProblem(error);
  return data;
}

function CaptureActivityWindow({
  scope,
  scopeRow,
}: Readonly<{ scope: Scope; scopeRow: ReactNode }>) {
  const t = useT();
  const [filter, setFilter] = useState<Outcome | null>(null);
  // The entry rather than its id, because the drawer names the message and the
  // trace read it makes returns rungs, not correspondence.
  const [openTrace, setOpenTrace] = useState<TraceEntry | null>(null);
  const [openEmail, setOpenEmail] = useOpenEmail();
  // Paged, because a busy mailbox fills more than one page of the 24 hours.
  const query = useInfiniteQuery({
    queryKey: ["capture-activity", scope],
    initialPageParam: undefined as string | undefined,
    // One literal route per branch: openapi-fetch cannot type a union path
    // without a cast.
    queryFn: ({ pageParam }) =>
      scope === "workspace"
        ? fetchWindow("/capture/activity/workspace", pageParam)
        : fetchWindow("/capture/activity", pageParam),
    getNextPageParam: (last) => last.page.next_cursor ?? undefined,
  });
  const loaded = query.data;
  return (
    <>
      <SettingList bleed="settings">
        {scopeRow}
        <QueryGate query={query} pendingLabel={t("captureActivity.title")}>
          {({ pages: [firstPage] }) =>
            // The funnel counts the window, so it comes off the first page.
            firstPage && (
              <SettingRow
                label={t("captureActivity.outcomes")}
                description={t("captureActivity.scopeNote")}
                layout="stack"
                control={
                  <>
                    <CaptureFunnel
                      funnel={firstPage.funnel}
                      selected={filter}
                      onSelect={setFilter}
                    />
                    {/* Only while something waits; a cadence over nothing
                      outstanding is furniture a reader learns to skip. */}
                    {(firstPage.funnel.deferred ?? 0) > 0 && (
                      <VerdictPassNote
                        clock={firstPage.sender_verdict}
                        subject="senders"
                      />
                    )}
                  </>
                }
              />
            )
          }
        </QueryGate>
      </SettingList>
      {loaded && (
        <CaptureLog
          pages={loaded.pages}
          filter={filter}
          more={query.hasNextPage && <LoadMoreButton query={query} />}
          onClear={() => setFilter(null)}
          onOpen={setOpenTrace}
        />
      )}
      {openTrace && (
        <CaptureActivityDrawer
          traceId={openTrace.id}
          message={openTrace}
          onOpenEmail={(activityId) => {
            // Two drawers at once are two focus traps, and the reader is moving
            // from the trace to the message rather than reading both.
            setOpenTrace(null);
            setOpenEmail(activityId);
          }}
          onClose={() => setOpenTrace(null)}
        />
      )}
      <OpenEmailDrawer
        activityId={openEmail}
        zone={viewerZone()}
        onClose={() => setOpenEmail(null)}
      />
    </>
  );
}

// The rows already loaded, filtered on the bucket the counters group by so the
// list and the tile above it give one answer.
function CaptureLog({
  pages,
  filter,
  more,
  onClear,
  onOpen,
}: Readonly<{
  pages: CaptureActivity[];
  filter: Outcome | null;
  /** Outside the rows, so a filter matching nothing loaded can still page on. */
  more: ReactNode;
  onClear: () => void;
  onOpen: (entry: TraceEntry) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const first = pages[0];
  if (first === undefined) return null;
  const entries = pages.flatMap((page) => page.data);
  const shown = filter
    ? entries.filter((entry) => entry.outcome_now === filter)
    : entries;
  return (
    <>
      <PanelGroupHead title={t("captureActivity.messages")} level="h3" />
      {(!first.payload_capture_enabled || filter || shown.length === 0) && (
        <PanelBody className="form-stack">
          {!first.payload_capture_enabled && (
            // Said once, about the installation: per row it would read as a
            // fact about that one message.
            <p className="capture-activity__note t-caption">
              {t("captureActivity.payloadsOff")}
            </p>
          )}
          {filter && (
            // Both numbers: the funnel counts the window and this filters what
            // is loaded, so a bare "12" under a tile reading 26 looks wrong.
            <div className="capture-activity__countline">
              <p className="t-caption" role="status">
                {t("captureActivity.filtered", {
                  shown: formatNumber(shown.length, locale),
                  total: formatNumber(first.funnel[filter] ?? 0, locale),
                  outcome: t(`captureActivity.outcome.${filter}`),
                })}
              </p>
              {shown.length > 0 && (
                <Button onClick={onClear}>{t("table.clearFilters")}</Button>
              )}
            </div>
          )}
          {shown.length === 0 &&
            (filter ? (
              // A filter that matched nothing loaded is not an empty window.
              <EmptyState
                title={t("captureActivity.emptyFiltered")}
                action={
                  <Button onClick={onClear}>{t("table.clearFilters")}</Button>
                }
              >
                {null}
              </EmptyState>
            ) : (
              <EmptyState>{t("captureActivity.empty")}</EmptyState>
            ))}
        </PanelBody>
      )}
      {shown.length > 0 && (
        <CaptureMessages
          entries={shown}
          payloads={first.payload_capture_enabled}
          onOpen={onOpen}
        />
      )}
      {more && <PanelBody>{more}</PanelBody>}
    </>
  );
}

// The funnel doubles as the filter, because the counters ARE the question a
// reader arrives with: "26 waiting on a verdict — which 26?".
//
// It filters the rows already LOADED, not the window, so the count line beside
// it says both numbers. A filter that silently showed 12 of 26 would be a worse
// answer than no filter at all.
//
// Each tile is labelled with its bucket's own name, which is honest again now
// that the counters group by the SETTLED outcome: every message under
// `deferred` really is still waiting. While they grouped by what the pipeline
// recorded, one number covered the senders being judged and the ones already
// answered, and this tile read "Waiting on a verdict — 31" over rows that each
// said the verdict had landed.
function CaptureFunnel({
  funnel,
  selected,
  onSelect,
}: Readonly<{
  funnel: CaptureActivity["funnel"];
  selected: Outcome | null;
  onSelect: (outcome: Outcome | null) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <StatStrip
      className="capture-activity__funnel"
      testId="capture-activity-funnel"
      density="compact"
    >
      {OUTCOMES.map((outcome) => (
        <button
          key={outcome}
          type="button"
          aria-pressed={selected === outcome}
          onClick={() => onSelect(selected === outcome ? null : outcome)}
        >
          <StatCard
            // Rows on a phone: five slots two abreast clip both the label and
            // the figure, and the press target stays the whole row because the
            // plate keeps the button's chrome off it (statstrip.css).
            narrow="row"
            // The tile's own short name. The full outcome sentences still feed
            // the filter line and the row chip, each read one at a time; a slot
            // compared across a row of five carries a NAME, and "Waiting on a
            // sender verdict" over a figure is a sentence where a label goes.
            label={t(`captureActivity.funnel.${outcome}`)}
            // Zero is a reading, not an absence: "no message was dropped as
            // internal today" is exactly what somebody comes here to confirm.
            value={formatNumber(funnel[outcome] ?? 0, locale)}
          />
        </button>
      ))}
    </StatStrip>
  );
}
