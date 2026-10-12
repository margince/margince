// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

type Status = components["schemas"]["MeetingInvitation"]["status"];

/** The states in which the calendar has not answered yet. */
export const INVITATION_IN_FLIGHT: ReadonlySet<Status> = new Set([
  "pending",
  "rescheduling",
  "canceling",
]);

/** Where one meeting's invitation is cached, for the writes that replace it. */
export function meetingInvitationKey(id?: string, token?: string) {
  return ["meeting-invitation", id, token];
}

/**
 * One booked meeting's calendar invitation: the host reads it by id, a guest
 * by the token their email carries. Read again while the calendar is still
 * working, so a page left open sees the status settle and the video link
 * arrive.
 */
export function useMeetingInvitation(id?: string, token?: string) {
  return useQuery({
    queryKey: meetingInvitationKey(id, token),
    enabled: Boolean(id ?? token),
    queryFn: async () => {
      const result = token
        ? await api.GET("/public/meeting/{token}", {
            params: { path: { token } },
          })
        : await api.GET("/scheduling/invitations/{id}", {
            params: { path: { id: id ?? "" } },
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && INVITATION_IN_FLIGHT.has(status) ? 3000 : false;
    },
  });
}
