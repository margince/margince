import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { stable } from "../../format/collate";
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

type Seat = Pick<
  components["schemas"]["DealRoomParticipant"],
  "delivery_state" | "revoked_at"
>;

// How often the roster is read again while an invitation mail is on its way.
// The relay's answer is stamped on the row just after the invite answers, and
// must reach it.
export const MAIL_POLL_MS = 2_000;

// How long after a queued invitation the roster keeps reading: the server's
// 10-second relay wait, plus a margin. `pending` also means "no relay
// configured", so the watch ends rather than polling forever.
export const MAIL_WATCH_MS = 30_000;

function mailWatchKey(roomId: string) {
  return ["deal-room-mail-watch", roomId] as const;
}

// Called when an invitation was handed to the sender: the roster reads again
// until the relay's answer lands on it.
export function watchInviteMail(
  queryClient: ReturnType<typeof useQueryClient>,
  roomId: string,
  now = Date.now(),
) {
  queryClient.setQueryData(mailWatchKey(roomId), now + MAIL_WATCH_MS);
}

export function seatPollInterval(
  seats: readonly Seat[] | undefined,
  watchUntil: number | undefined,
  now: number,
): number | false {
  if (watchUntil === undefined || now > watchUntil) {
    return false;
  }
  const onItsWay = seats?.some(
    (seat) => !seat.revoked_at && seat.delivery_state === "pending",
  );
  return onItsWay ? MAIL_POLL_MS : false;
}

export function useParticipants(roomId: string) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: participantsKey(roomId),
    refetchInterval: (query) =>
      seatPollInterval(
        query.state.data?.data,
        queryClient.getQueryData<number>(mailWatchKey(roomId)),
        Date.now(),
      ),
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
      .sort(stable)
      .at(-1),
    // Whether the count is a fact yet. A "0 invited" drawn while the read is in
    // flight is a wrong statement rather than a loading one.
    counted: participants.isSuccess,
  };
}
