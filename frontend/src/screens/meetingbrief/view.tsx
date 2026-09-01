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
import { BriefHeader, type MeetingFacts, type PreparedFor } from "./header";
import {
  Background,
  BodyPanels,
  GlanceLine,
  GoalPanel,
  RiskCallout,
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
  titleId,
  onClose,
  scopeSlot,
  formatWhen,
}: Readonly<{
  state: BriefViewState;
  meeting?: MeetingFacts;
  preparedFor?: PreparedFor;
  onOpenRecord: (entityType: string, entityId: string) => void;
  titleId: string;
  onClose: () => void;
  // The project picker or the scope line, built by the drawer because only it
  // knows what the reader chose.
  scopeSlot?: ReactNode;
  formatWhen?: (utcIso: string) => string;
}>) {
  const t = useT();
  const brief = state.kind === "ready" ? state.brief : undefined;
  return (
    <>
      <div className="drawer-head">
        <div className="pe-drawer-title">
          <h2 id={titleId}>{t("person.meeting.title")}</h2>
          <Button
            small
            iconOnly
            onClick={onClose}
            aria-label={t("person.drawer.close")}
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
            emptyLabel={t("person.meeting.empty")}
            loadingLabel={t("person.meeting.loading")}
            loadingLines={8}
            detail={
              state.kind === "failed" ? { onRetry: state.onRetry } : undefined
            }
          >
            {brief && (
              <div className="mb-stack">
                <GlanceLine brief={brief} onOpenRecord={onOpenRecord} />
                <GoalPanel brief={brief} onOpenRecord={onOpenRecord} />
                <RiskCallout brief={brief} onOpenRecord={onOpenRecord} />
                <BodyPanels brief={brief} onOpenRecord={onOpenRecord} />
                <Background brief={brief} onOpenRecord={onOpenRecord} />
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
        <span className="pe-disclosure">
          {t("person.meeting.assembledNow")}
        </span>
        <Button onClick={onClose}>{t("person.drawer.close")}</Button>
      </div>
    </>
  );
}
