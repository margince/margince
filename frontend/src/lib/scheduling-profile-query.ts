import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "./capability";
import { ProblemError, unwrap } from "./problem";
import { useMe } from "./session";

type Profile = components["schemas"]["SchedulingProfile"];

/**
 * The signed-in seat's booking profile. The server answers only a seat holding
 * activity:create, so a refused seat gets that refusal here without a request.
 */
export function useSchedulingProfile({
  refreshOnReturn = false,
  enabled = true,
}: Readonly<{ refreshOnReturn?: boolean; enabled?: boolean }> = {}) {
  const answered = useMe().data?.authorization !== undefined;
  const refused = useBookingRefused();
  return useQuery({
    queryKey: ["scheduling-profile"],
    enabled: enabled && answered,
    refetchOnWindowFocus: refreshOnReturn ? "always" : false,
    queryFn: async () => {
      if (refused) throw new ProblemError(BOOKING_REFUSED);
      return unwrap(await api.GET("/scheduling/profile"));
    },
  });
}

// The server's own answer to this seat, so the page reads as it did when asked.
const BOOKING_REFUSED = { status: 403, code: "permission_denied" };

/** Whether this seat may book meetings, which is what owning a booking profile takes. */
export function useCanBook(): boolean {
  return useCan("activity", "create");
}

/**
 * Whether /me has answered that this seat may not book. False until it
 * answers, so no refusal is claimed before the server has decided.
 */
export function useBookingRefused(): boolean {
  const canBook = useCanBook();
  const answered = useMe().data?.authorization !== undefined;
  return answered && !canBook;
}

/**
 * Whether guests can book on the host's page now: the server's own rule, a
 * page switched on whose calendar can take bookings. A guest of a paused page
 * is answered as if the page did not exist.
 */
export function bookingPagePaused(profile: Profile): boolean {
  return !profile.enabled || (profile.blocker ?? "none") !== "none";
}

/**
 * Writes some fields of the booking profile over the server's latest copy.
 * A card saving one setting then never puts back what another card or tab
 * changed.
 */
export async function putSchedulingChanges(
  changes: Partial<Profile>,
): Promise<Profile> {
  const latest = unwrap(await api.GET("/scheduling/profile"));
  return unwrap(
    await api.PUT("/scheduling/profile", {
      body: { ...latest, ...changes },
    }),
  );
}

export function usePatchSchedulingProfile() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: putSchedulingChanges,
    onSuccess: async (profile) => {
      client.setQueryData(["scheduling-profile"], profile);
      await client.invalidateQueries({ queryKey: ["reliable-availability"] });
    },
  });
}
