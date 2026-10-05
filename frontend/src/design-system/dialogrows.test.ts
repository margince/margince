// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  attr,
  classes,
  declarations,
  definedName,
  definition,
  descendants,
  elementsIn,
  enclosing,
  isZero,
  keyOf,
  localNames,
  markupFiles,
  modalClasses,
  type Owners,
  opening,
  ownersIn,
  type Placed,
  type Prop,
  sheetTexts,
  srcDir,
  tag,
  within,
} from "../../scripts/lib/dialoglayout";
import { parseSource, sourceFileAt } from "../../scripts/lib/source-tree";
import { classVariants } from "../testing/classnames";

// A dialog's rows sit one stack step apart: none flush, none doubled by its own
// margin. Stacks and margins are read from the sheets in each box's context.

const FIELD_ROWS = ["Field", "ChoiceList", "Checkbox", "Radio"];
type Verdict = "spaced" | "flush" | "doubled" | "unplaced";
type Row = { at: string; where: string; verdict: Verdict; by?: string };
type Guest = { file: string; name: string };
type Links = Map<ts.Node, ts.Node>;
type Slot = { roots: ts.Node[]; links: Links };

const upper = (n: ts.Node) => /^[A-Z]/.test(tag(n));
const childOfJsx = (n: ts.Node) =>
  !!n.parent && (ts.isJsxElement(n.parent) || ts.isJsxFragment(n.parent));
const isDialogTag = (file: ts.SourceFile, name: string, keys: Set<string>) => {
  const def = definition(file, name);
  return !!def && keys.has(keyOf(def));
};
// Only a label hidden in every render: `labelHidden={x}` shows it in one.
const inPlace = (n: ts.Node) => {
  const hidden = attr(n, "labelHidden");
  const e = hidden?.initializer;
  return (
    !!hidden &&
    (!e || (ts.isJsxExpression(e) && e.expression?.getText() === "true"))
  );
};
const literal = (n: ts.Node, name: string) => {
  const init = attr(n, name)?.initializer;
  const e = init && ts.isJsxExpression(init) ? init.expression : init;
  return e && ts.isStringLiteral(e) ? e.text : undefined;
};

// The names a component is handed: its destructured props, or `props.x`.
function givenTo(def: ts.Node) {
  const fn =
    ts.isVariableDeclaration(def) && def.initializer ? def.initializer : def;
  const first =
    ts.isFunctionLike(fn) && fn.parameters.length > 0
      ? fn.parameters[0].name
      : undefined;
  const names = new Set(["children"]);
  if (first && ts.isObjectBindingPattern(first)) {
    for (const b of first.elements) names.add(b.name.getText());
  }
  const props = first && ts.isIdentifier(first) ? first.text : undefined;
  return (e: ts.Node, name?: string) =>
    (ts.isIdentifier(e) &&
      names.has(e.text) &&
      (name === undefined || e.text === name)) ||
    (ts.isPropertyAccessExpression(e) &&
      e.expression.getText() === props &&
      (name === undefined || e.name.text === name));
}

// A component that renders a dialog around its children is a dialog, to a
// fixed point. Any other prop is followed to where it renders instead.
function dialogKeys(files: readonly ts.SourceFile[], seed: Set<string>) {
  const keys = new Set(seed);
  const candidates = files.flatMap((file) =>
    [...declarations(file)]
      .filter(([name]) => /^[A-Z]/.test(name))
      .map(([, def]) => {
        const given = givenTo(def);
        const handed = descendants(def).filter(
          (x) =>
            ts.isJsxExpression(x) &&
            !!x.expression &&
            given(x.expression, "children"),
        );
        const around = new Set<ts.Node>();
        for (const x of handed) {
          for (let p = enclosing(x); p && within(p, def); p = enclosing(p)) {
            if (upper(p)) around.add(p);
          }
        }
        return { file, def, around: [...around] };
      })
      .filter((c) => c.around.length > 0),
  );
  for (let grew = true; grew; ) {
    grew = false;
    for (const { file, def, around } of candidates) {
      if (keys.has(keyOf(def))) continue;
      if (around.some((e) => isDialogTag(file, tag(e), keys))) {
        keys.add(keyOf(def));
        grew = true;
      }
    }
  }
  return keys;
}

// The JSX an expression evaluates to, through parentheses and conditions.
function jsxOf(e: ts.Node | undefined): ts.Node[] {
  if (!e) return [];
  if (ts.isParenthesizedExpression(e)) return jsxOf(e.expression);
  if (opening(e) || ts.isJsxFragment(e)) return [e];
  if (ts.isConditionalExpression(e)) {
    return [...jsxOf(e.whenTrue), ...jsxOf(e.whenFalse)];
  }
  return ts.isBinaryExpression(e) ? jsxOf(e.right) : [];
}
function returned(def: ts.Node): ts.Node[] {
  const fn =
    ts.isVariableDeclaration(def) && def.initializer ? def.initializer : def;
  if (!ts.isFunctionLike(fn)) return [];
  if (ts.isArrowFunction(fn) && !ts.isBlock(fn.body)) return jsxOf(fn.body);
  return descendants(fn)
    .filter(ts.isReturnStatement)
    .filter((r) => ts.findAncestor(r.parent, ts.isFunctionLike) === fn)
    .flatMap((r) => jsxOf(r.expression));
}

// The elements a component renders at its root. JSX a named local function
// builds lands where that function is used, at the root only if a use is.
function rootsOf(def: ts.Node): ts.Node[] {
  const own =
    ts.isVariableDeclaration(def) && def.initializer ? def.initializer : def;
  const nameOf = (fn: ts.Node) => {
    if (ts.isFunctionDeclaration(fn)) return fn.name;
    const bound = fn.parent;
    return ts.isVariableDeclaration(bound) && ts.isIdentifier(bound.name)
      ? bound.name
      : undefined;
  };
  const atRoot = (n: ts.Node, hops: number): boolean => {
    const parent = enclosing(n);
    if (parent && within(parent, def)) return false;
    const fn = ts.findAncestor(n.parent, ts.isFunctionLike);
    const name = fn && fn !== own && within(fn, def) && nameOf(fn);
    if (!fn || !name || hops === 0) return true;
    return descendants(def).some(
      (use) =>
        ts.isIdentifier(use) &&
        use.text === name.text &&
        use !== name &&
        !within(use, fn) &&
        atRoot(use, hops - 1),
    );
  };
  return elementsIn(def).filter((e) => atRoot(e, 4));
}

function rowsIn(
  source: ts.SourceFile,
  own: Owners,
  keys: Set<string>,
  hosted: ReadonlySet<string> = new Set(),
) {
  const isDialog = (n: ts.Node) =>
    upper(n)
      ? isDialogTag(n.getSourceFile(), tag(n), keys)
      : literal(n, "role") === "dialog";
  const defOf = (n: ts.Node) =>
    upper(n) ? definition(n.getSourceFile(), tag(n)) : undefined;
  const variantsOf = (n: ts.Node) =>
    classVariants(n.getSourceFile(), attr(n, "className")?.initializer);

  // Rows rendered away from where they are written: a JSX variable or call
  // (`inline`), and JSX handed to a prop (`slots`), placed where it renders.
  const inline = new Map<ts.Node, ts.Node[]>();
  const slots = new Map<ts.Node, Slot[]>();
  const siteOf = new Map<ts.Node, ts.Node>();
  const frameOf = new Map<ts.Node, Links>();
  const unplaced = new Set<ts.Node>();
  let links: Links = new Map();
  const up = (n: ts.Node): ts.Node | undefined =>
    siteOf.get(n) ?? links.get(n)?.parent ?? n.parent;
  const enclosingV = (n: ts.Node) => {
    for (let p = up(n); p; p = up(p)) if (ts.isJsxElement(p)) return p;
    return undefined;
  };
  const hostedAt = (n: ts.Node) => [
    ...(inline.get(n) ?? []),
    ...(slots.get(n) ?? [])
      .filter((s) => s.links === links)
      .flatMap((s) => s.roots),
  ];
  const vdesc = (root: ts.Node) => {
    const out: ts.Node[] = [];
    const seen = new Set<ts.Node>();
    const visit = (n: ts.Node) => {
      if (seen.has(n)) return;
      seen.add(n);
      out.push(n);
      for (const r of hostedAt(n)) visit(r);
      ts.forEachChild(n, visit);
    };
    ts.forEachChild(root, visit);
    return out;
  };
  for (const x of descendants(source)) {
    if (!ts.isJsxExpression(x) || !x.expression || !childOfJsx(x)) continue;
    const e = x.expression;
    const callee = ts.isCallExpression(e) ? e.expression : undefined;
    const named = ts.isIdentifier(e) ? e : callee;
    const decl = named && declarations(source).get(named.getText());
    const value =
      decl && ts.isIdentifier(e) && ts.isVariableDeclaration(decl)
        ? jsxOf(decl.initializer)
        : [];
    const roots = callee && decl ? returned(decl) : value;
    if (roots.length === 0) continue;
    inline.set(x, roots);
    for (const r of roots) if (!siteOf.has(r)) siteOf.set(r, x);
  }
  // Where a component puts the prop `name`, following it through components
  // that hand it on.
  const slotSite = (
    call: ts.Node,
    name: string,
    path: Links,
    depth = 0,
  ): Slot["links"] | undefined => {
    const def = defOf(call);
    if (!def || depth > 4) return undefined;
    const next: Links = new Map(path).set(def, call);
    const given = givenTo(def);
    const site = descendants(def).find(
      (x) =>
        ((ts.isJsxExpression(x) && childOfJsx(x)) || ts.isReturnStatement(x)) &&
        !!x.expression &&
        given(x.expression, name),
    );
    if (site) {
      next.set(site, site);
      return next;
    }
    for (const a of descendants(def).filter(ts.isJsxAttribute)) {
      const e = a.initializer;
      const on = a.parent.parent;
      if (!e || !ts.isJsxExpression(e) || !e.expression) continue;
      if (!given(e.expression, name)) continue;
      const element = ts.isJsxOpeningElement(on) ? on.parent : on;
      const found = slotSite(element, a.name.getText(), next, depth + 1);
      if (found) return found;
    }
    return undefined;
  };
  for (const a of descendants(source).filter(ts.isJsxAttribute)) {
    const e = a.initializer;
    const roots = e && ts.isJsxExpression(e) ? jsxOf(e.expression) : [];
    const on = a.parent.parent;
    const call = ts.isJsxOpeningElement(on) ? on.parent : on;
    if (roots.length === 0 || !upper(call)) continue;
    const found = slotSite(call, a.name.getText(), new Map());
    const site = found && [...found].find(([k, v]) => k === v)?.[0];
    if (!found || !site) {
      for (const r of roots) unplaced.add(r);
      continue;
    }
    found.delete(site);
    slots.set(site, [...(slots.get(site) ?? []), { roots, links: found }]);
    for (const r of roots) {
      siteOf.set(r, site);
      frameOf.set(r, found);
    }
  }
  const frameFor = (n: ts.Node) => {
    for (let p: ts.Node | undefined = n; p; p = p.parent) {
      const frame = frameOf.get(p);
      if (frame) return frame;
    }
    return new Map<ts.Node, ts.Node>();
  };
  const unplacedRow = (n: ts.Node) => {
    for (let p: ts.Node | undefined = n; p; p = p.parent) {
      if (unplaced.has(p)) return true;
    }
    return false;
  };

  // A field whose label is hidden edits a value in place among its text,
  // rather than standing as a row of a form.
  const isField = (n: ts.Node) =>
    (FIELD_ROWS.some((f) => localNames(n.getSourceFile(), f).has(tag(n))) &&
      !inPlace(n)) ||
    classes(n).includes("form-row");
  // A component counts by the field rows it renders at its own root.
  const width = (n: ts.Node) => {
    if (isField(n)) return 1;
    const def = defOf(n);
    if (!def || isDialog(n)) return 0;
    return rootsOf(def).filter(isField).length;
  };
  const slotOf = (component: ts.Node) => {
    const def = defOf(component);
    const slot =
      def &&
      descendants(def).find(
        (n) =>
          ts.isJsxExpression(n) &&
          /^(props\.)?children$/.test(n.expression?.getText() ?? ""),
      );
    return slot && enclosing(slot);
  };
  const boxOf = (p: ts.Node) => {
    const slot = upper(p) && slotOf(p);
    const own = [...classes(p), ...(slot ? classes(slot) : [])];
    if (!isDialog(p)) return own;
    if (!upper(p)) return own;
    const box = modalClasses(literal(p, "size"), literal(p, "placement"));
    return [...own, ...box];
  };
  // The classes of every box from `n`'s parent up to and including the dialog.
  // A guest's own root ends the walk short of it, under a `.modal` all the same.
  const ancestry = (n: ts.Node) => {
    const out: string[][] = [];
    for (let p = enclosingV(n); p; p = enclosingV(p)) {
      out.push(boxOf(p));
      if (isDialog(p)) return out;
    }
    return [...out, ["modal"]];
  };
  const subset = (need: Set<string>, cls: string[]) =>
    [...need].every((c) => cls.includes(c));
  // The rule that sets `prop` on a box with these classes under this chain:
  // `!important` first, then the heavier, then the later.
  const resolve = (
    cls: string[],
    chain: string[][],
    prop: Prop,
    skip: (r: Placed) => boolean = (r) => r.place !== undefined,
  ) => {
    const applies = (r: Placed) =>
      [...r.subject].every((c) => cls.includes(c)) &&
      r.context.every((need, i) =>
        r.child && i === r.context.length - 1
          ? subset(need, chain[0] ?? [])
          : chain.some((a) => subset(need, a)),
      );
    const rank = (r: Placed) =>
      /!important/.test(r.props[prop] ?? "") ? 1 : 0;
    return [...cls, "*"]
      .flatMap((c) => own.placed.get(c) ?? [])
      .filter((r) => r.props[prop] !== undefined && !skip(r) && applies(r))
      .sort(
        (a, b) => rank(a) - rank(b) || a.weight - b.weight || a.order - b.order,
      )
      .at(-1);
  };
  // A gap class built as `ds-gap-${gap}` reads as every class so prefixed.
  const spaces = (built: string[], chain: string[][], across: boolean) => {
    const known = [...own.placed.keys()];
    const cls = built.flatMap((c) =>
      c.endsWith("-") ? known.filter((k) => k.startsWith(c)) : [c],
    );
    const get = (prop: Prop) =>
      (resolve(cls, chain, prop)?.props[prop] ?? "").replace("!important", "");
    const shown = get("display");
    const column = get("direction").trim() === "column";
    const flex = /flex/.test(shown);
    const down =
      (/grid/.test(shown) || (flex && column)) && !isZero(get("rowGap") || "0");
    const side = flex && !column && !isZero(get("columnGap") || "0");
    return down || (across && side);
  };
  // `across` counts a row's gap too; a margin doubles only a gap it stacks on.
  // A box with a conditional class stacks only if every branch does.
  const stacks = (parent: ts.Node, across = true) => {
    const chain = ancestry(parent);
    const all = (n: ts.Node) =>
      variantsOf(n).every((v) => spaces(v, chain, across));
    if (all(parent)) return true;
    const slot = upper(parent) && slotOf(parent);
    return !!slot && all(slot);
  };
  // What a row's own box can be: each branch of its classes, or of a
  // component's root elements' classes joined with the ones it is handed.
  const boxesOf = (n: ts.Node, hops = 3): string[][] => {
    const own = variantsOf(n);
    const def = defOf(n);
    if (!def || hops === 0 || isDialog(n)) return own;
    return rootsOf(def)
      .flatMap((r) => boxesOf(r, hops - 1))
      .flatMap((cls) => own.map((o) => [...cls, ...o]));
  };
  // The rule that margins `n` vertically inside its stack, if any. A rule
  // written for a child OF this stack already counts the gap.
  const margined = (n: ts.Node) => {
    const chain = ancestry(n);
    const parent = enclosingV(n);
    const peers = parent ? peersOf(n, parent) : [];
    const elsewhere = (r: Placed) =>
      (r.place === "first-child" && peers[0] !== n) ||
      (r.place === "last-child" && peers.at(-1) !== n);
    const edge = (cls: string[], side: "top" | "bottom") => {
      const won = resolve(cls, chain, side, elsewhere);
      const stack = won?.child ? won.context.at(-1) : undefined;
      const aware = !!stack && stack.size > 0 && subset(stack, chain[0]);
      return aware || isZero(won?.props[side] ?? "0") ? undefined : won;
    };
    const inFlow = (cls: string[]) => !cls.some((c) => own.loose.has(c));
    return boxesOf(n)
      .filter(inFlow)
      .flatMap((cls) => [edge(cls, "top"), edge(cls, "bottom")])
      .find((m) => m !== undefined)?.selector;
  };
  const has = (n: ts.Node | undefined, set: Set<string>) =>
    !!n &&
    opening(n) !== undefined &&
    variantsOf(n).every((v) => v.some((c) => set.has(c)));
  const ternaries = new Map<ts.SourceFile, ts.ConditionalExpression[]>();
  const exclusive = (a: ts.Node, b: ts.Node) => {
    const file = a.getSourceFile();
    const all =
      ternaries.get(file) ??
      descendants(file).filter(ts.isConditionalExpression);
    ternaries.set(file, all);
    return all.some(
      (c) =>
        (within(a, c.whenTrue) && within(b, c.whenFalse)) ||
        (within(a, c.whenFalse) && within(b, c.whenTrue)),
    );
  };
  const mapped = (k: ts.Node) =>
    ts.isFunctionLike(k) &&
    ts.isCallExpression(k.parent) &&
    /\.(flatMap|map)$/.test(k.parent.expression.getText());
  const content = (k: ts.Node, parent: ts.Node) => {
    if (opening(k)) return enclosingV(k) === parent;
    if (!childOfJsx(k) || enclosingV(k) !== parent) return false;
    if (ts.isJsxText(k)) return k.text.trim() !== "";
    return (
      ts.isJsxExpression(k) &&
      !!k.expression &&
      hostedAt(k).length === 0 &&
      !descendants(k).some(opening)
    );
  };
  const peersOf = (n: ts.Node, parent: ts.Node) =>
    vdesc(parent).filter(
      (k) => content(k, parent) && (k === n || !exclusive(k, n)),
    );
  const judge = (n: ts.Node, rows: number): Verdict => {
    const parent = enclosingV(n);
    if (!parent) return "spaced";
    if (stacks(parent)) {
      return stacks(parent, false) && margined(n) ? "doubled" : "spaced";
    }
    const peers = peersOf(n, parent);
    const at = peers.indexOf(n);
    const [prev, next] = [peers[at - 1], peers[at + 1]];
    const between: ts.Node[] = [];
    for (let k = up(n); k && k !== parent; k = up(k)) between.push(k);
    if (rows > 1 || between.some(mapped)) return "flush";
    if (!prev && !next && !isDialog(parent)) return judge(parent, 1);
    const above = !prev || has(prev, own.band) || has(n, own.top);
    const below = !next || has(next, own.top) || has(n, own.band);
    return above && below ? "spaced" : "flush";
  };

  const dialogs = elementsIn(source).filter(isDialog);
  const bodies = [...hosted].flatMap((h) => declarations(source).get(h) ?? []);
  const roots = [...(hosted.size > 0 ? [] : dialogs), ...bodies];
  const inside = new Set(roots.flatMap((r) => vdesc(r).filter(opening)));
  // JSX handed to a prop counts wherever the prop lands inside a dialog.
  const slotted = hosted.size > 0 ? [] : [...frameOf.keys()];
  for (const r of slotted) {
    links = frameOf.get(r) ?? new Map();
    let p = enclosingV(r);
    while (p && !isDialog(p)) p = enclosingV(p);
    if (p) for (const e of [r, ...vdesc(r)].filter(opening)) inside.add(e);
  }
  links = new Map();
  const guests: Guest[] = [];
  for (const n of inside) {
    const def = defOf(n);
    if (!def || isDialog(n) || isField(n)) continue;
    const file = def.getSourceFile().fileName;
    if (file !== source.fileName) guests.push({ file, name: definedName(def) });
    else for (const e of vdesc(def).filter(opening)) inside.add(e);
  }
  // The title's space is the title gate's to judge.
  const titled = localNames(source, "Heading");
  const rows: Row[] = [];
  for (const n of inside) {
    links = frameFor(n);
    const parent = enclosingV(n);
    const file = n.getSourceFile();
    const { line } = file.getLineAndCharacterOfPosition(n.getStart());
    const where = `${relative(srcDir, file.fileName)}:${line + 1}`;
    const at = `${file.fileName}:${n.getStart()}`;
    const count = width(n);
    if (count > 0 && unplacedRow(n)) {
      rows.push({ at, where, verdict: "unplaced" });
      continue;
    }
    if (!parent || unplacedRow(n)) continue;
    const verdict = count > 0 ? judge(n, count) : undefined;
    const by = stacks(parent, false) ? margined(n) : undefined;
    if (verdict) rows.push({ at, where, verdict, by });
    else if (by && !titled.has(tag(n)))
      rows.push({ at, where, verdict: "doubled", by });
  }
  links = new Map();
  return { dialogs: dialogs.length, rows, guests };
}

// A component a dialog renders is judged in its own file as part of that
// dialog, and so is what it renders in turn, until nothing new turns up.
function rowsAcross(
  entry: readonly ts.SourceFile[],
  own: Owners,
  keys: Set<string>,
) {
  const rows = new Map<string, Row>();
  const seen = new Set<string>();
  let dialogs = 0;
  let depth = 0;
  let queue: Guest[] = [];
  const take = (found: ReturnType<typeof rowsIn>) => {
    for (const r of found.rows) rows.set(r.at, r);
    queue.push(...found.guests);
  };
  for (const source of entry) {
    const found = rowsIn(source, own, keys);
    dialogs += found.dialogs;
    take(found);
  }
  while (queue.length > 0) {
    const fresh = queue.filter((g) => !seen.has(`${g.file}#${g.name}`));
    queue = [];
    if (fresh.length > 0) depth++;
    const byFile = new Map<string, Set<string>>();
    for (const g of fresh) {
      seen.add(`${g.file}#${g.name}`);
      byFile.set(g.file, (byFile.get(g.file) ?? new Set()).add(g.name));
    }
    for (const [file, names] of byFile) {
      take(rowsIn(sourceFileAt(file), own, keys, names));
    }
  }
  return { rows: [...rows.values()], dialogs, depth };
}

const owners = ownersIn(sheetTexts());
const files = markupFiles();
const sources = files.map((f) => sourceFileAt(f));
const modalDef = definition(
  sourceFileAt(join(srcDir, "design-system", "modal.tsx")),
  "Modal",
);
const seed = new Set(modalDef ? [keyOf(modalDef)] : []);
const keys = dialogKeys(sources, seed);
const tree = rowsAcross(sources, owners, keys);
// The same census read from the text, so a walk that misses a dialog shape
// counts fewer than the text does.
const byText = files.reduce((n, f, i) => {
  const text = readFileSync(f, "utf8")
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
  const names = new Set(
    [...text.matchAll(/<([A-Z][\w]*)\b/g)].map((m) => m[1]),
  );
  const dialogsNamed = [...names].filter((t) =>
    isDialogTag(sources[i], t, keys),
  );
  const tags = dialogsNamed.reduce(
    (sum, t) => sum + (text.match(new RegExp(`<${t}\\b`, "g"))?.length ?? 0),
    0,
  );
  return n + tags + (text.match(/\srole="dialog"/g)?.length ?? 0);
}, 0);

const AT = join(srcDir, "design-system", "planted.tsx");
const PRELUDE = `import { Modal } from "./modal";
import { ConfirmModal } from "./confirmmodal";
import { Stack } from "./stack";`;
const HELPERS = `function Rows() { return <><Field /><Field /></>; }
function Body() { return <div><p /><Field /></div>; }
function Inline() { return <Field labelHidden />; }
function Cited() { const cite = () => <i className="m" />; return <p>{cite()}</p>; }
function Bare() { const cite = () => <i className="m" />; return cite(); }
function Refusals() { return <div className="m" />; }
function Sheet({ children }) { return <Modal>{children}</Modal>; }
function Shell({ body }) { return <Modal><div>{body}</div></Modal>; }
function Framed({ body }) { return <div className="form-stack">{body}</div>; }
function Outer({ body }) { return <Framed body={body} />; }
function fields() { return <><p /><Field /></>; }
const held = <><p /><Field /></>;
export { Modal as Pane };`;
const plantedRows = (jsx: string, own = owners) => {
  const text = `${PRELUDE}\nconst A = () => (${jsx});\n${HELPERS}`;
  const source = parseSource(AT, text);
  const { rows } = rowsAcross([source], own, dialogKeys([source], keys));
  return rows.map((r) => r.verdict).join(" ") || "no rows";
};
const TT = '<Heading className="modal-title">A</Heading>';
const ACT = '<div className="actions" />';
const inModal = (inner: string) => `<Modal>${TT}${inner}${ACT}</Modal>`;
const STACK =
  ".s { display: flex; flex-direction: column; gap: var(--space-3) }";
const M = ".m { margin-top: var(--space-2) }";
const MS =
  '<Modal><div className="s"><Field /><Field className="m" /></div></Modal>';
const SCOPED =
  ".f .pb { display: flex; flex-direction: column; gap: var(--space-3) }";
const PS = '<div className="s"><p /><Field /></div>';
type RowCase = { jsx: string; verdict: string; sheet: string };

// Three files, so a guest's guest is two files away from its dialog.
function plantedAcross() {
  const dir = mkdtempSync(join(tmpdir(), "dialogrows-"));
  const modal = relative(dir, join(srcDir, "design-system", "modal"));
  writeFileSync(
    join(dir, "a.tsx"),
    `import { Modal } from "${modal}";\nimport { B } from "./b";\nexport const A = () => <Modal><B /></Modal>;\n`,
  );
  writeFileSync(
    join(dir, "b.tsx"),
    `import { C } from "./c";\nexport function B() { return <C />; }\n`,
  );
  writeFileSync(
    join(dir, "c.tsx"),
    "export function C() { return <div><p /><Field /></div>; }\n",
  );
  const { rows, depth } = rowsAcross(
    [sourceFileAt(join(dir, "a.tsx"))],
    owners,
    keys,
  );
  return { verdicts: rows.map((r) => r.verdict).join(" "), depth };
}

describe("a dialog's field rows have an owner for the space between them", () => {
  it("walks every dialog in the tree, wrappers included", () => {
    expect(tree.dialogs).toBe(byText);
    expect(keys.size).toBeGreaterThan(seed.size);
    expect(tree.rows.length).toBeGreaterThan(0);
    expect(tree.depth).toBeGreaterThan(1);
  });

  it("follows a guest's guest into a third file", () => {
    expect(plantedAcross()).toEqual({ verdicts: "flush", depth: 2 });
  });

  it.each`
    spec                                                        | jsx                                                                                                         | verdict                   | sheet
    ${"catches a field under a paragraph"}                      | ${inModal("<p /><Field />")}                                                                                | ${"flush"}                | ${""}
    ${"catches a ChoiceList over a Checkbox"}                   | ${inModal("<ChoiceList /><Checkbox />")}                                                                    | ${"flush flush"}          | ${""}
    ${"passes a lone field between title and actions"}          | ${inModal("<Field />")}                                                                                     | ${"spaced"}               | ${""}
    ${"passes fields in a .form-stack"}                         | ${inModal('<div className="form-stack"><p /><Field /><Field /></div>')}                                     | ${"spaced spaced"}        | ${""}
    ${"passes fields whose ConfirmModal stacks them"}           | ${"<ConfirmModal><p /><Field /><Field /></ConfirmModal>"}                                                   | ${"spaced spaced"}        | ${""}
    ${"reads through a fragment and a condition"}               | ${inModal("<>{a && <Field />}<Field /></>")}                                                                | ${"flush flush"}          | ${""}
    ${"reads one field a map repeats"}                          | ${inModal("<div>{xs.map((x) => <Field key={x} />)}</div>")}                                                 | ${"flush"}                | ${""}
    ${"lets two branches of a ternary stand alone"}             | ${inModal("{a ? <Field /> : <Field />}")}                                                                   | ${"spaced spaced"}        | ${""}
    ${"lifts a lone field out of a bare wrapper"}               | ${inModal("<p /><form><Field /></form>")}                                                                   | ${"flush"}                | ${""}
    ${"passes fields side by side in a .form-row"}              | ${inModal('<div className="form-stack"><div className="form-row"><Field /><Field /></div></div>')}          | ${"spaced spaced spaced"} | ${""}
    ${"follows a component that renders field rows"}            | ${inModal("<Rows />")}                                                                                      | ${"flush"}                | ${""}
    ${"passes that component inside a stack"}                   | ${"<ConfirmModal><Rows /></ConfirmModal>"}                                                                  | ${"spaced"}               | ${""}
    ${"catches a field margin that doubles the gap"}            | ${'<Modal><div className="s"><Field className="x" /><Field /></div></Modal>'}                               | ${"doubled spaced"}       | ${`${STACK} .x { margin-top: var(--space-3) }`}
    ${"reads the stack from the stylesheet"}                    | ${'<Modal><div className="s"><Field /><Field /></div></Modal>'}                                             | ${"flush flush"}          | ${".s { display: flex; flex-direction: column }"}
    ${"joins a stack and a gap spelled by two classes"}         | ${'<Modal><div className="c g"><Field /><Field /></div></Modal>'}                                           | ${"spaced spaced"}        | ${".c { display: flex; flex-direction: column } .g { gap: var(--space-3) }"}
    ${"reads the gap a Stack is given"}                         | ${inModal('<Stack gap="2"><p /><Field /></Stack>')}                                                         | ${"spaced"}               | ${""}
    ${"lifts a field out of a render prop"}                     | ${inModal("<p /><Gate>{() => <Field />}</Gate>")}                                                           | ${"flush"}                | ${""}
    ${"calls a row handed to a prop it cannot place"}           | ${inModal("<View slot={<><Field /><Field /></>} />")}                                                       | ${"unplaced unplaced"}    | ${""}
    ${"reads the class a component is handed"}                  | ${inModal('<Card className="s"><Field /><Field /></Card>')}                                                 | ${"spaced spaced"}        | ${STACK}
    ${"follows a component the dialog renders"}                 | ${inModal("<Body />")}                                                                                      | ${"flush"}                | ${""}
    ${"catches any margin a dialog stack's child adds"}         | ${inModal('<div className="s"><p /><div className="m" /></div>')}                                           | ${"doubled"}              | ${`${STACK} ${M}`}
    ${"catches the action row's margin inside a stack"}         | ${inModal('<div className="form-stack"><Field /><div className="actions" /></div>')}                        | ${"spaced doubled"}       | ${""}
    ${"catches a margin on a component's root"}                 | ${"<ConfirmModal><Refusals /></ConfirmModal>"}                                                              | ${"doubled"}              | ${`${STACK.replace(".s", ".form-stack")} ${M}`}
    ${"lets a heavier rule under the stack zero it"}            | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} ${M} .s > .m { margin-top: 0 }`}
    ${"lets a rule for a child of the stack count the gap"}     | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} ${M} .s > .m { margin-top: var(--space-1) }`}
    ${"ignores a margin set under another context"}             | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} .other .m { margin-top: var(--space-2) }`}
    ${"ignores an element out of flow"}                         | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} ${M} .m { position: absolute }`}
    ${"reads :last-child against the row's place"}              | ${'<Modal><div className="s"><Field className="m" /><Field className="m" /></div></Modal>'}                 | ${"doubled spaced"}       | ${`${STACK} ${M} .m:last-child { margin-top: 0 }`}
    ${"ignores a vertical margin in a row"}                     | ${'<Modal><div className="r"><Field className="m" /><Field /></div></Modal>'}                               | ${"spaced spaced"}        | ${`.r { display: flex; gap: var(--space-3) } ${M}`}
    ${"leaves the title to the title gate"}                     | ${'<Modal><div className="form-stack"><Heading className="modal-title">A</Heading><Field /></div></Modal>'} | ${"spaced"}               | ${""}
    ${"reads a stack scoped to another host as no stack"}       | ${'<Modal><div className="pb"><p /><Field /></div></Modal>'}                                                | ${"flush"}                | ${SCOPED}
    ${"reads that stack under its host"}                        | ${'<Modal><div className="f"><div className="pb"><p /><Field /></div></div></Modal>'}                       | ${"spaced"}               | ${SCOPED}
    ${"lets the dialog stack zero every row's margin"}          | ${MS}                                                                                                       | ${"spaced spaced"}        | ${`${STACK} ${M} .modal .s > * { margin-block: 0 }`}
    ${"lets a later two-class rule beat that zero"}             | ${MS}                                                                                                       | ${"spaced doubled"}       | ${`${STACK} .modal .s > * { margin-block: 0 } .modal .m { margin-top: var(--space-2) }`}
    ${"reads a component that wraps Modal as a dialog"}         | ${"<Sheet><p /><Field /></Sheet>"}                                                                          | ${"flush"}                | ${""}
    ${"reads Modal re-exported under another name"}             | ${"<Pane><p /><Field /></Pane>"}                                                                            | ${"flush"}                | ${""}
    ${"reads an element with role=dialog as a dialog"}          | ${'<div role="dialog"><p /><Field /></div>'}                                                                | ${"flush"}                | ${""}
    ${"reads rows held in a JSX variable"}                      | ${inModal("{held}")}                                                                                        | ${"flush"}                | ${""}
    ${"reads rows a function returns"}                          | ${inModal("{fields()}")}                                                                                    | ${"flush"}                | ${""}
    ${"places a prop's rows where the dialog puts them"}        | ${"<Shell body={<><p /><Field /></>} />"}                                                                   | ${"flush"}                | ${""}
    ${"places a prop's rows in the stack that holds them"}      | ${inModal("<Framed body={<><p /><Field /></>} />")}                                                         | ${"spaced"}               | ${""}
    ${"follows a prop handed on to another component"}          | ${inModal("<Outer body={<><p /><Field /></>} />")}                                                          | ${"spaced"}               | ${""}
    ${"reads the drawer's own box classes"}                     | ${`<Modal placement="right">${PS}</Modal>`}                                                                 | ${"spaced"}               | ${".modal-drawer .s { display: flex; flex-direction: column; gap: var(--space-3) }"}
    ${"ignores a rule that holds only under a query"}           | ${`<Modal>${PS}</Modal>`}                                                                                   | ${"spaced"}               | ${`${STACK} @media (min-width: 1px) { .s { display: block } }`}
    ${"lets !important outrank a heavier rule"}                 | ${`<Modal>${PS}</Modal>`}                                                                                   | ${"spaced"}               | ${".s { display: flex !important; flex-direction: column; gap: var(--space-3) } .modal .s { display: block }"}
    ${"stacks only if every branch of a class does"}            | ${'<Modal><div className={a ? "s" : "t"}><p /><Field /></div></Modal>'}                                     | ${"flush"}                | ${STACK}
    ${"doubles if any branch of a class margins"}               | ${'<Modal><div className="s"><Field /><Field className={a && "m"} /></div></Modal>'}                        | ${"spaced doubled"}       | ${`${STACK} ${M}`}
    ${"weighs :where() as nothing"}                             | ${MS}                                                                                                       | ${"spaced doubled"}       | ${`${STACK} .modal :where(.s) > * { margin-block: 0 } ${M}`}
    ${"leaves a field editing in place among text alone"}       | ${inModal("<div><span /><Field labelHidden /></div>")}                                                      | ${"no rows"}              | ${""}
    ${"still catches the same field with its label shown"}      | ${inModal("<div><span /><Field /></div>")}                                                                  | ${"flush"}                | ${""}
    ${"catches a field whose label shows in a branch"}          | ${inModal("<div><span /><Field labelHidden={a} /></div>")}                                                  | ${"flush"}                | ${""}
    ${"leaves a component editing in place alone"}              | ${inModal("<div><span /><Inline /></div>")}                                                                 | ${"no rows"}              | ${""}
    ${"keeps a local function's element where it is used"}      | ${"<ConfirmModal><Cited /></ConfirmModal>"}                                                                 | ${"no rows"}              | ${`${STACK.replace(".s", ".form-stack")} ${M}`}
    ${"roots a local function's element the component returns"} | ${"<ConfirmModal><Bare /></ConfirmModal>"}                                                                  | ${"doubled"}              | ${`${STACK.replace(".s", ".form-stack")} ${M}`}
  `("$spec", ({ jsx, verdict, sheet }: RowCase) => {
    expect(plantedRows(jsx, sheet ? ownersIn([sheet]) : owners)).toBe(verdict);
  });

  it("spaces every field row in the tree", () => {
    const loose = tree.rows.filter((r) => r.verdict !== "spaced");
    expect(
      loose.map((r) => `${r.where} ${r.verdict}${r.by ? ` by ${r.by}` : ""}`),
      "stack the rows in a .form-stack, and drop a margin the stack's gap already pays",
    ).toEqual([]);
  });
});
