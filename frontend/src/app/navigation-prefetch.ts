// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  hashKey,
  InfiniteQueryObserver,
  onlineManager,
  type Query,
  type QueryClient,
  type QueryKey,
  QueryObserver,
  type QueryObserverResult,
} from "@tanstack/react-query";
import type { components } from "../api/schema";
import { contactsArrivalOptions } from "../features/contacts/contacts.queries";
import { dealsArrivalColumnOptions } from "../features/deals/deals.opening";
import { grants } from "../lib/accessgrants";
import { screenEnabled } from "../lib/corefeaturepolicy";
import { healthScope } from "../lib/healthscopekey";
import { pipelinesOptions } from "../lib/pipelines.queries";
import { SESSION_STALE_MS, sessionOptions } from "../lib/session";

// The rail links whose first read is warmed while a settled pointer or a
// keyboard focus rests on them. Only the bare primary links: their address
// carries no dial, so the read they open is known without the screen.
export type PrefetchTarget = "contacts" | "deals";

type Me = components["schemas"]["MeResponse"];

// A clicked link keeps its read alive this long for a destination that has
// not mounted yet; the read settling ends the lease sooner.
export const HANDOFF_MS = 10_000;

// A speculative reader watches the one read and nothing else: no polling, and
// no refetch when the window or the network returns.
const SPECULATIVE = {
  refetchInterval: false,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const;

type Lease = {
  target: PrefetchTarget;
  release: () => void;
  handedOff: boolean;
  // When a followed lease lets go, at the latest: one deadline per click.
  deadline?: number;
  timer?: ReturnType<typeof setTimeout>;
};

// Who the cached answers were read for. Two snapshots with the same scope
// would be served the same rows by the server.
function principalOf(me: Me | undefined): string | undefined {
  const scope = healthScope(me);
  return scope === undefined
    ? undefined
    : JSON.stringify([
        scope,
        me?.roles,
        me?.settings_availability?.disabled_features,
      ]);
}

function readsTarget(me: Me, target: PrefetchTarget): boolean {
  const disabled = me.settings_availability?.disabled_features ?? [];
  const object = target === "contacts" ? "contact" : "deal";
  return screenEnabled(target, disabled) && grants(me, object, "read");
}

/**
 * One owner of navigation-intent reads per QueryClient. It warms one target at
 * a time, under the exact options its screen mounts. A change of signed-in
 * principal drops every entry it created.
 */
export class NavigationPrefetch {
  readonly #client: QueryClient;
  #lease: Lease | undefined;
  #principal: string | undefined;
  // The reader scope the warmed entries are keyed under.
  #readerScope: string | undefined;
  // Bumped whenever the principal changes, so a catalog answer read for one
  // principal never starts a column read for the next.
  #generation = 0;
  readonly #owned = new Set<Query>();

  constructor(client: QueryClient) {
    this.#client = client;
    const sessionHash = hashKey(sessionOptions.queryKey);
    client.getQueryCache().subscribe((event) => {
      if (event.query.queryHash !== sessionHash) return;
      const me =
        event.type === "removed"
          ? undefined
          : client.getQueryData<Me>(sessionOptions.queryKey);
      if (this.#principal !== undefined && principalOf(me) !== this.#principal)
        this.#forget(healthScope(me));
    });
  }

  /** A settled pointer or keyboard focus rests on the link to `target`. */
  intend(target: PrefetchTarget): void {
    if (this.#lease?.target === target) return;
    this.#releaseLease();
    const me = this.#currentSession();
    if (!me || !readsTarget(me, target)) return;
    if (document.visibilityState !== "visible" || !onlineManager.isOnline())
      return;
    this.#principal = principalOf(me);
    // #currentSession answers only a snapshot whose scope is known.
    const readerScope = healthScope(me) ?? "";
    this.#readerScope = readerScope;
    if (target === "contacts") {
      const options = contactsArrivalOptions(readerScope);
      this.#warm(
        target,
        options.queryKey,
        new InfiniteQueryObserver(this.#client, { ...options, ...SPECULATIVE }),
      );
      return;
    }
    this.#warmDeals(readerScope);
  }

  /** The pointer or focus left the link to `target` without following it. */
  leave(target: PrefetchTarget): void {
    if (this.#lease?.target === target && !this.#lease.handedOff)
      this.#releaseLease();
  }

  /** The link to `target` was followed in this tab: its screen will adopt the read. */
  follow(target: PrefetchTarget, deadline = Date.now() + HANDOFF_MS): void {
    const lease = this.#lease;
    if (lease?.target !== target || lease.handedOff) return;
    lease.handedOff = true;
    lease.deadline = deadline;
    lease.timer = setTimeout(
      () => this.#releaseLease(lease),
      Math.max(0, deadline - Date.now()),
    );
  }

  // A session snapshot that is answered, current and not being re-read. A
  // hover never asks /me itself; a doubtful snapshot only means no prefetch.
  #currentSession(): Me | undefined {
    const state = this.#client.getQueryState(sessionOptions.queryKey);
    if (
      state?.status !== "success" ||
      state.isInvalidated ||
      state.fetchStatus !== "idle" ||
      Date.now() - state.dataUpdatedAt >= SESSION_STALE_MS
    )
      return undefined;
    return principalOf(state.data) === undefined ? undefined : state.data;
  }

  #warmDeals(readerScope: string): void {
    const catalog = this.#client.getQueryData(pipelinesOptions.queryKey);
    if (catalog) {
      this.#warmColumn(catalog, readerScope);
      return;
    }
    const generation = this.#generation;
    const catalogLease = this.#warm(
      "deals",
      pipelinesOptions.queryKey,
      new QueryObserver(this.#client, { ...pipelinesOptions, ...SPECULATIVE }),
      (pipelines) => {
        if (generation !== this.#generation) return;
        this.#warmColumn(pipelines, readerScope);
        // The column inherits a click that landed while the catalog was in flight.
        if (catalogLease?.deadline !== undefined)
          this.follow("deals", catalogLease.deadline);
      },
    );
  }

  #warmColumn(
    pipelines: components["schemas"]["Pipeline"][],
    readerScope: string,
  ): void {
    const options = dealsArrivalColumnOptions(pipelines, readerScope);
    if (options)
      this.#warm(
        "deals",
        options.queryKey,
        new QueryObserver(this.#client, { ...options, ...SPECULATIVE }),
      );
  }

  // One observer holds a read the cache lacks, so leaving the link can abort
  // it. It lets go once the read settles. A populated entry is left to the
  // screen's own mount, fresh or stale.
  #warm<Data>(
    target: PrefetchTarget,
    queryKey: QueryKey,
    observer: {
      subscribe: (
        listener: (result: QueryObserverResult<Data>) => void,
      ) => () => void;
    },
    onData?: (data: Data) => void,
  ): Lease | undefined {
    const cache = this.#client.getQueryCache();
    const held = cache.find({ queryKey, exact: true });
    if (
      held?.state.data !== undefined ||
      held?.state.fetchStatus === "fetching"
    )
      return undefined;
    this.#releaseLease();
    const lease: Lease = { target, handedOff: false, release: () => {} };
    this.#lease = lease;
    lease.release = observer.subscribe((result) => {
      if (result.fetchStatus !== "idle" || result.status === "pending") return;
      // A lease already replaced or left settles without starting the next read.
      const current = this.#lease === lease;
      this.#releaseLease(lease);
      if (current && result.status === "success") onData?.(result.data);
    });
    const query = cache.find({ queryKey, exact: true });
    if (query) this.#owned.add(query);
    return lease;
  }

  #releaseLease(lease = this.#lease): void {
    if (!lease) return;
    clearTimeout(lease.timer);
    lease.release();
    if (this.#lease === lease) this.#lease = undefined;
  }

  // The principal changed: no answer read for the old one may reach a screen
  // as fresh. Unwatched entries go. A mounted screen reads its entry again,
  // unless the entry is keyed by a reader scope that has changed: that screen
  // moves to its new key on its own, and reading the old one is wasted.
  #forget(readerScope: string | undefined): void {
    this.#releaseLease();
    this.#generation += 1;
    this.#principal = undefined;
    const previous = this.#readerScope;
    this.#readerScope = undefined;
    const cache = this.#client.getQueryCache();
    for (const query of this.#owned) {
      if (cache.get(query.queryHash) !== query) continue;
      const rekeyed =
        previous !== undefined &&
        previous !== readerScope &&
        query.queryKey.includes(previous);
      if (query.getObserversCount() === 0) cache.remove(query);
      // A reset clears the old answer before the screen reads again; an
      // invalidation would keep showing it while the new read runs.
      else if (!rekeyed)
        void this.#client.resetQueries({
          queryKey: query.queryKey,
          exact: true,
        });
    }
    this.#owned.clear();
  }
}

const controllers = new WeakMap<QueryClient, NavigationPrefetch>();

/** The client's one navigation-intent owner, made on first use. */
export function navigationPrefetch(client: QueryClient): NavigationPrefetch {
  let controller = controllers.get(client);
  if (!controller) {
    controller = new NavigationPrefetch(client);
    controllers.set(client, controller);
  }
  return controller;
}
