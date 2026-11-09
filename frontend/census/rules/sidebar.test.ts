// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedSettled, srcDir, storybookDir } from "../tree";
import {
  caseFindings,
  emptyRootFindings,
  introductionRoots,
  isSentenceCase,
  leafAndGroupFindings,
  rule,
  sharedTitleFindings,
  shelfFindings,
  sidebarOrder,
} from "./sidebar.census";

// An absence arm passes the same over a clean tree and a dead detector, so
// these plant each defect and read the detector directly.
describe("the sidebar detectors report what they are for", () => {
  const probe = join(srcDir, "design-system", "probe.tsx");
  const script = join(srcDir, "design-system", "probe.ts");

  it("sees a documented root with no story, or no docs page", () => {
    expect(
      emptyRootFindings(
        [
          { path: "a.mdx", title: "Get started/Introduction" },
          { path: "b.stories.tsx", title: "Shell/Top bar" },
        ],
        ["Get started", "Shell", "Records"],
      ),
    ).toEqual(["Records/ is documented and holds no story"]);
    expect(
      emptyRootFindings(
        [
          { path: "a.stories.tsx", title: "Get started/Introduction" },
          { path: "b.mdx", title: "Shell/Overview" },
        ],
        ["Get started", "Shell"],
      ),
    ).toEqual([
      "Get started/ is documented and holds no docs page",
      "Shell/ is documented and holds no story",
    ]);
  });

  it("reads the introduction's roots off its list, and nothing else", () => {
    const mdx = [
      "The **Theme** control is above.",
      "## Where things are",
      "",
      "- **Get started**: this page.",
      "- **Onboarding** and **Signed out**: the first run.",
      "  a wrapped line with **Not a root**",
      "",
      "## Checking a story",
      "- **Phone** at 390px.",
    ].join("\n");
    expect(introductionRoots(mdx)).toEqual([
      "Get started",
      "Onboarding",
      "Signed out",
    ]);
    expect(introductionRoots("- **Shell**: the frame.")).toEqual([]);
  });

  it("sees a title off the declared shelves, and a shelf with no story", () => {
    const shelves = ["Labels", "Messaging", "Loading"];
    expect(
      shelfFindings(
        [
          { path: "a", title: "Components/Labels/Tag pill" },
          { path: "b", title: "Components/Messaging/Toast" },
          { path: "c", title: "Components/Badges/Badge" },
          { path: "d", title: "Components/Labels" },
          { path: "e", title: "Patterns/Anything" },
          { path: "f", title: "Components/Labels/Tag pill/Row" },
          { path: "g", title: "Components/Labels/Tag pill/Row/Cell" },
        ],
        "Components",
        shelves,
        [3, 4],
      ),
    ).toEqual([
      'c files "Components/Badges/Badge" off the Components/ shelves the README declares',
      'd files "Components/Labels" off the Components/ shelves the README declares',
      'g files "Components/Labels/Tag pill/Row/Cell" off the Components/ shelves the README declares',
      "Components/Loading is declared and holds no story",
    ]);
  });

  it("sees a segment that is not Sentence case", () => {
    for (const segment of [
      "ListTable",
      "Stat Card",
      "stat card",
      "Stat  card",
      " Stat card",
      "",
      "ALERT",
      "Stat CARD",
      "1ALERT",
      "360View",
      "SMTP relay",
      "LDAP server",
      "Stat Äpfel",
      "ärger",
      "Ai pending",
      "Open in margince",
      "Ai-drafted reply",
      "Sign-In page",
      "Linkedin import",
      "Oauth return",
      "VCard import",
      "Vat mark",
    ]) {
      expect(isSentenceCase(segment)).toBe(false);
    }
    for (const segment of [
      "Stat card",
      "AI pending",
      "Company 360",
      "Open in Margince",
      "Sign-in page",
      "Ärger",
      "AI-drafted reply",
      "Margince’s core",
      "Margince's core",
      "IMAP connect form",
      "LinkedIn import",
      "OAuth return",
      "vCard import",
      "Voice DNA",
      "Add to Shortlist",
      "VAT mark",
    ]) {
      expect(isSentenceCase(segment)).toBe(true);
    }
    expect(
      caseFindings([
        { path: "a", title: "Components/Text and Data/List table" },
      ]),
    ).toEqual(['a: "Text and Data" is not Sentence case']);
  });

  it("holds a declared phrase to its spelling, whole or inside a segment", () => {
    for (const segment of ["MCP Apps", "Open in MCP Apps", "MCP Apps bridge"]) {
      expect(isSentenceCase(segment)).toBe(true);
    }
    for (const segment of [
      "Mcp Apps",
      "MCP apps x",
      "Open in MCP apps",
      "MCP Apps Bridge",
    ]) {
      expect(isSentenceCase(segment)).toBe(false);
    }
  });

  it("sees two files claiming one title, as Storybook's id folds it", () => {
    expect(
      sharedTitleFindings([
        { path: "a", title: "Components/Labels/Tag pill" },
        { path: "b", title: "Components/Labels/Row tags" },
        { path: "c", title: "Components/Labels/Tag/Pill" },
        { path: "d", title: "Components/Labels/AI tag" },
        { path: "e", title: "Components/Labels/Ai tag" },
      ]),
    ).toEqual([
      'id "components-labels-tag-pill" is claimed by a ("Components/Labels/Tag pill"), c ("Components/Labels/Tag/Pill")',
      'id "components-labels-ai-tag" is claimed by d ("Components/Labels/AI tag"), e ("Components/Labels/Ai tag")',
    ]);
  });

  it("sees a title that is both a leaf and a group", () => {
    expect(
      leafAndGroupFindings([
        { path: "a", title: "Components/Labels/Tag" },
        { path: "b", title: "Components/Labels/Tag/Row" },
        { path: "c", title: "Components/Labels/Tag pill" },
        { path: "d", title: "Components/Labels/AI mark" },
        { path: "e", title: "Components/Labels/Ai mark/Row" },
        { path: "f", title: "Components/Labels/Chip/Pill" },
        { path: "g", title: "Components/Labels/Chip pill/Row" },
      ]),
    ).toEqual([
      'a: "Components/Labels/Tag" is a leaf and also the group of "Components/Labels/Tag/Row"',
      'd: "Components/Labels/AI mark" is a leaf and also the group of "Components/Labels/Ai mark/Row"',
      'f: "Components/Labels/Chip/Pill" is a leaf and also the group of "Components/Labels/Chip pill/Row"',
    ]);
  });

  it("reads the sidebar order off the default-exported preview", () => {
    const preview = [
      'const fixture = { storySort: { order: ["Wrong"] } };',
      "const preview: Preview = { parameters: { options: { storySort: {",
      '  method: "alphabetical",',
      '  order: ["Foundations", ["Color"], "Components", ["Labels", "Messaging"], "Shell"],',
      "} } } };",
      "export default preview;",
    ].join("\n");
    const read = sidebarOrder(probe, preview);
    expect(read?.roots).toEqual(["Foundations", "Components", "Shell"]);
    expect(read?.children.get("Components")).toEqual(["Labels", "Messaging"]);
    expect(read?.children.get("Foundations")).toEqual(["Color"]);
  });

  it("reports no sidebar order rather than guessing one", () => {
    const preview = (order: string) =>
      `export default { parameters: { options: { storySort: { order: ${order} } } } };`;
    for (const order of [
      "ROOTS",
      "[...ROOTS]",
      '[["Color"], "Foundations"]',
      '["Components", [LABELS]]',
      '["Components", ["Labels"], ["Messaging"]]',
    ]) {
      expect(sidebarOrder(probe, preview(order))).toBe(null);
    }
    expect(
      sidebarOrder(probe, 'const p = { storySort: { order: ["Shell"] } };'),
    ).toBe(null);
    expect(sidebarOrder(probe, preview('["Shell"]'))).toEqual({
      roots: ["Shell"],
      children: new Map(),
    });
    expect(
      sidebarOrder(
        script,
        'export default <P>{ parameters: { options: { storySort: { order: ["Shell"] } } } };',
      )?.roots,
    ).toEqual(["Shell"]);
  });
});

describe("the sidebar rule", () => {
  it("reports a planted misfiled story through its own corpus and settle", async () => {
    const story = (name: string, title: string) => ({
      path: join(srcDir, "design-system", `${name}.stories.tsx`),
      area: "src" as const,
      text: `export default { title: "${title}" };\n`,
    });
    const findings = await plantedSettled(rule, [
      story("badge", "Components/Badges/Badge"),
      story("pill", "Components/Labels/Tag Pill"),
      {
        path: join(storybookDir, "preview.tsx"),
        area: "storybook",
        text: 'export default { parameters: { options: { storySort: { order: ["Shell"] } } } };\n',
      },
    ]);
    expect(findings).toContain(
      'src/design-system/badge.stories.tsx files "Components/Badges/Badge" off the Components/ shelves the README declares',
    );
    expect(findings).toContain(
      'src/design-system/pill.stories.tsx: "Tag Pill" is not Sentence case',
    );
    expect(findings).toContain("the preview sorts the roots Shell");
  });
});
