// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  foldForSearch as fold,
  holdsEveryWord,
  searchWords,
} from "../../format/collate";
import type { MessageKey } from "../../i18n/en";
import {
  groupHeadingKey,
  type SettingsPage,
  type SettingsPageId,
} from "../../lib/settingscatalog";
import {
  matchesConfigurationFilter,
  type SettingName,
} from "./configurationentries";
import {
  COMPANY_CONTEXT_SECTIONS,
  SETTINGS_SECTIONS,
} from "./settingssections";

/**
 * Finding a setting by the word a reader already has for it.
 *
 * Pure, and deliberately: it takes the pages this reader may open rather than
 * asking who they are, so the same list that draws the sidebar decides what the
 * search can find. A search that resolved permission separately would eventually
 * disagree with the navigation beside it, and the disagreement a reader notices
 * is the one where search offers a page that then refuses them.
 *
 * It never calls an endpoint: the catalog is a short table, not a query. The
 * installation's setting names are the one list the server holds, and the
 * caller passes them in.
 */

// Old names and the nouns a reader types, in English: a translated query already
// matches the translated label. The rail's search and the palette share them.
const ALIASES: Partial<Record<SettingsPageId, readonly string[]>> = {
  account: [
    "password",
    "name",
    "email",
    "language",
    "signature",
    "profile",
    "theme",
    "appearance",
    "install",
  ],
  meetings: [
    "calendar",
    "availability",
    "working hours",
    "timezone",
    "booking",
    "buffer",
    "notice",
  ],
  voice: ["writing style", "tone", "drafts"],
  agents: [
    "agents",
    "ai",
    "claude",
    "chatgpt",
    "copilot",
    "gemini",
    "connector",
    "passport",
    "token",
    "api",
    "api key",
    "mcp",
    "credentials",
    "automation",
  ],
  notifications: [
    "notifications",
    "alerts",
    "digest",
    "email",
    "brief",
    "home",
  ],
  connections: [
    "mailbox",
    "imap",
    "gmail",
    "outlook",
    "email",
    "linkedin",
    "oauth",
    "calendar",
  ],
  "capture-activity": [
    "mail",
    "email",
    "held",
    "judgement",
    "why",
    "capture log",
    "trace",
  ],
  company: [
    "general",
    "installation",
    "workspace",
    "currency",
    "fx",
    "timezone",
    "fiscal year",
  ],
  authentication: [
    "sign in",
    "sign-in",
    "login",
    "sso",
    "oidc",
    "oauth app",
    "google",
    "microsoft",
    "token",
  ],
  members: [
    "users",
    "contacts",
    "roster",
    "invite",
    "seat",
    "deactivate",
    "roles",
  ],
  teams: ["team", "group", "manager"],
  roles: ["role", "permission", "access", "rights", "scope", "grant"],
  seats: ["license", "licence", "entitlement", "capacity", "headcount"],
  pipelines: ["stages", "deal stages", "funnel", "won", "lost"],
  leads: ["lead sources", "vocabulary", "qualification", "disqualify"],
  fields: [
    "custom fields",
    "custom-fields",
    "data model",
    "schema",
    "column",
    "attribute",
  ],
  tags: ["labels", "vocabulary"],
  products: [
    "product",
    "offer templates",
    "offer-templates",
    "price list",
    "rate card",
    "catalog",
  ],
  capture: [
    "mail",
    "email",
    "email capture",
    "domain",
    "sharing",
    "inbox",
    "rules",
    "website",
    "enrichment",
    "limit",
  ],
  integrations: ["webhook", "api"],
  knowledge: ["documents", "handbook", "corpus", "rag", "upload"],
  import: ["csv", "upload", "migration", "bulk"],
  models: [
    "ai",
    "routing",
    "providers",
    "anthropic",
    "openai",
    "keys",
    "embeddings",
  ],
  automations: ["rules", "workflow", "triggers", "automation"],
  usage: ["spend", "cost", "budget", "tokens", "money"],
  mcp: [
    "agent",
    "mcp",
    "oauth",
    "client id",
    "client secret",
    "claude",
    "chatgpt",
    "codex",
    "gemini",
    "copilot",
    "dcr",
    "cimd",
    "token",
  ],
  "model-calls": ["trace", "logs", "calls", "debug", "prompt"],
  privacy: ["gdpr", "dsr", "consent", "retention", "erasure", "purpose"],
  jurisdiction: ["country", "law", "legal", "pack", "deadline", "floor"],
  audit: ["trail", "log", "who", "history", "compliance"],
  "system-health": ["jobs", "queue", "reindex", "maintenance", "stuck"],
  configuration: [
    "environment",
    "variable",
    "env",
    "env file",
    "flag",
    "yaml",
    "source",
    "locked",
  ],
  extensions: ["units", "plugins", "modules", "extension"],
  reset: ["empty", "wipe", "delete everything", "danger"],
};

// The company-context card's own words. That card draws only where
// `companyContextShown` says, so another reader would land on a page without them.
const PROFILE_ALIASES: readonly string[] = [
  "logo",
  "vat",
  "address",
  "context",
];

/** What the search needs to know about the reader asking. */
type SettingsSearchReader = { readonly seesCompanyContext: boolean };

/** The reader a caller gets when it names none: no company-context words. */
const NO_COMPANY_CONTEXT: SettingsSearchReader = { seesCompanyContext: false };

/** The words `page` answers to besides its own name, for this reader. */
export function settingsPageAliases(
  page: SettingsPageId,
  reader: SettingsSearchReader = NO_COMPANY_CONTEXT,
): readonly string[] {
  return [
    ...(ALIASES[page] ?? []),
    ...(page === "company" && reader.seesCompanyContext ? PROFILE_ALIASES : []),
  ];
}

/** The names of the panels and settings `page` draws, in the reader's words. */
export function settingsPageSections(
  page: SettingsPageId,
  t: (key: MessageKey) => string,
  reader: SettingsSearchReader = NO_COMPANY_CONTEXT,
): readonly string[] {
  return [
    ...SETTINGS_SECTIONS[page],
    ...(page === "company" && reader.seesCompanyContext
      ? COMPANY_CONTEXT_SECTIONS
      : []),
  ].map((key) => t(key));
}

/** Everything `page` answers to besides its own name, for the ⌘K palette. */
export function settingsPageKeywords(
  page: SettingsPageId,
  t: (key: MessageKey) => string,
  reader: SettingsSearchReader = NO_COMPANY_CONTEXT,
): readonly string[] {
  return [
    ...settingsPageAliases(page, reader),
    ...settingsPageSections(page, t, reader),
  ];
}

export type SettingsHit = {
  page: SettingsPage;
  /** The page's own name, already translated — what the row shows. */
  label: string;
  /** The group it sits under, so a hit says where it lives. */
  group: string;
  /** The setting on the page that matched, when nothing above it did. */
  section: string | undefined;
  /** What the page's filter opens on, when only a setting's name matched. */
  filter?: string;
};

/** The page a setting's key or variable finds. */
export const CONFIGURATION_PAGE: SettingsPageId = "configuration";

/**
 * How Configuration answers a query that names settings by key or variable:
 * the filter it opens on, and the key matched when only one was.
 *
 * The page's own filter decides the match, so the page never opens empty.
 */
export function configurationFilterFor(
  query: string,
  settings: readonly SettingName[],
): { filter: string; named: string | undefined } | undefined {
  const filter = query.trim();
  if (filter === "") {
    return undefined;
  }
  const matched = settings.filter((setting) =>
    matchesConfigurationFilter(setting, filter),
  );
  if (matched.length === 0) {
    return undefined;
  }
  return { filter, named: matched.length === 1 ? matched[0]?.key : undefined };
}

/**
 * The pages matching `query`, best first.
 *
 * `pages` is the reader's OWN visible list, so nothing here can offer a page
 * the sidebar would not. An empty query returns nothing rather than everything:
 * the sidebar is already the list of everything, and repeating it under a
 * search box would say the reader had searched for something.
 *
 * `settings` are the installation's setting names, empty for a reader who may
 * not open Configuration.
 */
export function settingsSearch(
  query: string,
  pages: readonly SettingsPage[],
  t: (key: MessageKey) => string,
  reader: SettingsSearchReader = NO_COMPANY_CONTEXT,
  settings: readonly SettingName[] = [],
): readonly SettingsHit[] {
  // Split on whitespace, and every word must land somewhere. A reader types
  // "email signature" as one thought, but the two words live in different
  // fields — "email" in the page's aliases, "signature" in another alias
  // beside it — so demanding that the whole phrase be one substring answered
  // nothing for exactly the queries a contact composes naturally.
  const words = searchWords(query);
  if (words.length === 0) {
    return [];
  }
  const scored: { hit: SettingsHit; rank: number }[] = [];
  for (const page of pages) {
    const label = t(`settings.tab.${page.id}`);
    const group = t(groupHeadingKey(page.group));
    const sub = t(`settings.page.${page.id}.sub`);
    const sections = settingsPageSections(page.id, t, reader);
    // Order is the ranking: the page's own name, its group, the words it
    // answers to, the settings it draws, and last its description.
    const named = [
      fold(label),
      fold(group),
      ...settingsPageAliases(page.id, reader).map(fold),
    ];
    const fields = [...named, ...sections.map(fold), fold(sub)];
    // Every word, or the page does not answer. Ranked by the BEST field any
    // one word reached, so "email signature" ranks under the same rule a
    // single word would — and a page matching only half the phrase is out
    // rather than ranked low, because a reader who typed two words meant both.
    if (holdsEveryWord(words, fields)) {
      const rank = Math.min(...words.map((word) => rankOf(word, fields)));
      const section = namedSection(
        words,
        sections,
        Math.floor(rank / 2) - named.length,
      );
      scored.push({ hit: { page, label, group, section }, rank });
    } else if (page.id === CONFIGURATION_PAGE) {
      const setting = configurationFilterFor(query, settings);
      if (setting !== undefined) {
        // Below every page whose own words matched: finite, so the sort can
        // subtract it.
        const { filter, named } = setting;
        scored.push({
          hit: { page, label, group, section: named, filter },
          rank: Number.MAX_SAFE_INTEGER,
        });
      }
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

// The setting a hit through the sections alone names: the one holding every
// word, else the one the best word reached.
function namedSection(
  words: readonly string[],
  sections: readonly string[],
  best: number,
): string | undefined {
  if (best < 0 || best >= sections.length) {
    return undefined;
  }
  const whole = sections.find((section) => holdsEveryWord(words, [section]));
  return whole ?? sections[best];
}

/**
 * How well the needle matches, lower being better, or Infinity for no match.
 *
 * A prefix beats a contained substring at the same field, so typing "au" puts
 * Audit log above the pages that merely contain those letters. Beyond that the
 * FIELD decides, which is what the caller's ordering above encodes.
 */
function rankOf(needle: string, fields: readonly string[]): number {
  for (const [index, field] of fields.entries()) {
    if (field.startsWith(needle)) {
      return index * 2;
    }
    if (field.includes(needle)) {
      return index * 2 + 1;
    }
  }
  return Number.POSITIVE_INFINITY;
}
