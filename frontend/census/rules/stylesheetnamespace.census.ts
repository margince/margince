// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One stylesheet per class namespace. Two sheets that declare one class
// collide at equal specificity, and the winner is whichever the bundler
// injected last: an order no source file states.

import { withoutComments } from "../../src/testing/css";
import type { CensusRule } from "../tree";

export const namespaces = [
  { prefix: "askai-", home: "features/ai/ai.css" },
  { prefix: "auth-", home: "components/auth-core.css" },
  { prefix: "book-", home: "features/booking/book.css" },
  { prefix: "clientsurface-", home: "features/settings/client.css" },
  {
    prefix: "companydeepread-",
    home: "features/companies/companydeepread.css",
  },
  { prefix: "corrections-", home: "features/contacts/contactcorrections.css" },
  { prefix: "create-", home: "components/create.css" },
  {
    prefix: "datefield-",
    home: "features/automations/automations.datefield.css",
  },
  { prefix: "errorboundary-", home: "app/errorboundary.css" },
  { prefix: "historyfields-", home: "features/records/historyfields.css" },
  { prefix: "leadsignals-", home: "features/leads/leadsignals.css" },
  { prefix: "library-", home: "features/voice-knowledge/library.css" },
  { prefix: "listquery-", home: "components/listquery.css" },
  { prefix: "oauthconsent-", home: "features/automations/oauthconsent.css" },
  { prefix: "offers-", home: "features/deals/offers.css" },
  {
    prefix: "partnercommissions-",
    home: "features/projects/partnercommissions.css",
  },
  { prefix: "querygate-", home: "design-system/querystates.css" },
  {
    prefix: "relationshiprows-",
    home: "components/relationshiprows.css",
  },
  {
    prefix: "repeatablerowsfield-",
    home: "components/repeatablerowsfield.css",
  },
  { prefix: "scheduledsends-", home: "features/mail/scheduledsends.css" },
  { prefix: "storyhost-", home: "mcp-apps/story-hosts.css" },
  { prefix: "transcript-", home: "design-system/transcriptread.css" },
];

const homeSheet = (home: string) => `src/${home}`;

/** Every declaration of another sheet's namespace in one stylesheet. */
export function strayDeclarations(rel: string, text: string): string[] {
  // Selectors only: a class cited in a comment is a cross-reference.
  const declarations = withoutComments(text);
  return namespaces.flatMap(({ prefix, home }) =>
    rel === homeSheet(home)
      ? []
      : [...declarations.matchAll(new RegExp(`\\.${prefix}[\\w-]+`, "g"))].map(
          ([selector]) =>
            `${rel}: declares ${selector}, and the ${prefix}* namespace belongs to ${home}`,
        ),
  );
}

export const rule: CensusRule = {
  name: "stylesheetnamespace",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        file.rel.endsWith(".css"),
    ),
  floors: (corpus) =>
    namespaces
      .filter(
        ({ home }) => !corpus.some((file) => file.rel === homeSheet(home)),
      )
      .map(({ home }) => `the home sheet ${homeSheet(home)} was not read`),
  visit: (file) => strayDeclarations(file.rel, file.text),
  advice:
    "Declare a screen's classes only in its home sheet. Move the rule there, " +
    "or give the other sheet's element a class of its own namespace.",
};
