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

type SiteReadStopReason = NonNullable<
  components["schemas"]["SiteReadReport"]["stopped_reason"]
>;

/**
 * The stop reasons that are a CEILING the read was given, rather than something
 * that got in its way.
 *
 * This one list is what every surface asks. It lives beside the page-kind
 * vocabulary for the same reason: the company panel and the onboarding dossier
 * must not disagree about whether the same stop was a problem.
 *
 * A DECLARED MIRROR of `siteReadOwnCeiling` in the contacts module, which
 * decides the same question for the activity rail's own wording. This list said
 * two and the server's said three, so a deadline stop had the rail saying the
 * site was read and the panel warning about the same read — one reader, two
 * answers. `backend/gates/sitereadstops_test.go` fails in both directions now;
 * a reason added on either side alone is a build failure, not a disagreement
 * somebody meets in the product.
 */
const CONFIGURED_STOPS = [
  "page_cap",
  "byte_cap",
  "deadline",
] as const satisfies readonly SiteReadStopReason[];

/** A stop reason that names a budget the read spent. */
export type ConfiguredStopReason = (typeof CONFIGURED_STOPS)[number];

/**
 * Whether a stop reason means the read ran to the size it was configured for.
 *
 * A page cap, a byte cap and a wall-clock deadline are all the operator's own
 * budget, reached as designed: the read covered less than the whole site, which
 * its page count already says, and nothing went wrong. `budget` is the other
 * kind — the workspace ran out of AI credit, which is a condition somebody
 * repairs rather than a bound the crawl was built to fill, so a later run gets
 * further only once they have.
 *
 * Every stop still bounds the read and every stop is still SAID. This decides
 * the tone, not whether a reader is told.
 */
export function stopIsConfigured(
  reason: components["schemas"]["SiteReadReport"]["stopped_reason"],
): reason is ConfiguredStopReason {
  return (CONFIGURED_STOPS as readonly (string | null | undefined)[]).includes(
    reason,
  );
}
