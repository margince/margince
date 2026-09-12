// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { foldForMatch } from "../format/collate";
import type { MessageKey } from "../i18n/en";
import type { SettingsPage, SettingsPageId } from "./settingscatalog";

/**
 * Finding a setting by the word a reader already has for it.
 *
 * Pure, and deliberately: it takes the pages this reader may open rather than
 * asking who they are, so the same list that draws the sidebar decides what the
 * search can find. A search that resolved permission separately would eventually
 * disagree with the navigation beside it, and the disagreement a reader notices
 * is the one where search offers a page that then refuses them.
 *
 * It never calls an endpoint. Twenty-eight rows is not a query.
 */

/**
 * The words a page answers to besides its own name.
 *
 * Three kinds, and each earns its place:
 *
 *   - the OLD name, so a reader who learnt "General" still finds Company
 *     profile and a stale bookmark's word still works;
 *   - the plain noun, because a reader searches for the thing they want to
 *     change ("password", "logo", "vat") rather than the page that holds it;
 *   - the neighbouring word, where two pages could each plausibly answer —
 *     "email" reaches both Connections and Capture, and letting one of them
 *     hide the other would make the search wrong for half of its askers.
 *
 * English only, on purpose. These are search aliases rather than copy: a
 * German reader's query lands on the translated LABEL, which is already
 * indexed, and inventing a German alias for a word nobody types would be a
 * translation of a guess.
 */
const ALIASES: Partial<Record<SettingsPageId, readonly string[]>> = {
  account: [
    "password",
    "name",
    "email",
    "language",
    "signature",
    "profile",
    "brief",
    "theme",
    "appearance",
  ],
  voice: ["writing", "tone", "style", "drafts"],
  agents: ["passport", "token", "api", "mcp", "credentials", "automation"],
  connections: ["mailbox", "imap", "gmail", "outlook", "email", "linkedin"],
  "capture-activity": ["mail", "email", "held", "judgement", "why"],
  company: [
    "general",
    "installation",
    "currency",
    "logo",
    "vat",
    "address",
    "timezone",
    "fiscal year",
    "context",
  ],
  authentication: [
    "sign in",
    "login",
    "sso",
    "oidc",
    "oauth",
    "google",
    "microsoft",
    "app",
  ],
  members: ["users", "contacts", "invite", "seat", "deactivate", "roles"],
  teams: ["team", "group", "manager"],
  seats: ["license", "licence", "entitlement", "capacity", "headcount"],
  pipelines: ["stage", "deal", "funnel", "won", "lost"],
  leads: ["lead", "source", "vocabulary", "qualification"],
  fields: ["custom field", "data model", "column", "attribute"],
  tags: ["tag", "label", "vocabulary"],
  products: ["product", "offer", "template", "price", "catalog"],
  capture: ["mail", "email", "domain", "sharing", "inbox", "rules"],
  integrations: ["webhook", "overlay", "hubspot", "mirror", "sync"],
  knowledge: ["document", "corpus", "rag", "upload"],
  import: ["csv", "upload", "migration", "bulk"],
  models: ["ai", "routing", "provider", "anthropic", "openai", "key"],
  automations: ["rule", "workflow", "trigger", "automation"],
  usage: ["spend", "cost", "budget", "tokens", "money"],
  "model-calls": ["trace", "log", "call", "debug", "prompt"],
  privacy: ["gdpr", "dsr", "consent", "retention", "erasure", "purpose"],
  audit: ["trail", "log", "who", "history", "compliance"],
  "system-health": ["jobs", "queue", "reindex", "maintenance", "stuck"],
  extensions: ["unit", "plugin", "module", "extension"],
  reset: ["empty", "wipe", "delete everything", "danger"],
};

/**
 * A form of the text that matches the way contacts actually type.
 *
 * Diacritics folded, because a reader searching a German or Vietnamese label
 * types what their keyboard gives them and a fold in one direction only would
 * make "Zurucksetzen" miss "Zurücksetzen" while the reverse worked.
 *
 * The CASE fold is `foldForMatch`, not `toLocaleLowerCase`: a locale-aware fold
 * maps Turkish `I` to a dotless `ı`, so a needle folded under one locale stops
 * matching a haystack folded under another — and both sides of this comparison
 * are strings the product holds rather than a sentence anybody reads.
 */
function fold(text: string): string {
  return foldForMatch(text.normalize("NFD").replace(/\p{Diacritic}/gu, ""));
}

export type SettingsHit = {
  page: SettingsPage;
  /** The page's own name, already translated — what the row shows. */
  label: string;
  /** The group it sits under, so a hit says where it lives. */
  group: string;
};

/**
 * The pages matching `query`, best first.
 *
 * `pages` is the reader's OWN visible list, so nothing here can offer a page
 * the sidebar would not. An empty query returns nothing rather than everything:
 * the sidebar is already the list of everything, and repeating it under a
 * search box would say the reader had searched for something.
 */
export function settingsSearch(
  query: string,
  pages: readonly SettingsPage[],
  t: (key: MessageKey) => string,
): readonly SettingsHit[] {
  // Split on whitespace, and every word must land somewhere. A reader types
  // "email signature" as one thought, but the two words live in different
  // fields — "email" in the page's aliases, "signature" in another alias
  // beside it — so demanding that the whole phrase be one substring answered
  // nothing for exactly the queries a contact composes naturally.
  const words = fold(query)
    .split(/\s+/)
    .filter((word) => word !== "");
  if (words.length === 0) {
    return [];
  }
  const scored: { hit: SettingsHit; rank: number }[] = [];
  for (const page of pages) {
    const label = t(`settings.tab.${page.id}`);
    const group = t(`settings.group.${page.group}`);
    const sub = t(`settings.page.${page.id}.sub`);
    const fields = [
      // Order is the ranking: a hit on the page's own name beats one on its
      // description, which beats one on a word it merely answers to.
      fold(label),
      fold(group),
      ...(ALIASES[page.id] ?? []).map(fold),
      fold(sub),
    ];
    // Every word, or the page does not answer. Ranked by the BEST field any
    // one word reached, so "email signature" ranks under the same rule a
    // single word would — and a page matching only half the phrase is out
    // rather than ranked low, because a reader who typed two words meant both.
    const ranks = words.map((word) => rankOf(word, fields));
    if (ranks.every((rank) => rank !== undefined)) {
      scored.push({
        hit: { page, label, group },
        rank: Math.min(...(ranks as number[])),
      });
    }
  }
  return scored
    .sort(
      (a, b) =>
        a.rank - b.rank ||
        // Ties keep CATALOG order rather than falling to alphabetical: the
        // reader has just been looking at that order in the sidebar.
        pages.indexOf(a.hit.page) - pages.indexOf(b.hit.page),
    )
    .map((entry) => entry.hit);
}

/**
 * How well the needle matches, lower being better, or undefined for no match.
 *
 * A prefix beats a contained substring at the same field, so typing "au" puts
 * Audit log above the pages that merely contain those letters. Beyond that the
 * FIELD decides, which is what the caller's ordering above encodes.
 */
function rankOf(needle: string, fields: readonly string[]): number | undefined {
  for (const [index, field] of fields.entries()) {
    if (field.startsWith(needle)) {
      return index * 2;
    }
    if (field.includes(needle)) {
      return index * 2 + 1;
    }
  }
  return undefined;
}

export { fold as foldForSearch };
