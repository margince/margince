// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// An empty text field has nothing in it to say it takes typing; its boundary
// does that alone. So WCAG 1.4.11 asks 3:1 of it, and --borderControl holds the
// ratio (tokens.test.ts). The fields are read from the tree, not listed.

import ts from "typescript";
import {
  alternativesOf,
  classesOf,
  compounds,
  declaredValues,
  type Rule,
  splitTopLevel,
  subjectOf,
} from "../../scripts/lib/css-rules";
import { typeValues } from "../../scripts/lib/input-type";
import { classVariants } from "../../src/testing/classnames";
import {
  type CensusFile,
  type CensusRule,
  frontendRoot,
  isAppSheet,
  sheetRules,
} from "../tree";

// Fields that rest with no edge inside a container another rule draws. Each
// is named with why until the owner settles it.
const EDGELESS: Readonly<Record<string, string>> = {
  ".compose-sheet > .richtext": "the message body fills the composer's sheet",
  ".palette-input input": "the palette's bar is the field the dialog opens on",
  ".input-icon-flush .input":
    "the palette's bar is the field the dialog opens on",
};

const ENTRY_TAGS = new Set(["input", "textarea", "select"]);
const TOGGLE_TYPES = ["checkbox", "radio", "range", "file", "color", "hidden"];
const TOGGLE = new RegExp(`\\[type=["']?(?:${TOGGLE_TYPES.join("|")})`);
const ENTRY_ELEMENT = /^(?:input|textarea|select)(?![\w-])/;
// `:focus` and `:focus-within` light a field whenever the caret is in it.
// A rule that answers only `:focus-visible` is a control taking the keyboard.
const CARET_FOCUS = /:focus(?:-within)?(?![\w-])/;
const NEGATED = /:not\((?:[^()]|\([^()]*\))*\)/g;
// A class that says how a box is now, not which box it is.
const STATE_CLASS = /^(?:is|has)-/;
// A state or a part of the box, never its rest. `:where()` and `:is()` only
// narrow which box it is, and `:not()` is taken out before this reads.
const NOT_REST =
  /::|:(?!where\(|is\()[\w-]+|\[(?:aria-(?:invalid|disabled|readonly)|disabled|readonly)|\.(?:is|has)-/;
const SIDES = ["top", "right", "bottom", "left"].concat(
  ["block", "inline"].flatMap((axis) => [axis, `${axis}-start`, `${axis}-end`]),
);
const EDGES = ["border", ...SIDES.map((side) => `border-${side}`)].flatMap(
  (property) => [property, `${property}-color`],
);
const CONTROL = "var(--borderControl)";
const DRAWN = /^\d+(?:\.\d+)?px\s+(?:solid|dashed|dotted)\s+(.+)$/;
const RING =
  /^inset\s+0(?:px)?\s+0(?:px)?\s+0(?:px)?\s+\d+(?:\.\d+)?px\s+(.+)$/;

const relative = (rule: Rule) => rule.file.slice(frontendRoot.length + 1);
const edgesOf = (rule: Rule) =>
  EDGES.flatMap((property) => declaredValues(rule.body, property));
const noEdge = (value: string) =>
  /^(?:0(?:px)?|none|transparent)$/.test(value) ||
  DRAWN.exec(value)?.[1] === "transparent";
const ownClasses = (compound: string) =>
  [...classesOf(compound)].filter((name) => !STATE_CLASS.test(name));

function isEntry(element: ts.JsxOpeningLikeElement): boolean {
  const said = (name: string) =>
    element.attributes.properties.find(
      (property): property is ts.JsxAttribute =>
        ts.isJsxAttribute(property) && property.name.getText() === name,
    );
  const literal = (name: string) => {
    const value = said(name)?.initializer;
    return value && ts.isStringLiteral(value) ? value.text : undefined;
  };
  if (literal("aria-readonly") === "true" || said("readOnly")) return false;
  const editable = said("contentEditable");
  const off = editable?.initializer?.getText().replace(/[{}"\s]/g, "");
  if ((editable && off !== "false") || literal("role") === "textbox") {
    return true;
  }
  const types = typeValues(element.attributes);
  return (
    ENTRY_TAGS.has(element.tagName.getText()) &&
    (types.length === 0 || types.some((type) => !TOGGLE_TYPES.includes(type)))
  );
}

// A class list can branch a lot; past this the reader throws and the census fails.
const BRANCHES = 256;

// Classes written on a box that takes typing, and on no other element: such a
// class names a field wherever a sheet draws it.
function entryClasses(modules: readonly CensusFile[]): Set<string> {
  const onEntry = new Set<string>();
  const elsewhere = new Set<string>();
  const visit = (source: ts.SourceFile, node: ts.Node) => {
    if (ts.isJsxAttribute(node) && node.name.getText() === "className") {
      const element = node.parent.parent;
      const value = node.initializer;
      const read =
        value && ts.isJsxExpression(value) ? value.expression : value;
      const entry =
        (ts.isJsxOpeningElement(element) ||
          ts.isJsxSelfClosingElement(element)) &&
        isEntry(element);
      for (const name of classVariants(source, read, BRANCHES).flat()) {
        if (!name.includes("*")) (entry ? onEntry : elsewhere).add(name);
      }
    }
    ts.forEachChild(node, (child) => visit(source, child));
  };
  for (const module of modules) {
    if (module.source) visit(module.source, module.source);
  }
  return new Set([...onEntry].filter((name) => !elsewhere.has(name)));
}

// A box whose hover and focus-within draw one edge is a tile the pointer
// lights. A field's focus edge shows where the caret is.
const litLikeHover = (rule: Rule, subject: string, rules: readonly Rule[]) => {
  const hovered = subject.replace(CARET_FOCUS, ":hover");
  return rules.some(
    (other) =>
      other.file === rule.file &&
      other.body === rule.body &&
      subjectOf(other.selector) === hovered,
  );
};

const subjectsOf = (rule: Rule) => alternativesOf(rule.selector).map(subjectOf);

// The classes a field carries. One is written only on a field, or it names a
// box that recolours its edge where the caret is. A class compounded with a
// field class joins them, round after round, until none does.
function fieldClasses(rules: readonly Rule[], entry: ReadonlySet<string>) {
  const fields = new Set(entry);
  for (const rule of rules) {
    for (const subject of subjectsOf(rule)) {
      const caret = CARET_FOCUS.test(subject.replace(NEGATED, ""));
      if (
        caret &&
        edgesOf(rule).length > 0 &&
        !litLikeHover(rule, subject, rules)
      ) {
        for (const name of ownClasses(subject)) fields.add(name);
      }
    }
  }
  let grew = true;
  while (grew) {
    grew = false;
    for (const subject of rules.flatMap(subjectsOf)) {
      const classes = ownClasses(subject);
      if (!classes.some((name) => fields.has(name))) continue;
      for (const name of classes) {
        grew ||= !fields.has(name);
        fields.add(name);
      }
    }
  }
  return fields;
}

// What an edge declaration draws in, or undefined when it draws nothing.
function inkOf(property: string, value: string): string | undefined {
  if (property.endsWith("-color")) return value;
  return DRAWN.exec(value)?.[1] ?? value;
}

// Inset rings and drawn outlines are edges too; a shadow token is depth.
const ringsOf = (rule: Rule) => [
  ...declaredValues(rule.body, "box-shadow").flatMap((value) =>
    splitTopLevel(value, ",").flatMap((layer) => RING.exec(layer)?.[1] ?? []),
  ),
  ...declaredValues(rule.body, "outline").flatMap((value) => {
    const ink = DRAWN.exec(value)?.[1];
    return ink && ink !== "transparent" ? [ink] : [];
  }),
];

function findingsOf(
  rule: Rule,
  alternative: string,
  framing: ReadonlySet<string>,
): string[] {
  const where = `${relative(rule)}: ${alternative}`;
  const shelled = compounds(alternative)
    .slice(0, -1)
    .some((compound) => ownClasses(compound).some((name) => framing.has(name)));
  const edges = EDGES.flatMap((property) =>
    declaredValues(rule.body, property).map((value) => ({ property, value })),
  );
  return [
    ...edges.flatMap(({ property, value }) => {
      if (noEdge(value)) {
        return shelled || alternative in EDGELESS
          ? []
          : [
              `${where} draws no edge of its own and names no field or ruled row around it`,
            ];
      }
      return inkOf(property, value) === CONTROL
        ? []
        : [`${where} draws ${value}`];
    }),
    ...ringsOf(rule)
      .filter((ink) => ink !== CONTROL)
      .map((ink) => `${where} rings itself in ${ink}`),
  ];
}

// Classes of a box whose rest rule draws an edge in --borderControl: a bare
// field on such a row has its edge drawn for it.
const controlRows = (rules: readonly Rule[]) =>
  new Set(
    rules.flatMap((rule) =>
      EDGES.some((property) =>
        declaredValues(rule.body, property).some(
          (value) => inkOf(property, value) === CONTROL,
        ),
      )
        ? subjectsOf(rule)
            .filter((subject) => !NOT_REST.test(subject.replace(NEGATED, "")))
            .flatMap(ownClasses)
        : [],
    ),
  );

// A field's rest rule that draws its edge in anything but --borderControl.
function weakEdges(rules: readonly Rule[], fields: ReadonlySet<string>) {
  const framing = new Set([...fields, ...controlRows(rules)]);
  return rules.flatMap((rule) =>
    alternativesOf(rule.selector).flatMap((alternative) => {
      const subject = subjectOf(alternative);
      const field =
        (ENTRY_ELEMENT.test(subject) && !TOGGLE.test(subject)) ||
        ownClasses(subject).some((name) => fields.has(name));
      if (!field || NOT_REST.test(subject.replace(NEGATED, ""))) return [];
      return findingsOf(rule, alternative, framing);
    }),
  );
}

const isAppModule = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".tsx") &&
  !/\.(?:test|stories)\.tsx$/.test(file.path);

const fieldsOf = (corpus: readonly CensusFile[]) => {
  const rules = sheetRules(corpus.filter(isAppSheet));
  return {
    rules,
    fields: fieldClasses(rules, entryClasses(corpus.filter(isAppModule))),
  };
};

export const rule: CensusRule = {
  name: "fieldedge",
  corpus: (tree) =>
    tree.files.filter((file) => isAppSheet(file) || isAppModule(file)),
  // `token-box` is written on an <input> and no sheet focuses it, so only the
  // markup reading can find it.
  floors: (corpus) => {
    const { fields } = fieldsOf(corpus);
    return ["input", "textarea", "field-shell", "lt-fsearch-input", "token-box"]
      .filter((name) => !fields.has(name))
      .map((name) => `.${name} no longer reads as a field`);
  },
  across: (corpus) => {
    const { rules, fields } = fieldsOf(corpus);
    return weakEdges(rules, fields);
  },
  advice:
    "draw a text field's resting edge in var(--borderControl), the token that holds 1.4.11's 3:1; a field with no edge sits inside a field or a row ruled in that token",
};
