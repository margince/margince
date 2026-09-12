// The brief's body, as a pure component: state in, prose out, no fetching.
//
// Split from the drawer so the four states a reader can land in — assembling,
// failed, nothing recorded, prepared — can each be rendered from a fixture in
// a test and a story. The connected drawer holds the query and hands the
// answer here.

import { X } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { SurfaceState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { CoachPanel, MeetingPaths } from "./coaching";
import { BriefHeader, type MeetingFacts, type PreparedFor } from "./header";
import {
  AccountArc,
  AdvancePanel,
  LikelyAsks,
  ObjectivePanel,
  Scenarios,
  TopRisk,
  Unknowns,
} from "./plan";
import {
  Background,
  BodyPanels,
  GlanceLine,
  GoalPanel,
  RisksPanel,
} from "./sections";
import "./meetingbrief.css";

type MeetingBrief = components["schemas"]["MeetingBrief"];

// The four answers a read can produce. A union rather than three booleans,
// because "loading and failed" and "ready with no data" are states the caller
// should not be able to spell by accident.
export type BriefViewState =
  | { kind: "loading" }
  | { kind: "failed"; message: string; onRetry: () => void }
  | { kind: "ready"; brief: MeetingBrief };

function stateOf(state: BriefViewState) {
  switch (state.kind) {
    case "loading":
      return "loading" as const;
    case "failed":
      return "failed" as const;
    default:
      // A brief with no sections but a withheld source is NOT empty. "Nothing
      // is recorded for this meeting" and "you are not being shown what is"
      // are different facts, and answering the second with the first is the
      // silence the omissions exist to break: a reader would walk in believing
      // the record was blank.
      return state.brief.sections.length === 0 &&
        (state.brief.omitted ?? []).length === 0
        ? ("empty" as const)
        : ("ready" as const);
  }
}

export function MeetingBriefView({
  state,
  meeting,
  preparedFor,
  onOpenRecord,
  onOpenEmail,
  titleId,
  onClose,
  scopeSlot,
  formatWhen,
  formatDay,
}: Readonly<{
  state: BriefViewState;
  meeting?: MeetingFacts;
  preparedFor?: PreparedFor;
  onOpenRecord: (entityType: string, entityId: string) => void;
  // Opens a cited message in the host's own email drawer; see `Citations`.
  onOpenEmail?: (activityId: string) => void;
  titleId: string;
  onClose: () => void;
  // The project picker or the scope line, built by the drawer because only it
  // knows what the reader chose.
  scopeSlot?: ReactNode;
  formatWhen?: (utcIso: string) => string;
  formatDay?: (utcIso: string) => string;
}>) {
  const t = useT();
  const brief = state.kind === "ready" ? state.brief : undefined;
  return (
    <>
      <div className="drawer-head">
        <div className="pe-drawer-title">
          <h2 id={titleId}>{t("contact.meeting.title")}</h2>
          <Button
            small
            iconOnly
            onClick={onClose}
            aria-label={t("contact.drawer.close")}
          >
            <X aria-hidden="true" />
          </Button>
        </div>
        <BriefHeader
          brief={brief}
          meeting={meeting}
          preparedFor={preparedFor}
          formatWhen={formatWhen}
        />
      </div>
      <div className="drawer-body">
        <div className="mb-body">
          {scopeSlot}
          <SurfaceState
            state={stateOf(state)}
            emptyLabel={t("contact.meeting.empty")}
            loadingLabel={t("contact.meeting.loading")}
            loadingLines={8}
            detail={
              state.kind === "failed" ? { onRetry: state.onRetry } : undefined
            }
          >
            {brief && (
              <div className="mb-stack">
                <GlanceLine
                  brief={brief}
                  onOpenRecord={onOpenRecord}
                  onOpenEmail={onOpenEmail}
                />
                {/* The plan leads when the server says it is a preparation.
                    An `outline` is added ABOVE the sections rather than in
                    place of them: a half-built plan that hid the risks and
                    talking points a reader already had would be a regression
                    wearing new panels. */}
                {brief.plan?.manager_coaching && (
                  <>
                    <CoachPanel
                      coaching={brief.plan.manager_coaching}
                      writtenByModel={brief.plan.generated_by === "model"}
                    />
                    <MeetingPaths
                      coaching={brief.plan.manager_coaching}
                      writtenByModel={brief.plan.generated_by === "model"}
                    />
                  </>
                )}
                {brief.plan && (
                  <>
                    <ObjectivePanel
                      plan={brief.plan}
                      onOpenRecord={onOpenRecord}
                      onOpenEmail={onOpenEmail}
                    />
                    <TopRisk
                      plan={brief.plan}
                      onOpenRecord={onOpenRecord}
                      onOpenEmail={onOpenEmail}
                    />
                    <LikelyAsks
                      plan={brief.plan}
                      onOpenRecord={onOpenRecord}
                      onOpenEmail={onOpenEmail}
                    />
                    <Scenarios plan={brief.plan} />
                    <AccountArc
                      plan={brief.plan}
                      onOpenRecord={onOpenRecord}
                      onOpenEmail={onOpenEmail}
                      formatDay={formatDay ?? ((iso) => iso.slice(0, 10))}
                    />
                  </>
                )}
                <GoalPanel
                  brief={brief}
                  onOpenRecord={onOpenRecord}
                  onOpenEmail={onOpenEmail}
                />
                {/* The sections' risk list, unless the plan carried the one
                    risk that matters with what to do about it — two warn
                    panels on one surface is no warning at all, and the plan's
                    is the one a reader can act on. */}
                {!brief.plan?.top_risk && (
                  <RisksPanel
                    brief={brief}
                    onOpenRecord={onOpenRecord}
                    onOpenEmail={onOpenEmail}
                  />
                )}
                <BodyPanels
                  brief={brief}
                  onOpenRecord={onOpenRecord}
                  onOpenEmail={onOpenEmail}
                />
                {brief.plan && (
                  <>
                    <AdvancePanel
                      plan={brief.plan}
                      onOpenRecord={onOpenRecord}
                      onOpenEmail={onOpenEmail}
                    />
                    <Unknowns plan={brief.plan} />
                  </>
                )}
                <Background
                  brief={brief}
                  onOpenRecord={onOpenRecord}
                  onOpenEmail={onOpenEmail}
                />
              </div>
            )}
          </SurfaceState>
          {/* The server's own sentence, under SurfaceState's generic one. A
              read can fail for a reason the reader can act on — a project they
              may not open, a meeting filed elsewhere — and "could not load"
              throws that away. Outside the SurfaceState because it renders
              children only when the state is ready. */}
          {state.kind === "failed" && (
            <p className="mb-failed-detail">{state.message}</p>
          )}
        </div>
      </div>
      <div className="drawer-foot">
        <span className="pe-disclosure t-caption">
          {t("contact.meeting.assembledNow")}
        </span>
        <Button onClick={onClose}>{t("contact.drawer.close")}</Button>
      </div>
    </>
  );
}
