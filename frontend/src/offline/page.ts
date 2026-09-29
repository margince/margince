// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { escapeHtml } from "../format/html";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import type { Locale } from "../i18n/locale";
import { vi } from "../i18n/vi";

type OfflineCopy = Readonly<
  Record<"offline.title" | "offline.body" | "offline.retry", string>
>;

/** English first: it is the block a page without script shows. */
const OFFLINE_CATALOGS: Readonly<Record<Locale, OfflineCopy>> = { en, de, vi };

function block(locale: string, copy: OfflineCopy, hidden: boolean): string {
  return [
    `<main class="offline" lang="${locale}"${hidden ? " hidden" : ""}>`,
    `<h1>${escapeHtml(copy["offline.title"])}</h1>`,
    `<p>${escapeHtml(copy["offline.body"])}</p>`,
    `<a class="btn btn-primary" href="" data-retry>${escapeHtml(copy["offline.retry"])}</a>`,
    "</main>",
  ].join("\n");
}

/** Styles are inline because the worker serves this page and its script, nothing else. */
export function renderOfflinePage(css: string, script: string): string {
  const blocks = Object.entries(OFFLINE_CATALOGS).map(([locale, copy], index) =>
    block(locale, copy, index > 0),
  );
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>${escapeHtml(en["offline.title"])}</title>
<style>${css}</style>
<script type="module" src="${escapeHtml(script)}"></script>
</head>
<body>
${blocks.join("\n")}
</body>
</html>
`;
}
