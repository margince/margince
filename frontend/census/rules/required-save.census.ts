// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A form with a required field keeps its submit enabled. Pressing it puts
// "{Field} is required." on each empty one and focuses the first
// (`useFieldFaults` in src/design-system/savepage.tsx). A submit greyed out
// while a field is empty says nothing about why.
//
// The rule finds a `confirmDisabled`, or a `disabled` on a `Button`, whose
// expression tests a value for emptiness, directly or through one const of the
// same file. A button that gives its reason (`reason`, `reasonId`,
// `confirmReason`) says why, so it is spared.

import ts from "typescript";
import type { CensusFile, CensusRule } from "../tree";

const EMPTINESS =
  /\.trim\(\)|[!=]==\s*""|""\s*[!=]==|\.length\s*(===|>|<)\s*[01]\b/;
const EXPLAINED = new Set(["reason", "reasonId", "confirmReason"]);

function attributeNamed(
  element: ts.JsxOpeningLikeElement,
  name: string,
): ts.JsxAttribute | undefined {
  return element.attributes.properties.find(
    (property): property is ts.JsxAttribute =>
      ts.isJsxAttribute(property) && property.name.getText() === name,
  );
}

// A call or a function makes its own value; only a plain expression is a
// test the reader could have written inline.
function producesValue(initializer: ts.Expression): boolean {
  return (
    ts.isCallExpression(initializer) ||
    ts.isArrowFunction(initializer) ||
    ts.isFunctionExpression(initializer) ||
    ts.isObjectLiteralExpression(initializer)
  );
}

// Each const's initializer text by name, so `disabled={!ready}` is judged by
// what `ready` tests.
function constInitializers(source: ts.SourceFile): Map<string, string> {
  const consts = new Map<string, string>();
  const visit = (node: ts.Node) => {
    if (
      ts.isVariableDeclaration(node) &&
      ts.isIdentifier(node.name) &&
      node.initializer &&
      !producesValue(node.initializer)
    ) {
      consts.set(node.name.text, node.initializer.getText(source));
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return consts;
}

function testsEmptiness(
  expression: ts.Expression,
  consts: ReadonlyMap<string, string>,
): boolean {
  if (EMPTINESS.test(expression.getText())) return true;
  let named = false;
  const visit = (node: ts.Node) => {
    const property =
      ts.isPropertyAccessExpression(node.parent) && node.parent.name === node;
    if (
      ts.isIdentifier(node) &&
      !property &&
      EMPTINESS.test(consts.get(node.text) ?? "")
    ) {
      named = true;
    }
    if (!named) ts.forEachChild(node, visit);
  };
  visit(expression);
  return named;
}

function heldSubmit(
  element: ts.JsxOpeningLikeElement,
): ts.Expression | undefined {
  for (const name of EXPLAINED) {
    if (attributeNamed(element, name)) return undefined;
  }
  const tag = element.tagName.getText();
  const held =
    attributeNamed(element, "confirmDisabled") ??
    (tag === "Button" ? attributeNamed(element, "disabled") : undefined);
  const initializer = held?.initializer;
  return initializer &&
    ts.isJsxExpression(initializer) &&
    initializer.expression
    ? initializer.expression
    : undefined;
}

/** Every line in `source` that greys out a submit on an empty value. */
export function heldSubmitsIn(source: ts.SourceFile): number[] {
  const consts = constInitializers(source);
  const lines: number[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
      const held = heldSubmit(node);
      if (held && testsEmptiness(held, consts)) {
        lines.push(
          source.getLineAndCharacterOfPosition(held.getStart(source)).line + 1,
        );
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return lines;
}

// A held submit that is not an empty required field, each with its reason.
// The list only shrinks: a waived file that no longer holds one fails.
const WAIVED: ReadonlyMap<string, string> = new Map([
  [
    "../extensions/openchannel/frontend/endpointcard.tsx",
    "An extension; the extension surface offers no useFieldFaults yet.",
  ],
  [
    "src/components/tagfollow.tsx",
    "A count of ticked contacts; the button acts on the selection.",
  ],
  [
    "src/design-system/resolvesheet.tsx",
    "`canSubmit` holds the save for a caller state, such as data still loading.",
  ],
  [
    "src/features/ai/ai-serving-editor.tsx",
    "Format tidies typed JSON; it submits no form.",
  ],
  [
    "src/features/analytics/reporting.setupproposal.tsx",
    "An AI suggestion step; the box it reads belongs to the outer form.",
  ],
  [
    "src/features/automations/agentallowances.form.tsx",
    "Save waits for a change; it counts changed settings, not an empty field.",
  ],
  [
    "src/features/booking/booking-alias.tsx",
    "The alias is optional; an empty or unchanged box has nothing to save.",
  ],
  [
    "src/features/capture/emailaccesseditor.tsx",
    "An unchanged audience or no picked members; a selection, not a field.",
  ],
  [
    "src/features/capture/integrations-provider-credential.tsx",
    "A typed confirmation phrase before a destructive act.",
  ],
  [
    "src/features/companies/companycontacts/domaincontacts.tsx",
    "A count of ticked contacts to link; a selection, not a field.",
  ],
  [
    "src/features/contacts/contactdrawers.tsx",
    "A count of research findings chosen to keep; a selection, not a field.",
  ],
  [
    "src/features/deals/deal360/deallines.tsx",
    "An optional add row inside the dialog; it adds only what was typed.",
  ],
  [
    "src/features/deals/dealroomthread.tsx",
    "A comment composer with no label; the box is the whole form.",
  ],
  [
    "src/features/import/linkedin-import.tsx",
    "Holds Save while the address is unchanged.",
  ],
  [
    "src/features/leads/leadbulk.tsx",
    "A bulk bar that needs selected leads and an owner pick.",
  ],
  [
    "src/features/leads/leadsignals.tsx",
    "Holds Save until a question is answered, a selection count; the note explains on press.",
  ],
  [
    "src/features/lists/filterpropose.tsx",
    "A prompt box for the AI; the box is the whole form.",
  ],
  [
    "src/features/onboarding/onboarding-conversation/connect-also.tsx",
    "An onboarding conversation step that waits for the link it asks for.",
  ],
  [
    "src/features/onboarding/onboarding-conversation/decision-scene.tsx",
    "An onboarding conversation scene; the answer is a pick among cards.",
  ],
  [
    "src/features/onboarding/onboarding-conversation/voice-scenes.tsx",
    "An onboarding conversation scene; the paste box adds text to the chat.",
  ],
  [
    "src/features/records/customfields.choices.tsx",
    "Holds Save while no choice is kept, a selection count.",
  ],
  [
    "src/features/records/timelineactions.tsx",
    "Holds the change while the audience is unchanged or no member is picked.",
  ],
  [
    "src/features/settings/lossreasons.tsx",
    "Apply acts on the reasons picked in the list.",
  ],
  [
    "src/features/voice-knowledge/corpusask.tsx",
    "A question box for an AI answer; the box is the whole form.",
  ],
  [
    "src/features/voice-knowledge/knowledge.documents.tsx",
    "A count of dropped files; the drop zone says none are chosen.",
  ],
  [
    "src/features/worklist/brief.teamplan.tsx",
    "A reply box on a commitment; empty or unchanged text has nothing to save.",
  ],
  [
    "src/features/worklist/worklist.manager.tsx",
    "An inline hand-over row of one picker and its verb.",
  ],
]);

const judged = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.source !== undefined &&
  file.rel.endsWith(".tsx") &&
  !/\.(test|stories)\.tsx$/.test(file.rel);

export const rule: CensusRule = {
  name: "required-save",
  corpus: (tree) => tree.files.filter(judged),
  floors: (corpus) => [
    ...(corpus.length > 200 ? [] : ["fewer than 200 modules were read"]),
    ...[...WAIVED.keys()]
      .filter((rel) => !corpus.some((file) => file.rel === rel))
      .map((rel) => `${rel} is waived but no longer exists`),
  ],
  visit: (file) => {
    const lines = file.source ? heldSubmitsIn(file.source) : [];
    if (WAIVED.has(file.rel)) {
      return lines.length > 0
        ? []
        : [`${file.rel}: waived but holds no submit; remove its waiver`];
    }
    return lines.map((line) => `${file.rel}:${line}`);
  },
  advice:
    "Keep the submit enabled. On press, put each empty required field's " +
    "sentence on it with `useFieldFaults().show(blankRequired(...))` " +
    "(src/design-system/savepage.tsx, src/lib/faultsentence.ts), which also " +
    "focuses the first. A button that must stay off says why with `reason`.",
};
