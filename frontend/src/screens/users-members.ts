// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useIsMutating, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

export type User = components["schemas"]["User"];
type MemberAction = NonNullable<User["allowed_actions"]>[number];

const MEMBERS_KEY = ["users-admin"] as const;

// include_inactive is honored server-side only for a caller holding
// `user_admin:read`, so a deactivated member stays reachable for reactivation.
export function useMembers() {
  return useQuery({
    queryKey: MEMBERS_KEY,
    queryFn: async (): Promise<User[]> => {
      const { data, error } = await api.GET("/users", {
        params: { query: { include_inactive: true } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

// The roster's allowed_actions run the checks the verb itself runs, so a member
// offers what the write accepts. An absent list offers nothing.
export function offers(member: User, action: MemberAction): boolean {
  return member.allowed_actions?.includes(action) ?? false;
}

// One key per member, so the role cell and the verbs cell each see the other's
// write in flight.
export function memberMutationKey(memberId: string, verb: string) {
  return ["member", memberId, verb];
}

// The member's own cell, where focus lands once the verb that moved it is gone.
export function memberAnchorId(memberId: string): string {
  return `member-${memberId}`;
}

export function useMemberBusy(memberId: string): boolean {
  return useIsMutating({ mutationKey: ["member", memberId] }) > 0;
}

// Returned to each onSuccess so the mutation stays pending until the new roster
// lands; settling first would redraw the replaced role from the stale cache.
export function useMemberRefresh() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: MEMBERS_KEY }),
      // Settings › Seats reads the seat count from its own endpoint.
      qc.invalidateQueries({ queryKey: ["installation-seat-usage"] }),
      qc.invalidateQueries({ queryKey: ["installation-license"] }),
    ]);
}
