// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { useContactGraph } from "../contactgraph";
import { isOpenAsk } from "../introasks";
import type { IntroRequest } from "../introrequests";

export type Graph = NonNullable<ReturnType<typeof useContactGraph>["data"]>;

export type GraphReading = ReturnType<typeof readGraph>;

/**
 * readGraph is the page's whole reading of one payload, in one place. Laying
 * out the chapters and deriving these values from two reads are two jobs.
 */
export function readGraph(
  data: Graph,
  asks: readonly IntroRequest[],
  focus: string | null,
) {
  const nodes = data.nodes ?? [];
  const routes = data.routes ?? [];
  const alternatives =
    routes.length > 1 || (routes.length === 0 && !!data.route);
  const withheld = (data.groups_omitted ?? []).length > 0;
  const open = asks.find(isOpenAsk);
  return {
    anchor: nodes.find((n) => n.group === "anchor"),
    targetName: nodes.find((n) => n.group === "anchor")?.label ?? "",
    routes,
    lead: routes[0],
    // The ask in flight. Only one route per colleague can be open at a time,
    // and the newest is the one this page is about.
    open,
    // The card shows what the lead panel cannot: the other routes, or the lone
    // `route` of a server that predates the candidate list. An empty `routes`
    // with no `route` means nobody reaches the contact, so there is no card.
    alternatives,
    // Job titles for the lead's chain, read off the same payload's nodes.
    roles: {
      via: nodes.find((n) => n.user_id === routes[0]?.via_user_id)?.sublabel,
      through: nodes.find(
        (n) =>
          n.contact_id !== undefined &&
          n.contact_id === routes[0]?.through_contact_id,
      )?.sublabel,
      target: nodes.find((n) => n.group === "anchor")?.sublabel,
    },
    legacy: routes.length === 0 ? data.route : undefined,
    withheld,
    // A map of the contact alone, and a handoff whose first step picks from no
    // routes, would only restate the way-in answer in two pictures.
    lonely: nodes.every((n) => n.group === "anchor"),
    showRelay: open !== undefined || routes.length > 0 || !!data.route,
    // Whether the working grid has a left column at all.
    hasMain: alternatives || withheld,
    // The drawing's verb, joined back to the route it means. A node has the
    // graph's own id and a route names a user id, so the join is `user_id`.
    routeAt: (nodeId: string) => {
      const node = nodes.find((n) => n.id === nodeId);
      return node?.user_id === undefined
        ? undefined
        : routes.find((r) => r.via_user_id === node.user_id);
    },
    // A selection holds only against the graph on screen. The tab stays mounted
    // from one contact to the next, and a graph may reload without the node.
    focused: nodes.some((n) => n.id === focus) ? focus : null,
  };
}
