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
  type QueryObserverOptions,
  type QueryObserverResult,
} from "@tanstack/react-query";
import type { components } from "../api/schema";
import { contactsArrivalOptions } from "../features/contacts/contacts.queries";
import { dealsArrivalColumnOptions } from "../features/deals/deals.opening";
import { grants } from "../lib/accessgrants";
import { contact360Query, contactQuery } from "../lib/contact.queries";
import { screenEnabled } from "../lib/corefeaturepolicy";
import { healthScope } from "../lib/healthscopekey";
import { latestInboundQuery } from "../lib/latestinbound.queries";
import { pipelinesOptions } from "../lib/pipelines.queries";
import { SESSION_STALE_MS, sessionOptions } from "../lib/session";

// The rail links whose first read is warmed while a settled pointer or a
// keyboard focus rests on them. Only the bare primary links: their address
// carries no dial, so the read they open is known without the screen.
export type PrefetchTarget = "contacts" | "deals";

// A link to one contact's page warms the reads that page waits on to draw.
export type RecordTarget = `contact:${string}`;

export function contactTarget(id: string): RecordTarget {
  return `contact:${id}`;
}

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
  target: PrefetchTarget | RecordTarget;
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
      if (event.type === "removed") this.#owned.delete(event.query);
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
    const me = this.#readerFor(target);
    if (!me) return;
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

  /** A settled pointer, a press or a keyboard focus rests on a link to one contact. */
  intendContact(id: string): void {
    const target = contactTarget(id);
    if (this.#lease?.target === target) return;
    this.#releaseLease();
    if (!this.#readerFor("contacts")) return;
    const releases = [
      this.#holdRead(contactQuery(id)),
      this.#holdRead(contact360Query(id)),
      this.#holdRead(latestInboundQuery("contact", id)),
    ].filter((release) => release !== undefined);
    if (releases.length === 0) return;
    this.#lease = {
      target,
      handedOff: false,
      release: () => {
        for (const release of releases) release();
      },
    };
  }

  /** The pointer or focus left the link to `target` without following it. */
  leave(target: PrefetchTarget | RecordTarget): void {
    if (this.#lease?.target === target && !this.#lease.handedOff)
      this.#releaseLease();
  }

  /** The link to `target` was followed in this tab: its screen will adopt the read. */
  follow(
    target: PrefetchTarget | RecordTarget,
    deadline = Date.now() + HANDOFF_MS,
  ): void {
    const lease = this.#lease;
    if (lease?.target !== target || lease.handedOff) return;
    lease.handedOff = true;
    lease.deadline = deadline;
    lease.timer = setTimeout(
      () => this.#releaseLease(lease),
      Math.max(0, deadline - Date.now()),
    );
  }

  // The session that may warm `screen`'s reads now, which also becomes the
  // principal the warmed entries are read for.
  #readerFor(screen: PrefetchTarget): Me | undefined {
    const me = this.#currentSession();
    if (!me || !readsTarget(me, screen)) return undefined;
    if (document.visibilityState !== "visible" || !onlineManager.isOnline())
      return undefined;
    this.#principal = principalOf(me);
    return me;
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
    if (this.#occupied(queryKey)) return undefined;
    this.#releaseLease();
    const lease: Lease = { target, handedOff: false, release: () => {} };
    this.#lease = lease;
    lease.release =
      this.#hold(queryKey, observer, (result) => {
        // A lease already replaced or left settles without starting the next read.
        const current = this.#lease === lease;
        this.#releaseLease(lease);
        if (current && result.status === "success") onData?.(result.data);
      }) ?? (() => {});
    return lease;
  }

  // One speculative observer for one read, let go once the read answers.
  #holdRead<Data, Key extends QueryKey>(
    options: QueryObserverOptions<Data, Error, Data, Data, Key>,
  ): (() => void) | undefined {
    let release: (() => void) | undefined;
    release = this.#hold(
      options.queryKey,
      new QueryObserver(this.#client, { ...options, ...SPECULATIVE }),
      () => release?.(),
    );
    return release;
  }

  // Whether the cache already holds or is reading the entry.
  #occupied(queryKey: QueryKey): boolean {
    const held = this.#client.getQueryCache().find({ queryKey, exact: true });
    return (
      held?.state.data !== undefined || held?.state.fetchStatus === "fetching"
    );
  }

  // Subscribes one observer to an entry the cache lacks and owns that entry,
  // calling onSettled once its read answers. Undefined when the entry is held.
  #hold<Data>(
    queryKey: QueryKey,
    observer: {
      subscribe: (
        listener: (result: QueryObserverResult<Data>) => void,
      ) => () => void;
    },
    onSettled: (result: QueryObserverResult<Data>) => void,
  ): (() => void) | undefined {
    if (this.#occupied(queryKey)) return undefined;
    const release = observer.subscribe((result) => {
      if (result.fetchStatus !== "idle" || result.status === "pending") return;
      onSettled(result);
    });
    const query = this.#client.getQueryCache().find({ queryKey, exact: true });
    if (query) this.#owned.add(query);
    return release;
  }

  #releaseLease(lease = this.#lease): void {
    if (!lease) return;
    clearTimeout(lease.timer);
    lease.release();
    if (this.#lease === lease) this.#lease = undefined;
  }

  // The principal changed: no answer read for the old one may reach a screen
  // as fresh. Unwatched entries go. A mounted screen reads its entry again.
  // An entry keyed by a changed reader scope is the exception: its screen
  // moves to the new key itself.
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
