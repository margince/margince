// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  elementsIn,
  handedTo,
  markupFiles,
  opening,
  srcDir,
} from "../../scripts/lib/dialoglayout";
import { parseSource, sourceFileAt } from "../../scripts/lib/source-tree";

// Each file's legacy `size` / `placement` and intent-less `Modal`s may only
// fall; stories count (readers copy them), tests do not (they pin the mapping).

/** Per file: legacy props handed to a dialog, and `Modal`s naming no box. */
const BASELINE: Record<string, { legacy: number; bare: number }> = {
  "app/shell.tsx": { legacy: 0, bare: 1 },
  "design-system/confirmmodal.tsx": { legacy: 2, bare: 0 },
  "design-system/emaildetail.tsx": { legacy: 2, bare: 0 },
  "design-system/filepreview.tsx": { legacy: 1, bare: 0 },
  "design-system/resolvesheet.tsx": { legacy: 1, bare: 0 },
  "screens/acquisitionsources.tsx": { legacy: 0, bare: 1 },
  "screens/adddocument.tsx": { legacy: 0, bare: 1 },
  "screens/addemploymentmodal.tsx": { legacy: 0, bare: 1 },
  "screens/ai-binding-editor.tsx": { legacy: 2, bare: 0 },
  "screens/ai-provider-sheet.tsx": { legacy: 2, bare: 0 },
  "screens/ai-task-outcome.stories.tsx": { legacy: 1, bare: 0 },
  "screens/ai-task-sheet.tsx": { legacy: 2, bare: 0 },
  "screens/aiexport.tsx": { legacy: 1, bare: 0 },
  "screens/analytics.explain.drawer.tsx": { legacy: 1, bare: 0 },
  "screens/analytics.sharelist.tsx": { legacy: 1, bare: 0 },
  "screens/approvaldrawer.tsx": { legacy: 2, bare: 0 },
  "screens/approvaleditor.tsx": { legacy: 0, bare: 1 },
  "screens/archive.tsx": { legacy: 0, bare: 1 },
  "screens/automations.tsx": { legacy: 0, bare: 2 },
  "screens/billingcontactmodal.tsx": { legacy: 0, bare: 1 },
  "screens/blocked-domains-decision.tsx": { legacy: 0, bare: 1 },
  "screens/brief.queue.tsx": { legacy: 2, bare: 0 },
  "screens/brief.teamplan.tsx": { legacy: 0, bare: 1 },
  "screens/brief.tsx": { legacy: 1, bare: 0 },
  "screens/bulkchange.tsx": { legacy: 0, bare: 1 },
  "screens/capture-activity-drawer.tsx": { legacy: 1, bare: 0 },
  "screens/capture-exclusions.tsx": { legacy: 0, bare: 1 },
  "screens/capture-owner-identities.tsx": { legacy: 0, bare: 1 },
  "screens/capture-purge-dialog.tsx": { legacy: 0, bare: 1 },
  "screens/commissiondecide.tsx": { legacy: 0, bare: 1 },
  "screens/companies.tsx": { legacy: 1, bare: 0 },
  "screens/company-context.tsx": { legacy: 1, bare: 0 },
  "screens/companyapprovals.tsx": { legacy: 1, bare: 0 },
  "screens/companycontacts/introrequest.tsx": { legacy: 0, bare: 1 },
  "screens/companyevidence.tsx": { legacy: 1, bare: 0 },
  "screens/companyreject.tsx": { legacy: 0, bare: 1 },
  "screens/compose.tsx": { legacy: 2, bare: 0 },
  "screens/composeschedule.tsx": { legacy: 1, bare: 0 },
  "screens/connectors.tsx": { legacy: 0, bare: 1 },
  "screens/consumer-mail-domains.tsx": { legacy: 0, bare: 1 },
  "screens/contactconsentpanel.tsx": { legacy: 1, bare: 0 },
  "screens/contactdrawers.tsx": { legacy: 2, bare: 0 },
  "screens/contactpage.tsx": { legacy: 1, bare: 0 },
  "screens/contractform.tsx": { legacy: 0, bare: 1 },
  "screens/contractlifecycle.tsx": { legacy: 0, bare: 3 },
  "screens/contracttermsfields.stories.tsx": { legacy: 0, bare: 1 },
  "screens/corpusask.tsx": { legacy: 2, bare: 0 },
  "screens/coverageexplorer.tsx": { legacy: 1, bare: 0 },
  "screens/create.tsx": { legacy: 0, bare: 1 },
  "screens/customfields.tsx": { legacy: 1, bare: 1 },
  "screens/deal360/confirmadvance.tsx": { legacy: 0, bare: 1 },
  "screens/deal360/dealactions.tsx": { legacy: 0, bare: 1 },
  "screens/deal360/outcomereviewmodal.tsx": { legacy: 0, bare: 1 },
  "screens/dealroomaccess.tsx": { legacy: 1, bare: 0 },
  "screens/dealsuggestion.tsx": { legacy: 0, bare: 1 },
  "screens/directsendmodal.tsx": { legacy: 1, bare: 0 },
  "screens/edit.tsx": { legacy: 0, bare: 1 },
  "screens/employmentedit.tsx": { legacy: 0, bare: 1 },
  "screens/employmentimport.tsx": { legacy: 0, bare: 1 },
  "screens/imap-connect-form.tsx": { legacy: 0, bare: 1 },
  "screens/import.tsx": { legacy: 1, bare: 0 },
  "screens/installation-settings.tsx": { legacy: 0, bare: 1 },
  "screens/introdecision.tsx": { legacy: 2, bare: 0 },
  "screens/introdrawer.tsx": { legacy: 2, bare: 0 },
  "screens/leads.qualify.tsx": { legacy: 1, bare: 0 },
  "screens/leadvocab.tsx": { legacy: 0, bare: 1 },
  "screens/linkedin-import.tsx": { legacy: 0, bare: 1 },
  "screens/logactivity.tsx": { legacy: 0, bare: 1 },
  "screens/magic.records.tsx": { legacy: 1, bare: 0 },
  "screens/meetingbrief/drawer.tsx": { legacy: 2, bare: 0 },
  "screens/merge.tsx": { legacy: 0, bare: 1 },
  "screens/offers.tsx": { legacy: 0, bare: 1 },
  "screens/onboarding-conversation/connect-dialog.tsx": { legacy: 0, bare: 1 },
  "screens/own-domains.tsx": { legacy: 0, bare: 1 },
  "screens/passwordcard.tsx": { legacy: 0, bare: 1 },
  "screens/privacy.tsx": { legacy: 0, bare: 2 },
  "screens/projecthealthmodal.tsx": { legacy: 0, bare: 1 },
  "screens/rate-manual.tsx": { legacy: 0, bare: 1 },
  "screens/rates.tsx": { legacy: 0, bare: 1 },
  "screens/recordroles.tsx": { legacy: 0, bare: 1 },
  "screens/recordteamassign.tsx": { legacy: 0, bare: 1 },
  "screens/relationshiprows.tsx": { legacy: 0, bare: 1 },
  "screens/relationships.tsx": { legacy: 0, bare: 1 },
  "screens/reporting.comparison.tsx": { legacy: 1, bare: 0 },
  "screens/reporting.evidence.tsx": { legacy: 1, bare: 0 },
  "screens/reporting.save.tsx": { legacy: 1, bare: 0 },
  "screens/reporting.schedule.tsx": { legacy: 0, bare: 1 },
  "screens/reporting.targetdialog.tsx": { legacy: 0, bare: 1 },
  "screens/retention.tsx": { legacy: 0, bare: 2 },
  "screens/retentionpolicyform.stories.tsx": { legacy: 0, bare: 1 },
  "screens/reviewtemplateeditor.tsx": { legacy: 0, bare: 1 },
  "screens/roles-settings.tsx": { legacy: 0, bare: 1 },
  "screens/savedviews.manage.tsx": { legacy: 0, bare: 1 },
  "screens/settings.tsx": { legacy: 1, bare: 1 },
  "screens/tagadmin.tsx": { legacy: 0, bare: 1 },
  "screens/tagpicker.tsx": { legacy: 0, bare: 1 },
  "screens/taskactions.tsx": { legacy: 1, bare: 1 },
  "screens/telegram-connect-form.tsx": { legacy: 0, bare: 1 },
  "screens/users-access.tsx": { legacy: 0, bare: 1 },
  "screens/users-admin.tsx": { legacy: 0, bare: 1 },
  "screens/users-password-link.tsx": { legacy: 1, bare: 0 },
  "screens/vcard-import.tsx": { legacy: 0, bare: 1 },
  "screens/webhooks.tsx": { legacy: 0, bare: 1 },
  "screens/worklist.meetingoutcome.tsx": { legacy: 0, bare: 1 },
};

const DIALOGS = new Set(["Modal", "ConfirmModal"]);
const LEGACY = ["size", "placement"];

// The names a file calls the two dialogs by: imported, renamed on import, or
// reached through a namespace import.
function dialogNames(source: ts.SourceFile) {
  const named = new Map<string, string>();
  const spaces = new Set<string>();
  for (const s of source.statements) {
    const bound = ts.isImportDeclaration(s)
      ? s.importClause?.namedBindings
      : undefined;
    if (bound && ts.isNamespaceImport(bound)) spaces.add(bound.name.text);
    for (const el of bound && ts.isNamedImports(bound) ? bound.elements : []) {
      const imported = (el.propertyName ?? el.name).text;
      if (DIALOGS.has(imported)) named.set(el.name.text, imported);
    }
  }
  return { named, spaces };
}

function dialogOf(e: ts.Node, names: ReturnType<typeof dialogNames>) {
  const tag = opening(e)?.tagName;
  if (tag && ts.isIdentifier(tag)) return names.named.get(tag.text);
  const space = tag && ts.isPropertyAccessExpression(tag) ? tag : undefined;
  return space &&
    names.spaces.has(space.expression.getText()) &&
    DIALOGS.has(space.name.text)
    ? space.name.text
    : undefined;
}

// A spread the reader cannot follow may carry either prop, so it counts as both.
const handed = (e: ts.Node, prop: string) => {
  const values = handedTo(e, prop);
  return values === "any" ? 1 : values.filter(Boolean).length;
};

function censusOf(source: ts.SourceFile) {
  const names = dialogNames(source);
  const count = { dialogs: 0, legacy: 0, bare: 0 };
  for (const e of elementsIn(source)) {
    const dialog = dialogOf(e, names);
    if (!dialog) continue;
    const legacy = LEGACY.reduce((n, prop) => n + handed(e, prop), 0);
    count.dialogs += 1;
    count.legacy += legacy;
    if (dialog === "Modal" && legacy === 0 && handed(e, "intent") === 0) {
      count.bare += 1;
    }
  }
  return count;
}

// A second reading of the same tags from the text alone, so a walk that stops
// recognising one shape disagrees with it instead of counting fewer.
function tagsInText(text: string): number {
  const code = text
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/(^|\s)\/\/.*$/gm, "$1");
  const imports = [...code.matchAll(/import\s*\{([^}]*)\}/g)].flatMap((m) =>
    m[1].split(","),
  );
  const names = imports.flatMap((spec) => {
    const [imported, local = imported] = spec.trim().split(/\s+as\s+/);
    return DIALOGS.has(imported) ? [local] : [];
  });
  const spaces = [...code.matchAll(/import\s*\*\s*as\s+(\w+)/g)].map(
    (m) => `${m[1]}\\.(?:Modal|ConfirmModal)`,
  );
  const tags = [...names, ...spaces];
  if (tags.length === 0) return 0;
  const pattern = new RegExp(`(?<![\\w$.])<(?:${tags.join("|")})\\b`, "g");
  return code.match(pattern)?.length ?? 0;
}

const pathOf = (file: string) => relative(srcDir, file).replaceAll("\\", "/");
let tree: Map<string, ReturnType<typeof censusOf>> | undefined;
const census = () => {
  tree ??= new Map(
    markupFiles().map((f) => [pathOf(f), censusOf(sourceFileAt(f))]),
  );
  return tree;
};

describe("the legacy overlay props only count down", () => {
  it("finds the dialogs it is meant to judge, every one the text spells", () => {
    const walked = [...census().values()].reduce((n, c) => n + c.dialogs, 0);
    const spelled = markupFiles().reduce(
      (n, f) => n + tagsInText(readFileSync(f, "utf8")),
      0,
    );
    expect(walked).toBeGreaterThan(0);
    expect(walked).toBe(spelled);
  });

  it("holds every file at or under its frozen count", () => {
    const grown = [...census()].flatMap(([file, c]) => {
      const allowed = BASELINE[file] ?? { legacy: 0, bare: 0 };
      return c.legacy > allowed.legacy || c.bare > allowed.bare
        ? [
            `${file}: ${c.legacy} legacy / ${c.bare} bare, allowed ${allowed.legacy} / ${allowed.bare}; name an intent instead`,
          ]
        : [];
    });
    expect(grown).toEqual([]);
  });

  it("keeps the baseline to what the tree still holds", () => {
    const stale = Object.entries(BASELINE).flatMap(([file, allowed]) => {
      const c = census().get(file) ?? { legacy: 0, bare: 0 };
      return c.legacy < allowed.legacy || c.bare < allowed.bare
        ? [
            `${file}: down to ${c.legacy} legacy / ${c.bare} bare; lower its entry`,
          ]
        : [];
    });
    expect(stale).toEqual([]);
  });

  it("counts every shape a legacy prop can be handed in", () => {
    const planted = `import { Modal as Pane, ConfirmModal } from "./atoms";
import * as ds from "./atoms";
const shared = { size: "wide" };
const A = () => (
  <>
    <Pane
      open
      size="wide"
      placement="right"
    />
    <ds.Modal {...shared} />
    <ConfirmModal {...(wide ? { size: "wide" } : { intent: "form" })} />
    <ds.Modal open />
    <Pane intent="drawer" />
    <ConfirmModal open />
    {/* <Pane size="wide" /> */}
  </>
);`;
    const source = parseSource("planted.tsx", planted);
    expect(censusOf(source)).toEqual({ dialogs: 6, legacy: 5, bare: 1 });
    expect(tagsInText(planted)).toBe(6);
  });
});
