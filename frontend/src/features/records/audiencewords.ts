// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { valueFor } from "../../lib/labelfor";

type Activity = components["schemas"]["Activity"];
type ActivityAudience = components["schemas"]["ActivityAudience"];

export type AudienceWords = Readonly<{
  title: MessageKey;
  legend: MessageKey;
  scope: MessageKey;
  hints?: Readonly<Record<ActivityAudience, MessageKey>>;
}>;

const MESSAGE_AUDIENCE_WORDS: AudienceWords = {
  title: "compose.audienceTitle",
  legend: "compose.audienceLegend",
  scope: "compose.audienceNote",
};

// The choices' hints for a row that is not a message, which name no noun.
const ROW_AUDIENCE_HINTS: Readonly<Record<ActivityAudience, MessageKey>> = {
  workspace: "activity.audience.workspaceHint",
  participants: "activity.audience.participantsHint",
  selected: "activity.audience.selectedHint",
};

// The visibility dialog names the kind of row it changes, so a note is not
// called a message.
const AUDIENCE_WORDS: Readonly<Record<Activity["kind"], AudienceWords>> = {
  email: MESSAGE_AUDIENCE_WORDS,
  message: MESSAGE_AUDIENCE_WORDS,
  note: {
    title: "activity.audience.noteTitle",
    legend: "activity.audience.noteLegend",
    scope: "activity.audience.noteScope",
    hints: ROW_AUDIENCE_HINTS,
  },
  call: {
    title: "activity.audience.callTitle",
    legend: "activity.audience.callLegend",
    scope: "activity.audience.callScope",
    hints: ROW_AUDIENCE_HINTS,
  },
  meeting: {
    title: "activity.audience.meetingTitle",
    legend: "activity.audience.meetingLegend",
    scope: "activity.audience.meetingScope",
    hints: ROW_AUDIENCE_HINTS,
  },
  task: {
    title: "activity.audience.taskTitle",
    legend: "activity.audience.taskLegend",
    scope: "activity.audience.taskScope",
    hints: ROW_AUDIENCE_HINTS,
  },
};

/** The words the visibility dialog uses for one kind of timeline row. */
export function audienceWords(kind: Activity["kind"]): AudienceWords {
  return valueFor(AUDIENCE_WORDS, kind, MESSAGE_AUDIENCE_WORDS);
}
