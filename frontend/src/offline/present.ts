// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** Without this script the page still reads, in its first block, and its retry
 *  link still reloads, losing only the route after the #. */
export function presentOfflinePage(
  page: Document,
  stored: string | null,
  languages: readonly string[],
): void {
  const blocks = Array.from(page.querySelectorAll<HTMLElement>("main[lang]"));
  // Not i18n's detectLocale: importing it would carry every catalog into this page.
  const asked = [stored, ...languages.map((tag) => tag.split("-")[0])];
  const shown =
    asked
      .map((locale) =>
        blocks.find((block) => block.lang === locale?.toLowerCase()),
      )
      .find((block) => block !== undefined) ?? blocks[0];
  if (shown === undefined) {
    return;
  }
  for (const block of blocks) {
    block.hidden = block !== shown;
  }
  page.documentElement.lang = shown.lang;
  page.title = shown.querySelector("h1")?.textContent ?? page.title;
  for (const retry of page.querySelectorAll("[data-retry]")) {
    retry.addEventListener("click", (event) => {
      event.preventDefault();
      page.location.reload();
    });
  }
}
