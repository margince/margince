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
type User = components["schemas"]["User"];
type TimezoneSource = components["schemas"]["SaveMyTimezoneRequest"]["source"];

/**
 * Writes the signed-in person's own time zone. `browser` is the app's guess and
 * the server stores it only while no zone is stored; `chosen` is the person's
 * pick. On success `/me` carries the stored zone, and the booking reads that
 * use it are asked again.
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
    onSuccess: async (user: User) => {
      // An answer that carries no zone changes nothing on screen.
      if (!user.timezone) return;
      client.setQueryData<Me>(["me"], (me) =>
        me
          ? {
              ...me,
              user: {
                ...me.user,
                timezone: user.timezone,
                timezone_chosen: user.timezone_chosen,
              },
            }
          : me,
      );
      await Promise.all([
        client.invalidateQueries({ queryKey: ["working-hours"] }),
        client.invalidateQueries({ queryKey: ["reliable-availability"] }),
      ]);
    },
  });
}

/**
 * On the first sign-in with no stored zone, stores the browser's zone as a
 * guess. Once per mount: the server keeps any zone already stored, so a second
 * browser never replaces the first one's guess or a chosen zone.
 */
export function useBrowserZoneOnFirstSignIn(me: Me | undefined) {
  const save = useSaveMyTimezone();
  const sent = useRef(false);
  const missing = me !== undefined && !me.user.timezone;
  const { mutate } = save;
  useEffect(() => {
    if (!missing || sent.current) return;
    sent.current = true;
    mutate({ timezone: viewerZone(), source: "browser" });
  }, [missing, mutate]);
}
