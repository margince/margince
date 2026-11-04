// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useRef,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { browserZone, isRenderableZone } from "../format/timezone";
import { useT } from "../i18n";
import { logUnexpectedError, unwrap } from "./problem";

/**
 * The signed-in user's own clock. Every time a signed-in screen shows is
 * read in it: a note saved at 16:03 in Berlin shows 16:03 to a reader in
 * Berlin, whatever zone the installation uses. `format/timezone.ts` states
 * which screens keep the company zone instead.
 *
 * Null when no provider is mounted, which only a story or a test does. Those
 * read the browser's zone, as every signed-in screen did before this.
 */
const ViewerZoneContext = createContext<string | null>(null);

/** The zone this reader's times are shown in. */
export function useViewerZone(): string {
  return useContext(ViewerZoneContext) ?? browserZone();
}

/**
 * The zone the user stored, or the browser's when none is stored or this
 * browser cannot render the stored one. A zone the formatters would refuse
 * must not take the application down, so it falls back and reports.
 */
export function renderableViewerZone(stored: string | undefined): string {
  if (stored === undefined || stored === "") return browserZone();
  if (isRenderableZone(stored)) return stored;
  logUnexpectedError(
    new Error(
      `the user's time zone is not one this browser can render: "${stored}" — times fall back to the browser's zone`,
    ),
  );
  return browserZone();
}

/** Serves one reader's zone to every screen under it. */
export function ViewerZoneProvider({
  zone,
  children,
}: Readonly<{ zone: string; children: ReactNode }>) {
  return (
    <ViewerZoneContext.Provider value={zone}>
      {children}
    </ViewerZoneContext.Provider>
  );
}

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
      { timezone: browserZone(), source: "browser" },
      {
        onError: () => {
          if (sentFor.current === userID) sentFor.current = null;
        },
      },
    );
  }, [me, mutate]);
}
