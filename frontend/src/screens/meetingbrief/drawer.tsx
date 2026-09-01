// The meeting brief drawer: the one component that fetches.
//
// It owns the read, the project scope the reader chose, and the routing of a
// citation to the record behind it. Everything it renders is MeetingBriefView,
// which holds no query and can therefore be put in front of a test or a story
// in any of its states.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { ENTITY, isEntityKind } from "../../app/entity";
import { useRecordZone } from "../../app/recordzone";
import { navigate } from "../../app/router";
import { Modal } from "../../design-system/atoms";
import {
  type PickableProject,
  ProjectPicker,
  ScopeLine,
} from "../../design-system/projectpicker";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf, throwProblem } from "../common";
import type { MeetingFacts, PreparedFor } from "./header";
import { type BriefViewState, MeetingBriefView } from "./view";

// A cited deal or contact goes to its own screen. The brief's other citation
// kind is the meeting activity itself, which has no screen and is rendered
// flat by the shared Citations — so this is only ever called for the two that
// route, and an unroutable kind is left where it is rather than guessed at.
function openCitedRecord(entityType: string, entityId: string) {
  if (isEntityKind(entityType)) {
    navigate(ENTITY[entityType].route(entityId));
  }
}

export function PersonMeetingBrief({
  activityId,
  open,
  onClose,
  projects = [],
  meeting,
  preparedFor,
}: Readonly<{
  activityId: string | null;
  open: boolean;
  onClose: () => void;
  // The person's live projects, for a meeting filed under none: the brief
  // scopes itself by the meeting's own filing, and only an unattributed
  // meeting needs to be told which body of work to prepare for.
  projects?: readonly PickableProject[];
  // What the opening page already knows about the room. The wire carries none
  // of it, and a caller that holds none passes none.
  meeting?: MeetingFacts;
  preparedFor?: PreparedFor;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The meeting's time is formatted here rather than by the page that opened
  // the drawer: the locale and the record zone are this tier's to know, and a
  // caller passing a pre-formatted string would be a second formatter.
  const zone = useRecordZone();
  // The project the reader chose to prepare for. It belongs to ONE meeting:
  // the same drawer is reused for the next meeting on the page, and a scope
  // chosen for a room about the ERP rollout must not narrow the brief for a
  // different room. No sole-project default here, unlike the composers — the
  // first read has to be unscoped to learn whether the meeting is filed, and
  // a default sent before that answer would refuse on a meeting filed
  // elsewhere.
  const [projectId, setProjectId] = useState("");
  const [chosenFor, setChosenFor] = useState(activityId);
  if (chosenFor !== activityId) {
    setChosenFor(activityId);
    setProjectId("");
  }
  const brief = useQuery({
    enabled: open && activityId != null,
    queryKey: ["meetingBrief", activityId, projectId],
    queryFn: async () => {
      const { data, error } = await api.GET("/activities/{id}/meeting-brief", {
        params: {
          path: { id: activityId ?? "" },
          query: projectId ? { project_id: projectId } : undefined,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  // A scope the server reports while the reader chose none is the meeting's
  // own filing: the brief is about that project whatever the reader picks,
  // so the picker stands down and the line alone says so.
  const filedByMeeting = brief.data?.scope != null && projectId === "";

  let state: BriefViewState;
  if (brief.isSuccess && brief.data) {
    state = { kind: "ready", brief: brief.data };
  } else if (brief.isError) {
    state = {
      kind: "failed",
      message: problemMessageOf(brief.error, t),
      onRetry: () => {
        void brief.refetch();
      },
    };
  } else {
    state = { kind: "loading" };
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy="person-meeting-title"
      size="wide"
      placement="right"
    >
      <MeetingBriefView
        state={state}
        meeting={meeting}
        preparedFor={preparedFor}
        onOpenRecord={openCitedRecord}
        titleId="person-meeting-title"
        onClose={onClose}
        formatWhen={(utcIso) => formatDateTime(utcIso, locale, zone)}
        scopeSlot={
          <>
            {filedByMeeting && brief.data?.scope && (
              <ScopeLine scope={brief.data.scope} />
            )}
            {!filedByMeeting && (brief.isSuccess || projectId !== "") && (
              <ProjectPicker
                projects={projects}
                projectId={projectId}
                onChange={setProjectId}
                scope={brief.data?.scope}
              />
            )}
          </>
        }
      />
    </Modal>
  );
}
