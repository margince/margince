// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";

// The Google editors name their kind in the first path segment.
const GOOGLE_DOCS_KINDS: Readonly<Record<string, MessageKey>> = {
  document: "docs.kind.docs",
  spreadsheets: "docs.kind.sheets",
  presentation: "docs.kind.slides",
  forms: "docs.kind.forms",
};

/**
 * What a link row's stage says it is. A link has no file extension, and its
 * host would read as one ("COM"), so the address names the kind instead.
 */
export function linkKind(address: string): MessageKey {
  let url: URL;
  try {
    url = new URL(address);
  } catch {
    return "docs.kind.web";
  }
  const host = url.hostname.toLowerCase();
  if (host === "docs.google.com") {
    const segment = url.pathname.split("/")[1] ?? "";
    return GOOGLE_DOCS_KINDS[segment] ?? "docs.kind.drive";
  }
  if (host === "drive.google.com") {
    return "docs.kind.drive";
  }
  return "docs.kind.web";
}
