// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { contact360Query, contactQuery } from "../lib/contact.queries";
import { latestInboundQuery } from "../lib/latestinbound.queries";
import { parseHash } from "../lib/router";
import { opensReservedPage } from "./reservedpages";

// A pointer that rests this long on a link is heading for it; one passing over
// a list on its way elsewhere starts nothing.
export const HOVER_INTENT_MS = 80;

/** The reads a contact's page waits on before it draws. */
export function prefetchContactPage(client: QueryClient, id: string): void {
  void client.prefetchQuery(contactQuery(id));
  void client.prefetchQuery(contact360Query(id));
  void client.prefetchQuery(latestInboundQuery("contact", id));
}

/** The contact whose page a link opens, unless that page is already open. */
export function contactOpenedBy(
  link: HTMLAnchorElement,
  current: string,
): string | undefined {
  const route = parseHash(link.hash);
  if (route.screen !== "contacts" || !route.id || opensReservedPage(route)) {
    return undefined;
  }
  const open = parseHash(current);
  return open.screen === "contacts" && open.id === route.id
    ? undefined
    : route.id;
}

function linkOf(event: Event): HTMLAnchorElement | undefined {
  const target = event.target;
  if (!(target instanceof Element)) return undefined;
  const link = target.closest("a[href]");
  return link instanceof HTMLAnchorElement ? link : undefined;
}

/**
 * Starts a record page's reads when the reader points at, presses or tabs to a
 * link to it, from any list, search result or mention. The page then opens on
 * reads that are already in flight or answered, instead of starting them.
 */
export function useRecordPrefetch(): void {
  const client = useQueryClient();
  useEffect(() => {
    let pending: number | undefined;
    const start = (link: HTMLAnchorElement) => {
      const id = contactOpenedBy(link, globalThis.location.hash);
      if (id) prefetchContactPage(client, id);
    };
    const onIntent = (event: Event) => {
      window.clearTimeout(pending);
      const link = linkOf(event);
      if (link) pending = window.setTimeout(() => start(link), HOVER_INTENT_MS);
    };
    const onPress = (event: Event) => {
      window.clearTimeout(pending);
      const link = linkOf(event);
      if (link) start(link);
    };
    document.addEventListener("pointerover", onIntent);
    document.addEventListener("focusin", onIntent);
    document.addEventListener("pointerdown", onPress);
    return () => {
      window.clearTimeout(pending);
      document.removeEventListener("pointerover", onIntent);
      document.removeEventListener("focusin", onIntent);
      document.removeEventListener("pointerdown", onPress);
    };
  }, [client]);
}
