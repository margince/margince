// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { SegmentedControl } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { isTimelineRead } from "../../lib/activitykeys";
import { unwrap } from "../../lib/problem";
import "./invitationanswer.css";

type Invitation = components["schemas"]["EmailCalendarInvitation"];
type Answer =
  components["schemas"]["AnswerCalendarInvitationRequest"]["response"];

const ANSWERS: readonly Answer[] = ["accepted", "tentative", "declined"];

/**
 * The answers to a calendar invitation, drawn where a reply would be. An
 * invitation asks for an RSVP, so the server's `calendar_invitation` replaces
 * the reply verb on every surface that offers one (ChannelReplyAction).
 *
 * The press answers on the reader's own calendar. One choice of three, so it
 * is a segmented control: the answer held is the pressed one, and can change.
 */
export function InvitationAnswer({
  activityId,
  invitation,
  onAnswered,
}: Readonly<{
  activityId: string;
  invitation: Invitation;
  /** For a surface the answer takes the row off, such as the worklist. */
  onAnswered?: () => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [held, setHeld] = useState(invitation.response);
  const answer = useMutation({
    mutationFn: async (response: Answer) =>
      unwrap(
        await api.PUT("/activities/{id}/calendar-response", {
          params: { path: { id: activityId } },
          body: { response },
        }),
      ),
    onSuccess: (answered) => {
      setHeld(answered.response);
      void queryClient.invalidateQueries({
        predicate: (query) => isTimelineRead(query.queryKey),
      });
      onAnswered?.();
    },
  });
  const labels = {
    needs_action: "",
    accepted: t("invitation.answer.accepted"),
    tentative: t("invitation.answer.tentative"),
    declined: t("invitation.answer.declined"),
  };
  return (
    <span className="invitation-answer">
      <SegmentedControl<Invitation["response"]>
        label={t("invitation.answer.label")}
        options={ANSWERS}
        value={held}
        labels={labels}
        busy={answer.isPending ? answer.variables : undefined}
        onChange={(response) => {
          if (response !== "needs_action") {
            answer.mutate(response);
          }
        }}
      />
      {answer.isError && (
        <span className="invitation-answer__error">
          <ErrorLine error={answer.error} inline />
        </span>
      )}
    </span>
  );
}
