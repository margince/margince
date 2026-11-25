// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// An empty text field has nothing in it to say it takes typing; its boundary
// does that alone. So WCAG 1.4.11 asks 3:1 of it, and --borderControl holds the
// ratio (tokens.test.ts). The fields are read from the tree, not listed.

import ts from "typescript";
import {
  alternativesOf,
  chain,
  classesOf,
  compounds,
  declaredValues,
  joined,
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

// Fields that rest with no edge inside a container no rule here can read.
const EDGELESS: Readonly<Record<string, string>> = {
  ".compose-sheet > .richtext":
    "the message body fills the composer's window, whose frame is its edge",
};

const ENTRY_TAGS = new Set(["input", "textarea", "select"]);
const TOGGLE_TYPES = ["checkbox", "radio", "range", "file", "color", "hidden"];
const TOGGLE = new RegExp(`\\[type=["']?(?:${TOGGLE_TYPES.join("|")})`);
const ENTRY_ELEMENT = /^(?:input|textarea|select)(?![\w-])/;
// `:focus` and `:focus-within` light a field whenever the caret is in it.
// A rule that answers only `:focus-visible` is a control taking the keyboard.
const CARET_FOCUS = /:focus(?:-within)?(?![\w-])/;
const CARETS = new RegExp(CARET_FOCUS.source, "g");
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
// A row rules a bare field when its rest rule draws a solid line in the
// control token all round it or along its foot.
const FOOT_SIDES = ["border-bottom", "border-block-end"];
const FOOT = ["border", ...FOOT_SIDES];
const RULED = /^\d+(?:\.\d+)?px\s+solid\s+var\(--borderControl\)$/;
// Every declaration that can take a foot away again.
const UNDRAWS = [
  ...FOOT,
  "border-color",
  "border-width",
  "border-style",
].concat(
  FOOT_SIDES.flatMap((side) =>
    ["color", "width", "style"].map((part) => `${side}-${part}`),
  ),
);
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
const atRest = (subject: string) =>
  !NOT_REST.test(subject.replace(NEGATED, ""));

// Every run of a selector's compounds that ends at compound `end`, written
// back with its combinators, to match against a frame's whole selector.
const runsEndingAt = (selector: string, end: number) => {
  const { compounds: parts, combinators: links } = chain(selector);
  return parts.slice(0, end + 1).map((_, start) =>
    joined({
      compounds: parts.slice(start, end + 1),
      combinators: links.slice(start, end),
    }),
  );
};
const wholeRun = (selector: string) =>
  runsEndingAt(selector, compounds(selector).length - 1)[0];

type Frames = {
  /** Classes a field carries wherever it is written. */
  fields: ReadonlySet<string>;
  /** Whole selectors of fields a focus rule names only inside some context. */
  framed: ReadonlySet<string>;
  /** Whole selectors of rows ruled in the control token. */
  rows: ReadonlySet<string>;
};

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
// box that recolours its edge where the caret is. A box lit only inside some
// context is a field there alone. Its whole selector is kept, and rules for
// it are read as a field's. A class compounded with a field class joins them,
// round after round.
function fieldClasses(rules: readonly Rule[], entry: ReadonlySet<string>) {
  const fields = new Set(entry);
  const framed = new Set<string>();
  for (const rule of rules) {
    for (const alternative of alternativesOf(rule.selector)) {
      const subject = subjectOf(alternative);
      const caret = CARET_FOCUS.test(subject.replace(NEGATED, ""));
      if (
        !caret ||
        edgesOf(rule).length === 0 ||
        litLikeHover(rule, subject, rules)
      ) {
        continue;
      }
      const parts = compounds(alternative);
      if (parts.length === 1) {
        for (const name of ownClasses(subject)) fields.add(name);
      } else {
        framed.add(wholeRun(alternative.replace(CARETS, "")));
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
  return { fields, framed };
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

// A field sits inside another field's frame at any depth. Or it sits on a ruled
// row: the compound just before it, or one further when joined by `>`.
function enclosed(alternative: string, { fields, framed, rows }: Frames) {
  const { compounds: parts, combinators: links } = chain(alternative);
  const end = parts.length - 2;
  const ancestors = parts.slice(0, -1);
  if (
    ancestors.some((compound) =>
      ownClasses(compound).some((name) => fields.has(name)),
    ) ||
    ancestors.some((_, at) =>
      runsEndingAt(alternative, at).some((run) => framed.has(run)),
    )
  ) {
    return true;
  }
  if (end < 0 || ![" ", ">"].includes(links[end])) return false;
  const near = links[end - 1] === ">" ? [end, end - 1] : [end];
  return near.some((at) =>
    runsEndingAt(alternative, at).some((run) => rows.has(run)),
  );
}

function findingsOf(rule: Rule, alternative: string, frames: Frames): string[] {
  const where = `${relative(rule)}: ${alternative}`;
  const edges = EDGES.flatMap((property) =>
    declaredValues(rule.body, property).map((value) => ({ property, value })),
  );
  return [
    ...edges.flatMap(({ property, value }) => {
      if (noEdge(value)) {
        return rulesOwnFoot(rule) ||
          alternative in EDGELESS ||
          enclosed(alternative, frames)
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

const valuesOf = (rule: Rule, properties: readonly string[]) =>
  properties.flatMap((property) => declaredValues(rule.body, property));
const ruled = (rule: Rule) =>
  valuesOf(rule, FOOT).some((value) => RULED.test(value));

type Side = { width: string; style: string; color: string };
const STYLE =
  /^(?:none|hidden|solid|dashed|dotted|double|groove|ridge|inset|outset)$/;
const WIDTH = /^(?:\d+(?:\.\d+)?(?:px|r?em)?|thin|medium|thick)$/;
const FOOT_SHORTHANDS = new Set([...FOOT, "border-block"]);
const FOOT_PART =
  /^border(?:-bottom|-block-end|(-block))?-(width|style|color)$/;

// A shorthand resets the side's width, style and colour, then sets those it names.
function sideOf(value: string): Side {
  const side = { width: "medium", style: "none", color: "currentcolor" };
  for (const token of splitTopLevel(value, " ")) {
    if (STYLE.test(token)) side.style = token;
    else if (WIDTH.test(token)) side.width = token;
    else side.color = token;
  }
  return side;
}

// The foot a rule leaves, read in declaration order. It starts as the field's
// own solid line, so a rule that only recolours the foot draws it.
function footOf(rule: Rule): Side {
  let foot: Side = { width: "1px", style: "solid", color: "" };
  for (const declaration of splitTopLevel(rule.body, ";")) {
    const at = declaration.indexOf(":");
    const property = declaration.slice(0, at).trim();
    const value = declaration.slice(at + 1).trim();
    const part = FOOT_PART.exec(property);
    if (FOOT_SHORTHANDS.has(property)) {
      foot = sideOf(value);
    } else if (part) {
      // A box list names the foot third; a block pair names it second.
      const box = splitTopLevel(value, " ");
      const named = /^border-(?:width|style|color)$/.test(property)
        ? (box[2] ?? box[0])
        : part[1]
          ? (box[1] ?? box[0])
          : value;
      foot = { ...foot, [part[2]]: named };
    }
  }
  return foot;
}

// A field may clear its other sides when the foot it leaves is a solid line in
// the control token.
function rulesOwnFoot(rule: Rule): boolean {
  const { width, style, color } = footOf(rule);
  return (
    !/^0(?:\.0+)?(?:px|r?em)?$/.test(width) &&
    style === "solid" &&
    color === CONTROL
  );
}

// A row is a box named by a class. Any rule on a box with those classes that
// takes the foot away, in any state, unseats it.
function ruledRows(rules: readonly Rule[]) {
  const unseating = rules
    .filter((rule) => valuesOf(rule, UNDRAWS).some(noEdge))
    .flatMap((rule) =>
      subjectsOf(rule)
        .filter((subject) => !subject.includes("::"))
        .map(ownClasses),
    );
  const seated = (alternative: string) => {
    const named = ownClasses(subjectOf(alternative));
    return (
      named.length > 0 &&
      atRest(subjectOf(alternative)) &&
      !unseating.some((classes) =>
        named.every((name) => classes.includes(name)),
      )
    );
  };
  return new Set(
    rules
      .filter(ruled)
      .flatMap((rule) => alternativesOf(rule.selector).filter(seated))
      .map(wholeRun),
  );
}

// A field's rest rule that draws its edge in anything but --borderControl.
function weakEdges(
  rules: readonly Rule[],
  { fields, framed }: Omit<Frames, "rows">,
) {
  const frames = { fields, framed, rows: ruledRows(rules) };
  // Child and descendant steps compare alike here: reading more boxes as
  // fields only widens what is checked.
  const loose = (selector: string) => selector.replaceAll(" > ", " ");
  const litInContext = new Set([...framed].map(loose));
  return rules.flatMap((rule) =>
    alternativesOf(rule.selector).flatMap((alternative) => {
      const subject = subjectOf(alternative);
      const field =
        (ENTRY_ELEMENT.test(subject) && !TOGGLE.test(subject)) ||
        ownClasses(subject).some((name) => fields.has(name)) ||
        litInContext.has(loose(wholeRun(alternative)));
      if (!field || !atRest(subject)) return [];
      return findingsOf(rule, alternative, frames);
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
    ...fieldClasses(rules, entryClasses(corpus.filter(isAppModule))),
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
    const { rules, fields, framed } = fieldsOf(corpus);
    return weakEdges(rules, { fields, framed });
  },
  advice:
    "draw a text field's resting edge in var(--borderControl), the token that holds 1.4.11's 3:1; a field with no edge sits inside a field, or on a row named just before it (or one `>` step further) whose rest rule draws a solid line in that token round it or at its foot",
};
