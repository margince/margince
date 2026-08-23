import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";

// The unpublished change list of a room — what the next release would tell
// the buyer. Read by the Publish panel, the deal's room card and the
// document board alike, so it lives apart from any of them.

export function changesKey(roomId: string) {
  return ["deal-room-changes", roomId] as const;
}

export function useRoomChanges(roomId: string, enabled = true) {
  return useQuery({
    queryKey: changesKey(roomId),
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/deal-rooms/{id}/changes", {
        params: { path: { id: roomId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}
