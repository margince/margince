import { useQuery, type useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import { unwrap } from "../../lib/problem";
import { roomBriefKey } from "./dealroombrief.key";

export function participantsKey(roomId: string) {
  return ["deal-room-participants", roomId] as const;
}

// Every read of "who sits where" (the room's roster and the contact page's
// room list) goes stale together when a seat changes.
export function refreshSeats(
  queryClient: ReturnType<typeof useQueryClient>,
  roomId: string,
) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: participantsKey(roomId) }),
    queryClient.invalidateQueries({ queryKey: ["deal-rooms-of"] }),
    queryClient.invalidateQueries({ queryKey: roomBriefKey(roomId) }),
  ]);
}

export function useParticipants(roomId: string) {
  return useQuery({
    queryKey: participantsKey(roomId),
    queryFn: async () => {
      return unwrap(
        await api.GET("/deal-rooms/{id}/participants", {
          params: { path: { id: roomId } },
        }),
      );
    },
  });
}

/**
 * Attendance, counted once: how many seats stand, how many have been through
 * the door, and when a buyer last looked.
 *
 * Two surfaces state it: the deal's Deal Room card and the head of the room's
 * own page. Two counters over one list give two answers to one question: has
 * anybody walked in? A room reading "2 signed in" in the deal's margin and
 * "3 signed in" on its own page tells a rep something false.
 *
 * Revoked seats are not invited any more, which is why the filter is on both
 * counts rather than on the second alone.
 */
export function useRoomAttendance(roomId: string) {
  const participants = useParticipants(roomId);
  const rows = (participants.data?.data ?? []).filter((p) => !p.revoked_at);
  return {
    invited: rows.length,
    active: rows.filter((p) => p.has_signed_in).length,
    lastSeen: rows
      .map((p) => p.last_seen_at)
      .filter((v): v is string => typeof v === "string")
      .sort()
      .at(-1),
    // Whether the count is a fact yet. A "0 invited" drawn while the read is in
    // flight is a wrong statement rather than a loading one.
    counted: participants.isSuccess,
  };
}
