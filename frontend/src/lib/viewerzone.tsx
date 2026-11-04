// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import { unwrap } from "./problem";

type Me = components["schemas"]["MeResponse"];
type TimezoneSource = components["schemas"]["SaveMyTimezoneRequest"]["source"];

/**
 * Writes the signed-in user's own time zone. `browser` is the app's guess,
 * stored only while no zone is stored. `chosen` is the user's pick.
 *
 * On success `/me` is read again rather than patched from this answer. Two
 * writes can answer out of order, and the server holds the zone that won. The
 * booking reads that use the zone are asked again too.
 */
export function useSaveMyTimezone() {
  const t = useT();
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({
      timezone,
      source,
    }: Readonly<{ timezone: string; source: TimezoneSource }>) =>
      unwrap(await api.PUT("/me/timezone", { body: { timezone, source } }), t),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["me"] }),
        client.invalidateQueries({ queryKey: ["working-hours"] }),
        client.invalidateQueries({ queryKey: ["reliable-availability"] }),
      ]);
    },
  });
}

/**
 * On a sign-in with no stored zone, stores the browser's zone as a guess, once
 * per user. The guard holds the user it was sent for, so a different user
 * signing in on the same tab is asked for too. A failed write clears it, so the
 * next read of `/me` tries again. The server keeps any zone already stored, so
 * a second browser never replaces the first one's guess or a chosen zone.
 */
export function useBrowserZoneOnFirstSignIn(me: Me | undefined) {
  const save = useSaveMyTimezone();
  const sentFor = useRef<string | null>(null);
  const { mutate } = save;
  // Run on every read of `me`, so a fresh read retries a failed guess.
  useEffect(() => {
    const userID = me?.user.id;
    if (!me || me.user.timezone || !userID || sentFor.current === userID)
      return;
    sentFor.current = userID;
    mutate(
      { timezone: viewerZone(), source: "browser" },
      {
        onError: () => {
          if (sentFor.current === userID) sentFor.current = null;
        },
      },
    );
  }, [me, mutate]);
}
