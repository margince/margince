// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

/**
 * What kind of page the crawl was looking at, in the reader's words. The enum
 * is closed and both read shapes carry it (`SiteReadPage.kind`, required, and
 * `CompanySiteReadPage.kind`, optional), so the vocabulary lives here once: a
 * company page, a deep-read report and the onboarding dossier must not name the
 * same page three different ways.
 */
const SITE_READ_KIND_LABELS: Record<
  components["schemas"]["SiteReadPage"]["kind"],
  MessageKey
> = {
  home: "deepread.kindHome",
  impressum: "deepread.kindImpressum",
  about: "deepread.kindAbout",
  team: "deepread.kindTeam",
  services: "deepread.kindServices",
  products: "deepread.kindProducts",
  contact: "deepread.kindContact",
  other: "deepread.kindOther",
};

// The same map seen as a plain lookup, for callers whose kind is only a string
// at compile time. Widening an assignment costs nothing and keeps the map above
// exhaustive over the enum — a cast at the call site would give up both.
const KIND_LABELS_BY_NAME: Readonly<Record<string, MessageKey>> =
  SITE_READ_KIND_LABELS;

/**
 * The same vocabulary for a caller that already has a label of its own and only
 * wants a better one. An absent kind and "other" both answer undefined: they say
 * nothing the caller's own wording does not, and "Other" in place of a real name
 * reads as information when it is not.
 */
export function namedSiteReadKind(
  kind: string | null | undefined,
): MessageKey | undefined {
  if (!kind || kind === "other") {
    return undefined;
  }
  return KIND_LABELS_BY_NAME[kind];
}

/**
 * Whether a stop reason means the read ran to the size it was configured for.
 *
 * A page or byte cap is the operator's own budget, reached as designed: the
 * read covered less than the whole site, which its page count already says, and
 * nothing went wrong. Budget and deadline are the other kind — the read wanted
 * to carry on and something outside it intervened, so a later run may get
 * further and a reader should be told to look.
 *
 * Both still bound the read and both are still SAID. This decides the tone, not
 * whether a reader is told. It lives beside the page-kind vocabulary for the
 * same reason: the company panel and the onboarding dossier must not disagree
 * about whether the same stop was a problem.
 */
export function stopIsConfigured(
  reason: components["schemas"]["SiteReadReport"]["stopped_reason"],
): boolean {
  return reason === "page_cap" || reason === "byte_cap";
}
