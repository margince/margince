// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import ts from "typescript";
import { afterAll, describe, expect, it } from "vitest";
import {
  FIELD_ROWS,
  labelledIn,
  NOT_ROWS,
} from "../../scripts/lib/dialogfields";
import {
  defOf,
  descendants,
  dialogSet,
  elementsIn,
  keyOf,
  markupFiles,
  modalBoxes,
  ownersIn,
  primitiveKey,
  sheetTexts,
  srcDir,
  textCensus,
  type Values,
  weightOf,
  whereOf,
} from "../../scripts/lib/dialoglayout";
import {
  dialogsIn,
  hints,
  rowsAcross,
  SCOPE_CAP,
} from "../../scripts/lib/dialogrows";
import { parseSource, sourceFileAt } from "../../scripts/lib/source-tree";
import { classVariants } from "../testing/classnames";

// Whichever test asks first builds the tree's census, inside its own timeout.
const TREE_BUILD = 60_000;
const once = <T>(make: () => T) => {
  let made: { value: T } | undefined;
  return () => {
    made ??= { value: make() };
    return made.value;
  };
};
const files = once(markupFiles);
const sources = once(() => files().map((f) => sourceFileAt(f)));
const owners = once(() => ownersIn(sheetTexts()));
const tree = once(() => rowsAcross(sources(), owners(), dialogSet()));
const census = once(() =>
  textCensus(files().map((f) => readFileSync(f, "utf8"))),
);

const AT = join(srcDir, "design-system", "planted.tsx");
const PRELUDE = `import { Modal } from "./modal";
import { ConfirmModal } from "./confirmmodal";
import { Checkbox, Field } from "./atoms";
import { ChoiceList } from "./choicelist";
import { Heading } from "./heading";
import { Stack } from "./stack";`;
const HELPERS = `function Rows() { return <><Field /><Field /></>; }
function Body() { return <div><p /><Field /></div>; }
function Inline() { return <Field labelHidden />; }
function Bare() { const cite = () => <Field className="m" />; return cite(); }
function Cited() { const cite = () => <i className="m" />; return <p>{cite()}</p>; }
function Refusals() { return <div className="m" />; }
function Wrapped() { return <div className="s"><p /><div className="m" /></div>; }
function Nest() { return <div className="s"><p /><div className="m"><Nest /></div></div>; }
function HostsView() { return <View slot={<Wrapped />} />; }
function Sheet({ children }) { return <Modal intent="form">{children}</Modal>; }
function Side({ children, intent = "drawer" }) { return <Modal intent={intent}>{children}</Modal>; }
function Shell({ body }) { return <Modal intent="form"><div>{body}</div></Modal>; }
function Framed({ body }) { return <div className="form-stack">{body}</div>; }
function Outer({ body }) { return <Framed body={body} />; }
function Explain({ body }) { return <Modal intent="form"><div className="form-stack">{body(1)}</div></Modal>; }
function Opened({ body, open }) { return <Modal intent="form">{open && body(1)}</Modal>; }
function Kept({ body }) { const shown = body(1); return <Modal intent="form">{shown}</Modal>; }
function Line({ control }) { return <div>{control}</div>; }
function Passed({ className }) { return <div className={className}><div className="s"><p /><div className="m" /></div></div>; }
function Merge({ className }) { return <div className={className}><p /><div className="m" /></div>; }
function Forward({ className }) { return <Merge className={className} />; }
function Lined({ look }) { return <div className={look}><Field /></div>; }
function Paired({ look }) { return <div><p /><div className={look} /></div>; }
function Gapped({ edge }) { return <div className="s"><p /><div className={edge} /></div>; }
function Banded({ look }) { return <div><div className={look} /><Field /></div>; }
function Hands({ edge }) { return <View slot={<div className={edge} />} />; }
const W2 = Wrapped;
const R2 = Rows;
const Ref2 = Refusals;
const O2 = Outer;
const F2 = Field;
function Ping() { return <div className="s"><p /><div className="m" /><Pong /></div>; }
function Pong() { return <div className="f"><Ping /></div>; }
function Tree() { return <div className="s"><p /><div className="m" />{open && <div className="f"><Tree /></div>}</div>; }
function Relay({ look }) { return <Gapped edge={look} />; }
function Local({ tone }) { const cls = tone; return <div className={cls}><p /><div className="m" /></div>; }
function Toggled({ on }) { return <div className={on ? "s" : "t"}><p /><div className="m" /></div>; }
function FwdAll(props) { return <Merge {...props} />; }
function Two(props) { return <FwdAll {...props} />; }
function TwoR({ ...rest }) { return <FwdAll {...rest} />; }
function Toned({ tone: t }) { return <div className={t}><p /><div className="m" /></div>; }
function SS(props) { return <Modal {...props}>{props.children}</Modal>; }
function SR({ children, ...rest }) { return <Modal {...rest}>{children}</Modal>; }
function FwdRest({ ...rest }) { return <Merge {...rest} />; }
function PropsAcc(props) { return <div className={props.className}><p /><div className="m" /></div>; }
function Padded({ className }) { return <Merge className={cx("pad", className)} />; }
function Templated({ className }) { return <Merge className={\`pad \${className}\`} />; }
const S2 = Sheet;
function Boxed({ children }) { return <div className="s">{children}</div>; }
const B2x = Boxed;
const Plain = a ? "div" : "section";
const WMm = memo(Wrapped);
const WFwd = forwardRef(Wrapped);
const WAlias = WMm;
function Shadow({ Wrapped }) { const Glyph = Wrapped; return <Glyph />; }
function Swap({ className }) { return className ? <div className={className}><p /><div className="m" /></div> : <Swap className="s" />; }
function Loop() { return <div className="f"><div className="s"><p /><div className="m" /></div><Loop /></div>; }
function Alt({ k }) { return <div className={k}><div className="s"><p /><div className="m" /></div><Alt k={k === "f" ? "g" : "f"} /></div>; }
function Fwd(props) { return render(props); }
function Labelled({ label }) { return <div className="s"><p>{label}</p></div>; }
function Configured({ intent, children }) { const cfg = useCfg(); return <Modal intent={cfg.intent}>{children}</Modal>; }
function Cfgd({ tone }) { const cfg = useCfg(); return <div className={cfg.tone}><p /></div>; }
function fields() { return <><p /><Field /></>; }
const held = <><p /><Field /></>;
export { Modal as Pane };`;
const keysOf = (source: ts.SourceFile) => dialogSet([source], dialogSet());
// Fixture text writes an interpolation as `#{…}`, so no string here reads as one.
const interpolated = (text: string) => text.replaceAll("#{", "${");
const plantedRows = (jsx: string, own = owners()) => {
  const text = `${PRELUDE}\nconst A = () => (${interpolated(jsx)});\n${HELPERS}`;
  const source = parseSource(AT, text);
  const { rows } = rowsAcross([source], own, keysOf(source));
  return rows.map((r) => r.verdict).join(" ") || "no rows";
};
const TITLE = '<Heading className="modal-title">A</Heading>';
const ACTIONS = '<div className="actions" />';
const inModal = (inner: string) =>
  `<Modal intent="form">${TITLE}${inner}${ACTIONS}</Modal>`;
const STACK =
  ".s { display: flex; flex-direction: column; gap: var(--space-3) }";
const FORM_STACK = STACK.replace(".s", ".form-stack");
const MARGIN = ".m { margin-top: var(--space-2) }";
const MARGINED_STACK =
  '<Modal intent="form"><div className="s"><Field /><Field className="m" /></div></Modal>';
const SCOPED =
  ".f .pb { display: flex; flex-direction: column; gap: var(--space-3) }";
const PARAGRAPH_STACK = '<div className="s"><p /><Field /></div>';
const DRAWER_STACK = `.modal-drawer ${STACK}`;
const DRAWER_BLOCK = `${STACK} .modal-drawer .s { display: block }`;
const HOSTED = inModal('<div className="f"><Wrapped /></div>');
const HOSTED_STACK = `.f ${STACK} ${MARGIN}`;
const HOSTED_MARGIN = `${STACK} .f ${MARGIN}`;
const UNSELECTED = 'cx(a && "q1", b && "q2", c && "q3", d && "q4")';
const BRANCHED = `<div className={${UNSELECTED}}><div className={cx("o", ${UNSELECTED})}>`;
type RowCase = { jsx: string; verdict: string; sheet: string };
const tempDirs: string[] = [];
const tempDir = (prefix: string) => {
  const dir = mkdtempSync(join(tmpdir(), prefix));
  tempDirs.push(dir);
  return dir;
};

// Three files, so a guest's guest is two files away from its dialog.
function plantedAcross(
  host = "<B />",
  rows = "<div><p /><Field /></div>",
  own = owners(),
) {
  const dir = tempDir("dialogrows-");
  const ds = relative(dir, join(srcDir, "design-system"));
  writeFileSync(
    join(dir, "a.tsx"),
    `import { Modal } from "${ds}/modal";\nimport { B } from "./b";\nexport const A = () => <Modal intent="form">${host}</Modal>;\n`,
  );
  writeFileSync(
    join(dir, "b.tsx"),
    `import { C } from "./c";\nexport function B() { return <C />; }\n`,
  );
  writeFileSync(
    join(dir, "c.tsx"),
    `import { Field } from "${ds}/atoms";\nexport function C() { return ${rows}; }\n`,
  );
  const a = sourceFileAt(join(dir, "a.tsx"));
  const { rows: found, depth } = rowsAcross([a], own, keysOf(a));
  return { verdicts: found.map((r) => r.verdict).join(" "), depth };
}

// A modal.tsx whose modalClass names and orders its parameters its own way.
function plantedModal(body: string) {
  const dir = tempDir("dialogbox-");
  const path = join(dir, "modal.tsx");
  writeFileSync(
    path,
    `const KINDS = ["sheet", "card"] as const; type Kind = (typeof KINDS)[number]; export function Modal({ kind, size = "default", placement = "center", children }) { return <div className={modalClass(kind, placement, size)}>{children}</div>; }
function modalClass(kind: Kind | undefined, where: "center" | "right", width: "default" | "wide") { ${body} }\n`,
  );
  return path;
}
const DRAWER_CLASS = `if (kind === "sheet") return "modal sheet"; if (where === "right") return width === "wide" ? "modal drawer wide" : "modal drawer"; return "modal";`;
const classCall = (expression: string) => {
  const code = `const x = <i className={${interpolated(expression)}} />;`;
  const source = parseSource(AT, code);
  const init = descendants(source).find(ts.isJsxAttribute)?.initializer;
  return () => classVariants(source, init);
};

describe("a dialog's field rows have an owner for the space between them", () => {
  afterAll(() => {
    for (const dir of tempDirs) rmSync(dir, { recursive: true, force: true });
  });

  it(
    "walks every dialog the text spells, wrappers included",
    () => {
      expect(tree().dialogs).toBe(census().dialogs);
      expect(dialogSet().keys.size).toBeGreaterThan(1);
      expect(tree().rows.length).toBeGreaterThan(0);
      expect(tree().depth).toBeGreaterThan(1);
    },
    TREE_BUILD,
  );

  it(
    "judges every component a dialog renders",
    () => {
      const { keys } = dialogSet();
      const fields = new Set(
        [...FIELD_ROWS].map(([name, file]) => primitiveKey(file, name)),
      );
      const shown = sources().flatMap((s) =>
        dialogsIn(s, keys).flatMap((d) =>
          elementsIn(d).flatMap((e) => defOf(e) ?? []),
        ),
      );
      const skipped = (def: ts.Node) =>
        keys.has(keyOf(def)) ||
        fields.has(keyOf(def)) ||
        tree().judged.has(def);
      expect(
        [...new Set(shown)].filter((d) => !skipped(d)).map(whereOf),
      ).toEqual([]);
    },
    TREE_BUILD,
  );

  it(
    "would count short of the text with only Modal and ConfirmModal",
    () => {
      const cut = new Set([
        dialogSet().modal,
        primitiveKey("confirmmodal", "ConfirmModal"),
      ]);
      const walked = sources().reduce(
        (n, s) => n + dialogsIn(s, cut).length,
        0,
      );
      expect(walked).toBeLessThan(census().dialogs);
    },
    TREE_BUILD,
  );

  it("spells a wrapper's wrapper, an alias and a role as dialogs", () => {
    const text = `import { Modal as Pane } from "./modal";
function Sheet({ children }) { return <Pane>{children}</Pane>; }
function Drawer({ children }) { return <Sheet>{children}</Sheet>; }
function Trigger({ children }) { return <Row>{children}<Modal /></Row>; }
// <Modal> in a comment
const Typed: FC<P> = ({ children }) => <Modal>{children}</Modal>;
const S2 = Sheet;
const S3: FC<P> = Sheet;
const A = () => <><Drawer /><div role="dialog" /><Trigger /><Typed /><S2 /><S3 /></>;`;
    expect(textCensus([text]).dialogs).toBe(9);
    const trailing = `${text}\nconst B = () => <p />; // <Modal> after code`;
    expect(textCensus([trailing]).dialogs).toBe(9);
  });

  it("follows a guest's guest into a third file", () => {
    expect(plantedAcross()).toEqual({ verdicts: "flush", depth: 2 });
  });

  it("reads a guest's rows under the boxes its host puts it in", () => {
    const scoped = ownersIn([SCOPED]);
    const stack = '<div className="pb"><p /><Field /></div>';
    const host = '<div className="f"><B /></div>';
    expect(plantedAcross(host, stack, scoped).verdicts).toBe("spaced");
    expect(plantedAcross("<B />", stack, scoped).verdicts).toBe("flush");
  });

  it(
    "refuses a guest fan-out past its scope cap",
    () => {
      const levels = Math.floor(Math.log2(SCOPE_CAP + 1)) + 1;
      const level = (i: number) =>
        `function L${i}() { return <><div className="a"><L${i + 1} /></div><div className="b"><L${i + 1} /></div></>; }`;
      const chain = Array.from({ length: levels }, (_, i) => level(i));
      const text = `${PRELUDE}\nconst A = () => <Modal intent="form"><L0 /></Modal>;\n${chain.join("\n")}`;
      const lineOf = (name: string) =>
        text.split("\n").findIndex((l) => l.startsWith(`function ${name}(`)) +
        1;
      const source = parseSource(AT, text);
      const boxed = ownersIn([`.a ${STACK} .b ${STACK}`]);
      expect(() => rowsAcross([source], boxed, keysOf(source))).toThrow(
        new RegExp(
          `planted.tsx:${lineOf("L0")}: more than \\d+ guest scopes to judge in the components this one calls, last at design-system/planted.tsx:${lineOf(`L${levels - 1}`)}; call them`,
        ),
      );
    },
    TREE_BUILD,
  );

  it("judges a component once whatever text its calls hand it", () => {
    const scopes = (n: number) => {
      const calls = Array.from(
        { length: n },
        (_, i) => `<Labelled label="l${i}" />`,
      );
      const text = `${PRELUDE}\nconst A = () => ${inModal(calls.join(""))};\n${HELPERS}`;
      const source = parseSource(AT, text);
      return rowsAcross([source], ownersIn([STACK]), keysOf(source)).scopes;
    };
    expect(scopes(40)).toBe(scopes(1));
  });

  it("keys a guest only on the props its classes read", () => {
    const scopes = (calls: string) => {
      const text = `${PRELUDE}\nconst A = () => ${inModal(calls)};\n${HELPERS}`;
      const source = parseSource(AT, text);
      return rowsAcross([source], ownersIn([STACK]), keysOf(source)).scopes;
    };
    const two = '<Cfgd tone="t" /><Cfgd tone="s" />';
    expect(scopes(two)).toBe(scopes('<Cfgd tone="t" />'));
  });

  it("names every design-system control that draws its own label", () => {
    const ds = files().filter(
      (f) => f.includes("/design-system/") && !f.endsWith(".stories.tsx"),
    );
    const found = ds.flatMap((f) => labelledIn(sourceFileAt(f)));
    expect(found.sort()).toEqual(
      [...FIELD_ROWS.keys(), ...NOT_ROWS.keys()].sort(),
    );
    const planted = parseSource(
      AT,
      `export function Pick() { return <label><input /></label>; }
const P2 = Pick;
export function Wrap() { return <P2 />; }
export const Named = Pick;`,
    );
    expect(labelledIn(planted)).toEqual(["Pick", "Wrap", "Named"]);
  });

  it("reads the Modal box classes by parameter position", () => {
    const path = plantedModal(DRAWER_CLASS);
    const given = new Map<string, Values>([
      ["placement", ["right"]],
      ["size", ["wide"]],
    ]);
    const drawer = ["modal", "drawer", "wide"];
    expect(modalBoxes(given, path)).toEqual([drawer]);
    given.set("kind", "any");
    expect(modalBoxes(given, path)).toEqual([["modal", "sheet"], drawer]);
    const any = new Map<string, Values>([["placement", "any"]]);
    expect(modalBoxes(any, path)).toEqual([["modal"], ["modal", "drawer"]]);
    const unread = plantedModal('switch (where) { default: return "modal"; }');
    expect(() => modalBoxes(new Map(), unread)).toThrow(/does not evaluate/);
    const empty = plantedModal('if (where === "right") return "modal";');
    expect(() => modalBoxes(new Map(), empty)).toThrow(/returns no class/);
  });

  it.each`
    expression                                    | variants
    ${'`q#{a ? " m" : ""}`'}                      | ${[["q", "m"], ["q"]]}
    ${"`tone-#{level}`"}                          | ${[["tone-*"]]}
    ${"`m#{k} base`"}                             | ${[["m*", "base"]]}
    ${"`a #{b} c`"}                               | ${[["a", "c"]]}
    ${'cx("a", open && `b-#{x ? "on" : "off"}`)'} | ${[["a"], ["a", "b-on"], ["a", "b-off"]]}
    ${'"tone-" + level'}                          | ${[["tone-*"]]}
    ${'"m" + (a ? "x y" : "z")'}                  | ${[["mx", "y"], ["mz"]]}
    ${'cx("a") + " b"'}                           | ${[["a", "b"]]}
    ${'"x-" + look(s)'}                           | ${[["x-*"]]}
    ${'`x #{["form-stack", "y"]}`'}               | ${[["x"]]}
    ${'`#{["a", "b"].join(" ")} c`'}              | ${[["a", "b", "c"]]}
  `("reads $expression as its branches", ({ expression, variants }) => {
    expect(classCall(expression)()).toEqual(variants);
  });

  it("reads a class read off an object as no class", () => {
    expect(classCall("styles.stack")()).toEqual([[]]);
  });

  it("refuses a class list past its branch cap", () => {
    const five = 'cx(a && "a", b && "b", c && "c", d && "d", e && "e")';
    expect(classCall(five)).toThrow(/more than 16 class branches/);
    const leaves = (n: number, p: string) =>
      `(${Array.from({ length: n - 1 }, (_, i) => `k === ${i} ? "${p}${i}" : `).join("")}"")`;
    expect(classCall(leaves(17, "c"))).toThrow(/more than 16/);
    expect(classCall(`${leaves(9, "a")} || ${leaves(9, "b")}`)).toThrow(
      /than 16/,
    );
  });

  it.each`
    selector                   | weight
    ${".a"}                    | ${1000}
    ${".a:where(.b .c)"}       | ${1000}
    ${":is(.a, .b .c)"}        | ${2000}
    ${":not(#x).a"}            | ${1_001_000}
    ${"div.a > p"}             | ${1002}
    ${".a::before"}            | ${1001}
    ${".a:hover"}              | ${2000}
    ${":nth-child(2 of .a.b)"} | ${3000}
  `("weighs $selector as $weight", ({ selector, weight }) => {
    expect(weightOf(selector)).toBe(weight);
  });

  it.each`
    spec                                                                | jsx                                                                                                                       | verdict                   | sheet
    ${"catches a field under a paragraph"}                              | ${inModal("<p /><Field />")}                                                                                              | ${"flush"}                | ${""}
    ${"catches a ChoiceList over a Checkbox"}                           | ${inModal("<ChoiceList /><Checkbox />")}                                                                                  | ${"flush flush"}          | ${""}
    ${"passes a lone field between title and actions"}                  | ${inModal("<Field />")}                                                                                                   | ${"spaced"}               | ${""}
    ${"passes fields in a .form-stack"}                                 | ${inModal('<div className="form-stack"><p /><Field /><Field /></div>')}                                                   | ${"spaced spaced"}        | ${""}
    ${"passes fields whose ConfirmModal stacks them"}                   | ${"<ConfirmModal><p /><Field /><Field /></ConfirmModal>"}                                                                 | ${"spaced spaced"}        | ${""}
    ${"reads through a fragment and a condition"}                       | ${inModal("<>{a && <Field />}<Field /></>")}                                                                              | ${"flush flush"}          | ${""}
    ${"reads one field a map repeats"}                                  | ${inModal("<div>{xs.map((x) => <Field key={x} />)}</div>")}                                                               | ${"flush"}                | ${""}
    ${"lets two branches of a ternary stand alone"}                     | ${inModal("{a ? <Field /> : <Field />}")}                                                                                 | ${"spaced spaced"}        | ${""}
    ${"lifts a lone field out of a bare wrapper"}                       | ${inModal("<p /><form><Field /></form>")}                                                                                 | ${"flush"}                | ${""}
    ${"passes fields side by side in a .form-row"}                      | ${inModal('<div className="form-stack"><div className="form-row"><Field /><Field /></div></div>')}                        | ${"spaced spaced spaced"} | ${""}
    ${"follows a component that renders field rows"}                    | ${inModal("<Rows />")}                                                                                                    | ${"flush"}                | ${""}
    ${"passes that component inside a stack"}                           | ${"<ConfirmModal><Rows /></ConfirmModal>"}                                                                                | ${"spaced"}               | ${""}
    ${"catches a field margin that doubles the gap"}                    | ${'<Modal intent="form"><div className="s"><Field className="x" /><Field /></div></Modal>'}                               | ${"doubled spaced"}       | ${`${STACK} .x { margin-top: var(--space-3) }`}
    ${"reads the stack from the stylesheet"}                            | ${'<Modal intent="form"><div className="s"><Field /><Field /></div></Modal>'}                                             | ${"flush flush"}          | ${".s { display: flex; flex-direction: column }"}
    ${"joins a stack and a gap spelled by two classes"}                 | ${'<Modal intent="form"><div className="c g"><Field /><Field /></div></Modal>'}                                           | ${"spaced spaced"}        | ${".c { display: flex; flex-direction: column } .g { gap: var(--space-3) }"}
    ${"reads the gap a Stack is given"}                                 | ${inModal('<Stack gap="2"><p /><Field /></Stack>')}                                                                       | ${"spaced"}               | ${""}
    ${"lifts a field out of a render prop"}                             | ${inModal("<p /><Gate>{() => <Field />}</Gate>")}                                                                         | ${"flush"}                | ${""}
    ${"calls a row handed to a prop it cannot place"}                   | ${inModal("<View slot={<><Field /><Field /></>} />")}                                                                     | ${"unplaced unplaced"}    | ${""}
    ${"calls a margin in a prop it cannot place"}                       | ${inModal('<View slot={<div className="m" />} />')}                                                                       | ${"unplaced"}             | ${MARGIN}
    ${"reads the class a component is handed"}                          | ${inModal('<Card className="s"><Field /><Field /></Card>')}                                                               | ${"spaced spaced"}        | ${STACK}
    ${"follows a component the dialog renders"}                         | ${inModal("<Body />")}                                                                                                    | ${"flush"}                | ${""}
    ${"catches any margin a dialog stack's child adds"}                 | ${inModal('<div className="s"><p /><div className="m" /></div>')}                                                         | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"catches the action row's margin inside a stack"}                 | ${inModal('<div className="form-stack"><Field /><div className="actions" /></div>')}                                      | ${"spaced doubled"}       | ${""}
    ${"catches a margin on a component's root"}                         | ${"<ConfirmModal><Refusals /></ConfirmModal>"}                                                                            | ${"doubled"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"lets a heavier rule under the stack zero it"}                    | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .s > .m { margin-top: 0 }`}
    ${"lets a rule for a child of the stack count the gap"}             | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .s > .m { margin-top: var(--space-1) }`}
    ${"ignores a margin set under another context"}                     | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} .other .m { margin-top: var(--space-2) }`}
    ${"ignores an element out of flow"}                                 | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .m { position: absolute }`}
    ${"reads :last-child against the row's place"}                      | ${'<Modal intent="form"><div className="s"><Field className="m" /><Field className="m" /></div></Modal>'}                 | ${"doubled spaced"}       | ${`${STACK} ${MARGIN} .m:last-child { margin-top: 0 }`}
    ${"ignores a vertical margin in a row"}                             | ${'<Modal intent="form"><div className="r"><Field className="m" /><Field /></div></Modal>'}                               | ${"spaced spaced"}        | ${`.r { display: flex; gap: var(--space-3) } ${MARGIN}`}
    ${"leaves the title to the title gate"}                             | ${'<Modal intent="form"><div className="form-stack"><Heading className="modal-title">A</Heading><Field /></div></Modal>'} | ${"spaced"}               | ${""}
    ${"reads a stack scoped to another host as no stack"}               | ${'<Modal intent="form"><div className="pb"><p /><Field /></div></Modal>'}                                                | ${"flush"}                | ${SCOPED}
    ${"reads that stack under its host"}                                | ${'<Modal intent="form"><div className="f"><div className="pb"><p /><Field /></div></div></Modal>'}                       | ${"spaced"}               | ${SCOPED}
    ${"lets the dialog stack zero every row's margin"}                  | ${MARGINED_STACK}                                                                                                         | ${"spaced spaced"}        | ${`${STACK} ${MARGIN} .modal .s > * { margin-block: 0 }`}
    ${"lets a later two-class rule beat that zero"}                     | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} .modal .s > * { margin-block: 0 } .modal .m { margin-top: var(--space-2) }`}
    ${"reads a gap shorthand after its longhand"}                       | ${'<Modal intent="form"><div className="s"><Field /><Field /></div></Modal>'}                                             | ${"spaced spaced"}        | ${".s { display: flex; flex-direction: column; row-gap: 0; gap: var(--space-3) }"}
    ${"reads a context's ancestors in order"}                           | ${'<Modal intent="form"><div className="o"><div className="b"><p /><Field /></div></div></Modal>'}                        | ${"flush"}                | ${".o .modal .b { display: flex; flex-direction: column; gap: var(--space-3) }"}
    ${"reads only the ancestor classes some rule selects"}              | ${inModal(`${BRANCHED}${PARAGRAPH_STACK}</div></div>`)}                                                                   | ${"spaced"}               | ${`.o ${STACK}`}
    ${"keeps an unselected box between a child rule's parent and row"}  | ${inModal(`<div className="o"><div className="q1">${PARAGRAPH_STACK}</div></div>`)}                                       | ${"flush"}                | ${`.o > ${STACK}`}
    ${"weighs :last-child above a later bare class"}                    | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} .m:last-child { margin-top: var(--space-2) } .m { margin-top: 0 }`}
    ${"weighs :is() by its heaviest argument"}                          | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} :is(.s, .q .r) > * { margin-block: 0 } .s .m { margin-top: var(--space-2) }`}
    ${"reads a component that wraps Modal as a dialog"}                 | ${"<Sheet><p /><Field /></Sheet>"}                                                                                        | ${"flush"}                | ${""}
    ${"reads Modal re-exported under another name"}                     | ${'<Pane intent="form"><p /><Field /></Pane>'}                                                                            | ${"flush"}                | ${""}
    ${"reads an element with role=dialog as a dialog"}                  | ${'<div role="dialog"><p /><Field /></div>'}                                                                              | ${"flush"}                | ${""}
    ${"reads a component with role=dialog as a dialog"}                 | ${'<Card role="dialog"><p /><Field /></Card>'}                                                                            | ${"flush"}                | ${""}
    ${"reads a two-value margin with !important"}                       | ${'<Modal intent="form"><div className="s"><Field className="m" /><Field /></div></Modal>'}                               | ${"spaced spaced"}        | ${`${STACK} .m { margin: 0 auto !important }`}
    ${"reads rows held in a JSX variable"}                              | ${inModal("{held}")}                                                                                                      | ${"flush"}                | ${""}
    ${"reads rows a function returns"}                                  | ${inModal("{fields()}")}                                                                                                  | ${"flush"}                | ${""}
    ${"places a prop's rows where the dialog puts them"}                | ${"<Shell body={<><p /><Field /></>} />"}                                                                                 | ${"flush"}                | ${""}
    ${"places a prop's rows in the stack that holds them"}              | ${inModal("<Framed body={<><p /><Field /></>} />")}                                                                       | ${"spaced"}               | ${""}
    ${"follows a prop handed on to another component"}                  | ${inModal("<Outer body={<><p /><Field /></>} />")}                                                                        | ${"spaced"}               | ${""}
    ${"places a render prop's rows where it is called"}                 | ${"<Explain body={(n) => <><p /><Field /></>} />"}                                                                        | ${"spaced"}               | ${""}
    ${"places a render prop called under a condition"}                  | ${"<Opened body={(n) => <><p /><Field /></>} />"}                                                                         | ${"flush"}                | ${""}
    ${"calls a render prop it cannot place unplaced"}                   | ${"<Kept body={(n) => <><p /><Field /></>} />"}                                                                           | ${"unplaced"}             | ${""}
    ${"places a prop through a component inside its own prop"}          | ${inModal("<p /><Line control={<Line control={<Field />} />} />")}                                                        | ${"flush"}                | ${""}
    ${"reads the drawer's own box classes"}                             | ${`<Modal intent="drawer">${PARAGRAPH_STACK}</Modal>`}                                                                    | ${"spaced"}               | ${DRAWER_STACK}
    ${"judges each intent a ternary picks"}                             | ${`<Modal intent={a ? "drawer" : "confirm"}>${PARAGRAPH_STACK}</Modal>`}                                                  | ${"flush"}                | ${DRAWER_BLOCK}
    ${"judges every intent an unread value can be"}                     | ${`<Modal intent={where}>${PARAGRAPH_STACK}</Modal>`}                                                                     | ${"flush"}                | ${DRAWER_BLOCK}
    ${"reads the intent a wrapper defaults to"}                         | ${`<Side>${PARAGRAPH_STACK}</Side>`}                                                                                      | ${"spaced"}               | ${DRAWER_STACK}
    ${"ignores a rule that holds only under a query"}                   | ${`<Modal intent="form">${PARAGRAPH_STACK}</Modal>`}                                                                      | ${"spaced"}               | ${`${STACK} @media (min-width: 1px) { .s { display: block } }`}
    ${"lets !important outrank a heavier rule"}                         | ${`<Modal intent="form">${PARAGRAPH_STACK}</Modal>`}                                                                      | ${"spaced"}               | ${".s { display: flex !important; flex-direction: column; gap: var(--space-3) } .modal .s { display: block }"}
    ${"stacks only if every branch of a class does"}                    | ${'<Modal intent="form"><div className={a ? "s" : "t"}><p /><Field /></div></Modal>'}                                     | ${"flush"}                | ${STACK}
    ${"doubles if any branch of a class margins"}                       | ${'<Modal intent="form"><div className="s"><Field /><Field className={a && "m"} /></div></Modal>'}                        | ${"spaced doubled"}       | ${`${STACK} ${MARGIN}`}
    ${"doubles if a template's branch margins"}                         | ${'<Modal intent="form"><div className="s"><Field /><Field className={`f#{a ? " m" : ""}`} /></div></Modal>'}             | ${"spaced doubled"}       | ${`${STACK} ${MARGIN}`}
    ${"reads a class an interpolation cuts as each it can be"}          | ${'<Modal intent="form"><div className="s"><Field /><Field className={`m#{k}`} /></div></Modal>'}                         | ${"spaced doubled"}       | ${`${STACK} .mx { margin-top: var(--space-2) }`}
    ${"weighs :where() as nothing"}                                     | ${MARGINED_STACK}                                                                                                         | ${"spaced doubled"}       | ${`${STACK} .modal :where(.s) > * { margin-block: 0 } ${MARGIN}`}
    ${"leaves a field editing in place among text alone"}               | ${inModal("<div><span /><Field labelHidden /></div>")}                                                                    | ${"no rows"}              | ${""}
    ${"still catches the same field with its label shown"}              | ${inModal("<div><span /><Field /></div>")}                                                                                | ${"flush"}                | ${""}
    ${"catches a field whose label shows in a branch"}                  | ${inModal("<div><span /><Field labelHidden={a} /></div>")}                                                                | ${"flush"}                | ${""}
    ${"leaves a component editing in place alone"}                      | ${inModal("<div><span /><Inline /></div>")}                                                                               | ${"no rows"}              | ${""}
    ${"keeps a local function's element where it is used"}              | ${"<ConfirmModal><Cited /></ConfirmModal>"}                                                                               | ${"no rows"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"roots a local function's element the component returns"}         | ${"<ConfirmModal><Bare /></ConfirmModal>"}                                                                                | ${"doubled"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"reads a same-file stack under the box its call is in"}           | ${HOSTED}                                                                                                                 | ${"doubled"}              | ${HOSTED_STACK}
    ${"reads that stack as absent where its call has no box"}           | ${inModal("<Wrapped />")}                                                                                                 | ${"no rows"}              | ${HOSTED_STACK}
    ${"reads a same-file margin under the box its call is in"}          | ${HOSTED}                                                                                                                 | ${"doubled"}              | ${HOSTED_MARGIN}
    ${"reads that margin as absent where its call has no box"}          | ${inModal("<Wrapped />")}                                                                                                 | ${"no rows"}              | ${HOSTED_MARGIN}
    ${"judges a same-file component under each call's boxes"}           | ${inModal('<Wrapped /><div className="f"><Wrapped /></div>')}                                                             | ${"doubled"}              | ${HOSTED_STACK}
    ${"catches a margin on a same-file root in a form stack"}           | ${inModal('<div className="form-stack"><Refusals /></div>')}                                                              | ${"doubled"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"ends at a component that renders itself"}                        | ${inModal("<Nest />")}                                                                                                    | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"keeps an unplaced prop out of another call's boxes"}             | ${'<><Modal intent="form"><div className="f"><HostsView /></div></Modal><Kept body={(n) => <Wrapped />} /></>'}           | ${"no rows"}              | ${HOSTED_STACK}
    ${"keeps a guest's unplaced prop out of its call's boxes"}          | ${inModal('<div className="f"><HostsView /></div>')}                                                                      | ${"no rows"}              | ${HOSTED_STACK}
    ${"reads a guest under the class its call hands its root"}          | ${inModal('<Passed className="f" />')}                                                                                    | ${"doubled"}              | ${HOSTED_STACK}
    ${"reads a guest's root as the stack its call makes it"}            | ${inModal('<Merge className="s" />')}                                                                                     | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a guest once for each class its calls hand"}              | ${inModal('<Merge className="t" /><Merge className="s" />')}                                                              | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"hands a class on through a guest's own call"}                    | ${inModal('<Forward className="s" />')}                                                                                   | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"counts a root a call's class makes a form row"}                  | ${inModal('<p /><Lined look="form-row" />')}                                                                              | ${"flush spaced"}         | ${""}
    ${"counts a guest's element its call makes a form row"}             | ${inModal('<Paired look="form-row" />')}                                                                                  | ${"flush"}                | ${""}
    ${"catches a margin a guest's call hands an element"}               | ${inModal('<Gapped edge="m" />')}                                                                                         | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"reads a band a guest's call hands the row above"}                | ${inModal('<Banded look="band" />')}                                                                                      | ${"spaced"}               | ${".band { margin-bottom: var(--space-3) }"}
    ${"calls a margin a guest's call hands an unplaced prop"}           | ${inModal('<Hands edge="m" />')}                                                                                          | ${"unplaced"}             | ${MARGIN}
    ${"follows a guest renamed by an alias"}                            | ${inModal('<div className="f"><W2 /></div>')}                                                                             | ${"doubled"}              | ${HOSTED_STACK}
    ${"counts the rows an aliased component renders"}                   | ${inModal("<R2 />")}                                                                                                      | ${"flush"}                | ${""}
    ${"catches a margin on an aliased component's root"}                | ${"<ConfirmModal><Ref2 /></ConfirmModal>"}                                                                                | ${"doubled"}              | ${`${FORM_STACK} ${MARGIN}`}
    ${"places a prop through an aliased component"}                     | ${inModal("<O2 body={<><p /><Field /></>} />")}                                                                           | ${"spaced"}               | ${""}
    ${"reads an alias of a field as a field"}                           | ${inModal("<p /><F2 />")}                                                                                                 | ${"flush"}                | ${""}
    ${"reads the body of a component a call classes as a row"}          | ${inModal('<Wrapped className="form-row" />')}                                                                            | ${"flush doubled"}        | ${`${STACK} ${MARGIN}`}
    ${"reads that body when the call sits in an unplaced prop"}         | ${inModal('<View slot={<Wrapped className="form-row" />} />')}                                                            | ${"unplaced doubled"}     | ${`${STACK} ${MARGIN}`}
    ${"judges a component again where a partner hosts it"}              | ${inModal("<Ping />")}                                                                                                    | ${"doubled"}              | ${HOSTED_STACK}
    ${"judges a component again where it hosts itself"}                 | ${inModal("<Tree />")}                                                                                                    | ${"doubled"}              | ${HOSTED_STACK}
    ${"reads a bare attribute as true"}                                 | ${inModal("<Toggled on />")}                                                                                              | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"reads a class a spread at the call hands"}                       | ${inModal('<Passed {...{ className: "f" }} />')}                                                                          | ${"doubled"}              | ${HOSTED_STACK}
    ${"hands a class on through a spread of the props"}                 | ${inModal('<FwdAll className="s" />')}                                                                                    | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"hands a class on through a spread of the rest"}                  | ${inModal('<FwdRest className="s" />')}                                                                                   | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a forwarding guest once for each class it hands on"}      | ${inModal('<Forward className="t" /><Forward className="s" />')}                                                          | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a spreading guest once for each class it hands on"}       | ${inModal('<FwdAll className="t" /><FwdAll className="s" />')}                                                            | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a guest once for each class a spread hands it"}           | ${inModal('<FwdAll {...{ className: "t" }} /><FwdAll {...{ className: "s" }} />')}                                        | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a guest once for each class its caller's props spread"}   | ${inModal('<Two className="t" /><Two className="s" />')}                                                                  | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a guest once for each class its caller's rest spreads"}   | ${inModal('<TwoR className="t" /><TwoR className="s" />')}                                                                | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"reads a renamed prop by the name the call hands"}                | ${inModal('<Toned tone="s" />')}                                                                                          | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"ignores an attribute named like a renamed prop's binding"}       | ${inModal('<Toned t="s" />')}                                                                                             | ${"no rows"}              | ${`${STACK} ${MARGIN}`}
    ${"reads the intent a wrapper spreads from its props"}              | ${`<SS intent="form">${PARAGRAPH_STACK}</SS>`}                                                                            | ${"spaced"}               | ${DRAWER_BLOCK}
    ${"reads the drawer a wrapper spreads from its props"}              | ${`<SS intent="drawer">${PARAGRAPH_STACK}</SS>`}                                                                          | ${"spaced"}               | ${DRAWER_STACK}
    ${"reads every intent a wrapper hands from a field it cannot read"} | ${`<Configured intent="form">${PARAGRAPH_STACK}</Configured>`}                                                            | ${"flush"}                | ${DRAWER_BLOCK}
    ${"reads the drawer a wrapper spreads from its rest"}               | ${`<SR intent="drawer">${PARAGRAPH_STACK}</SR>`}                                                                          | ${"spaced"}               | ${DRAWER_STACK}
    ${"judges a guest once for each prop it hands a call"}              | ${inModal('<Relay look="t" /><Relay look="m" />')}                                                                        | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a guest once for each prop a local class reads"}          | ${inModal('<Local tone="t" /><Local tone="s" />')}                                                                        | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"reads a class read off the props object"}                        | ${inModal('<PropsAcc className="s" />')}                                                                                  | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"hands a class on joined with another"}                           | ${inModal('<Padded className="s" />')}                                                                                    | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"hands a class on inside a template"}                             | ${inModal('<Templated className="s" />')}                                                                                 | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"reads an aliased dialog wrapper as a dialog"}                    | ${"<S2><p /><Field /></S2>"}                                                                                              | ${"flush"}                | ${""}
    ${"reads the children box of an aliased component"}                 | ${inModal('<B2x><p /><div className="m" /></B2x>')}                                                                       | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"keeps the classes of an element a binding names"}                | ${inModal('<div className="s"><p /><Plain className="m" /></div>')}                                                       | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"keeps the classes of a component it cannot read"}                | ${inModal('<div className="s"><p /><Fwd className="m" /></div>')}                                                         | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"reads the rows inside a memo as well as its own classes"}        | ${inModal('<div className="s"><p /><WMm className="m" /></div>')}                                                         | ${"doubled doubled"}      | ${`${STACK} ${MARGIN}`}
    ${"reads the rows inside a forwardRef the same way"}                | ${inModal('<div className="s"><p /><WFwd className="m" /></div>')}                                                        | ${"doubled doubled"}      | ${`${STACK} ${MARGIN}`}
    ${"reads through an alias of a memo to the rows inside it"}         | ${inModal('<div className="s"><p /><WAlias className="m" /></div>')}                                                      | ${"doubled doubled"}      | ${`${STACK} ${MARGIN}`}
    ${"reads an alias of a prop as unknown, not the file's own name"}   | ${inModal("<Shadow />")}                                                                                                  | ${"no rows"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a component again where it hands itself new props"}       | ${inModal("<Swap />")}                                                                                                    | ${"doubled"}              | ${`${STACK} ${MARGIN}`}
    ${"judges a component that renders only itself once"}               | ${inModal("<Loop />")}                                                                                                    | ${"doubled"}              | ${HOSTED_STACK}
    ${"judges a component that flips its own props and ends"}           | ${inModal('<Alt k="f" />')}                                                                                               | ${"doubled"}              | ${HOSTED_STACK}
  `("$spec", ({ jsx, verdict, sheet }: RowCase) => {
    expect(plantedRows(jsx, sheet ? ownersIn([sheet]) : owners())).toBe(
      verdict,
    );
  });

  it(
    "spaces every field row in the tree",
    () => {
      const loose = tree().rows.filter((r) => r.verdict !== "spaced");
      expect(
        loose.map((r) => `${r.where} ${r.verdict}${r.by ? ` by ${r.by}` : ""}`),
        `flush: ${hints.flush}; doubled: ${hints.doubled}; unplaced: ${hints.unplaced}`,
      ).toEqual([]);
    },
    TREE_BUILD,
  );
});
