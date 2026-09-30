// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n/locale";

/** Without this script the page still reads, in its first block, and its retry
 *  link still reloads, losing only the route after the #. Returns its removal. */
export function presentOfflinePage(page: Document, locale: Locale): () => void {
  const listening = new AbortController();
  const blocks = Array.from(page.querySelectorAll<HTMLElement>("main[lang]"));
  const shown = blocks.find((block) => block.lang === locale) ?? blocks[0];
  if (shown === undefined) {
    return () => listening.abort();
  }
  for (const block of blocks) {
    block.hidden = block !== shown;
  }
  page.documentElement.lang = shown.lang;
  page.title = shown.querySelector("h1")?.textContent ?? page.title;
  const reload = () => page.location.reload();
  for (const retry of page.querySelectorAll("[data-retry]")) {
    retry.addEventListener(
      "click",
      (event) => {
        event.preventDefault();
        reload();
      },
      { signal: listening.signal },
    );
  }
  // The page promises Margince loads again once the connection is back.
  page.defaultView?.addEventListener("online", reload, {
    once: true,
    signal: listening.signal,
  });
  return () => listening.abort();
}
