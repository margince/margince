// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { extensionLayers, filesMatching } from "../../scripts/lib/source-tree";
import { withoutComments } from "../testing/css";

// One stylesheet per class namespace.
//
// Two sheets declaring the same class collide at equal specificity, and the
// winner is whichever one the bundler injected last — an ordering no source
// file states and no import expresses. So one sheet's `margin: 4px` silently
// beats the other's `margin: 20px`, and the symptom surfaces as spacing that is
// wrong on one screen and right on its sibling.
//
// Unreadable by inspection: a duplicate declaration is not a syntax error and
// both files are correct on their own. Hence a gate over the tree rather than a
// rule someone has to remember while editing either sheet.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

const sheets = filesMatching(join(frontendRoot, "src"), /\.css$/)
  .concat(
    extensionLayers(join(frontendRoot, "..", "extensions")).flatMap((layer) =>
      filesMatching(layer, /\.css$/),
    ),
  )
  .map((file) => ({
    path: relative(frontendRoot, file).replace(/\\/g, "/"),
    file,
  }));

const namespaces = [
  { prefix: "archive-", home: "screens/archive.css" },
  { prefix: "askai-", home: "screens/ai.css" },
  { prefix: "auth-", home: "screens/auth.css" },
  { prefix: "book-", home: "screens/book.css" },
  { prefix: "clientsurface-", home: "screens/client.css" },
  { prefix: "commissiondecide-", home: "screens/commissiondecide.css" },
  { prefix: "companydeepread-", home: "screens/companydeepread.css" },
  { prefix: "companyreject-", home: "screens/companyreject.css" },
  { prefix: "corrections-", home: "screens/contactcorrections.css" },
  { prefix: "create-", home: "screens/create.css" },
  { prefix: "datefield-", home: "screens/automations.datefield.css" },
  { prefix: "dialog-", home: "screens/common.css" },
  { prefix: "errorboundary-", home: "app/errorboundary.css" },
  { prefix: "historyfields-", home: "screens/historyfields.css" },
  { prefix: "mergeaction-", home: "screens/merge.css" },
  { prefix: "oauthconsent-", home: "screens/oauthconsent.css" },
  { prefix: "offers-", home: "screens/offers.css" },
  { prefix: "partnercommissions-", home: "screens/partnercommissions.css" },
  { prefix: "querygate-", home: "screens/common.css" },
  { prefix: "relationshiprows-", home: "screens/relationshiprows.css" },
  { prefix: "relationships-", home: "screens/relationships.css" },
  { prefix: "repeatablerowsfield-", home: "screens/repeatablerowsfield.css" },
  { prefix: "scheduledsends-", home: "screens/scheduledsends.css" },
  { prefix: "storyhost-", home: "mcp-apps/story-hosts.css" },
  { prefix: "strength-", home: "screens/strength.css" },
  { prefix: "transcript-", home: "screens/transcriptread.css" },
];

const homeSheet = (home: string) => `src/${home}`;

describe("a screen's class namespace", () => {
  it("is walked from a corpus that holds every namespace's home sheet", () => {
    const paths = sheets.map(({ path }) => path);
    for (const { home } of namespaces) {
      expect(paths, home).toContain(homeSheet(home));
    }
  });

  it("is declared in exactly one stylesheet", () => {
    const violations: string[] = [];
    for (const { path, file } of sheets) {
      // Selectors only: a `.auth-shell` inside a comment is a cross-reference,
      // which is exactly how the two onboarding sheets cite this surface.
      const declarations = withoutComments(readFileSync(file, "utf8"));
      for (const { prefix, home } of namespaces) {
        if (path === homeSheet(home)) {
          continue;
        }
        for (const [selector] of declarations.matchAll(
          new RegExp(`\\.${prefix}[\\w-]+`, "g"),
        )) {
          violations.push(
            `${path}: declares ${selector} — the ${prefix}* namespace belongs to ${home}`,
          );
        }
      }
    }
    expect(violations, violations.join("\n")).toEqual([]);
  });
});
