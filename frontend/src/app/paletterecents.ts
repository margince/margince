// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useMemo, useSyncExternalStore } from "react";
import type { components } from "../api/schema";
import { healthScope } from "../lib/healthscopekey";
import { readSession, useMe } from "../lib/session";
import { createRecentReader, type ShownRecent } from "./paletterecents.reader";

type Me = components["schemas"]["MeResponse"];
const EMPTY: ShownRecent[] = [];
export function usePaletteRecents(open: boolean, empty: boolean) {
  const client = useQueryClient();
  const me = useMe().data;
  const scope = healthScope(me);
  const seat = me?.user.id;
  const reader = useMemo(
    () =>
      createRecentReader(
        seat,
        Boolean(open && scope),
        () => healthScope(client.getQueryData<Me>(["me"])) === scope,
        // Another tab can end the session while its cached identity is still present.
        async (signal) => {
          const request = new AbortController();
          const abort = () => request.abort();
          signal.addEventListener("abort", abort, { once: true });
          if (signal.aborted) request.abort();
          const stop = client.getQueryCache().subscribe(() => {
            if (healthScope(client.getQueryData<Me>(["me"])) !== scope)
              request.abort();
          });
          try {
            return healthScope(await readSession(request.signal)) === scope;
          } catch {
            return false;
          } finally {
            stop();
            signal.removeEventListener("abort", abort);
          }
        },
      ),
    [client, seat, scope, open],
  );
  const state = useSyncExternalStore(
    reader.subscribe,
    reader.getSnapshot,
    reader.getSnapshot,
  );
  return {
    ...state,
    entries: empty ? state.entries : EMPTY,
    remember: reader.remember,
    openRecord: reader.openRecord,
    clear: reader.clear,
    cancel: reader.cancel,
  };
}
