import { join, relative } from "node:path";
import ts from "typescript";
import { markupFiles, srcDir } from "./dialoglayout.corpus";
import type { Owners } from "./dialoglayout.css";
import {
  childrenHosts,
  declarations,
  defOf,
  descendants,
  keyOf,
  literalTypes,
  presetOf,
  primitiveKey,
} from "./dialoglayout.tags";
import { sourceFileAt } from "./source-tree";

export { markupFiles, sheetTexts, srcDir } from "./dialoglayout.corpus";
export {
  flowOf,
  isZero,
  type Owners,
  ownersIn,
  type Placed,
  type Prop,
  weightOf,
} from "./dialoglayout.css";
export {
  attr,
  attrs,
  callOf,
  childrenHosts,
  childrenSlot,
  classes,
  declarations,
  definedName,
  definition,
  defOf,
  descendants,
  domainOf,
  elementsIn,
  enclosing,
  fnOf,
  givenTo,
  handedJsx,
  idParts,
  initOf,
  jsxOf,
  keyOf,
  literalsIn,
  type Member,
  opening,
  presetOf,
  primitiveKey,
  propReadBy,
  rendered,
  returnedBy,
  rootsOf,
  spreads,
  tag,
  upper,
  variantsOf,
  within,
} from "./dialoglayout.tags";

export const isDialogTag = (element: ts.Node, keys: ReadonlySet<string>) => {
  const def = defOf(element);
  return !!def && keys.has(keyOf(def));
};

// A component that renders a dialog around its children is a dialog, to a
// fixed point from `Modal`.
function dialogKeys(
  files: readonly ts.SourceFile[],
  seed: Iterable<string>,
): Set<string> {
  const keys = new Set(seed);
  const candidates = files.flatMap((file) =>
    [...declarations(file)]
      .filter(([name]) => /^[A-Z]/.test(name))
      .map(([, def]) => ({ def, around: childrenHosts(def) }))
      .filter((c) => c.around.length > 0),
  );
  for (let grew = true; grew; ) {
    grew = false;
    for (const { def, around } of candidates) {
      if (keys.has(keyOf(def))) continue;
      if (around.some((e) => isDialogTag(e, keys))) {
        keys.add(keyOf(def));
        grew = true;
      }
    }
  }
  return keys;
}

export type DialogSet = { modal: string; keys: Set<string> };
let treeDialogs: DialogSet | undefined;

/** `Modal`'s key and every dialog key: over the tree, cached, or over
 * `files` grown from the keys already known in `from`. */
export function dialogSet(
  files?: readonly ts.SourceFile[],
  from?: DialogSet,
): DialogSet {
  if (!files && treeDialogs) return treeDialogs;
  const modal = primitiveKey("modal", "Modal");
  const read = files ?? markupFiles().map((f) => sourceFileAt(f));
  const set = { modal, keys: dialogKeys(read, from?.keys ?? [modal]) };
  if (!files) treeDialogs = set;
  return set;
}

const DECLARATION =
  /^(?:export\s+(?:default\s+)?)?(?:function\s+([A-Z]\w*)|const\s+([A-Z]\w*)\s*(?::[^=]*)?=)/gm;

function declaredIn(text: string) {
  const starts = [...text.matchAll(DECLARATION)];
  return starts.map((m, i) => ({
    name: m[1] ?? m[2],
    text: text.slice(m.index, starts[i + 1]?.index ?? text.length),
  }));
}

const wrapsChildren = (text: string, name: string) =>
  [...text.matchAll(new RegExp(`<${name}\\b[\\s\\S]*?</${name}>`, "g"))].some(
    (m) => /\{\s*(?:props\.)?children\s*\}/.test(m[0]),
  );

const aliases = (text: string, names: Set<string>) => [
  ...[...text.matchAll(/\b(\w+) as (\w+)\b/g)]
    .filter((m) => names.has(m[1]))
    .map((m) => m[2]),
  ...[...text.matchAll(/\bconst\s+(\w+)\s*(?::[^=]*)?=\s*(\w+)\s*;/g)]
    .filter((m) => names.has(m[2]))
    .map((m) => m[1]),
];

// A name a file can spell as a tag: one it declares, imports or renames to.
const spells = (text: string, name: string) =>
  new RegExp(
    `\\b(?:function|const)\\s+${name}\\b|import\\s*\\{[^}]*\\b${name}\\b[^}]*\\}|\\bas\\s+${name}\\b`,
  ).test(text);

// Not after a name: `useState<Ask>` is a type argument, not a tag.
const tagsOf = (text: string, names: string[]) =>
  names.length === 0
    ? 0
    : (text.match(new RegExp(`(?<![\\w$.])<(?:${names.join("|")})\\b`, "g"))
        ?.length ?? 0);

// A second count of the dialogs, read from text without the syntax tree, so a
// walk that misses one disagrees with it.
export function textCensus(texts: readonly string[]) {
  const files = texts.map((t) => {
    const text = t
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .replace(/(^|\s)\/\/.*$/gm, "$1");
    return { text, seen: new Map<string, boolean>() };
  });
  const visible = (file: (typeof files)[number], name: string) => {
    const known = file.seen.get(name) ?? spells(file.text, name);
    file.seen.set(name, known);
    return known;
  };
  const decls = files.flatMap((file) =>
    declaredIn(file.text).map((d) => ({ ...d, file })),
  );
  const wraps = (d: (typeof decls)[number], names: Set<string>) =>
    [...names].some((n) => visible(d.file, n) && wrapsChildren(d.text, n));
  const grow = (names: Set<string>, wrappers: boolean) => {
    for (let size = 0; size !== names.size; ) {
      size = names.size;
      const renamed = files.flatMap((f) => aliases(f.text, names));
      const wrapping = wrappers ? decls.filter((d) => wraps(d, names)) : [];
      for (const n of [...renamed, ...wrapping.map((d) => d.name)]) {
        names.add(n);
      }
    }
    return names;
  };
  const modal = grow(new Set(["Modal"]), false);
  const names = grow(new Set(modal), true);
  const count = (set: Set<string>) =>
    files.reduce(
      (n, f) =>
        n +
        tagsOf(
          f.text,
          [...set].filter((s) => visible(f, s)),
        ),
      0,
    );
  const roles = files.reduce(
    (n, f) =>
      n + (f.text.match(/\srole=(?:"dialog"|\{"dialog"\})/g)?.length ?? 0),
    0,
  );
  return { modals: count(modal), dialogs: count(names) + roles, names };
}

type Args = ReadonlyMap<string, string | undefined>;

const MODAL_FILE = join(srcDir, "design-system", "modal.tsx");

const unread = (n: ts.Node, what: string) =>
  new Error(
    `${n.getSourceFile().fileName}: modalClass ${what} \`${n.getText()}\`, which the dialog box reader does not evaluate; extend evaluate() in dialoglayout.ts`,
  );

const COMPARE = new Map([
  [ts.SyntaxKind.EqualsEqualsEqualsToken, true],
  [ts.SyntaxKind.ExclamationEqualsEqualsToken, false],
]);

function evaluate(e: ts.Expression, args: Args): string | boolean | undefined {
  if (ts.isParenthesizedExpression(e)) return evaluate(e.expression, args);
  if (ts.isStringLiteral(e)) return e.text;
  if (ts.isIdentifier(e)) {
    if (!args.has(e.text)) throw unread(e, "reads");
    return args.get(e.text);
  }
  if (ts.isConditionalExpression(e)) {
    const pick = evaluate(e.condition, args) ? e.whenTrue : e.whenFalse;
    return evaluate(pick, args);
  }
  const equal = ts.isBinaryExpression(e)
    ? COMPARE.get(e.operatorToken.kind)
    : undefined;
  if (!ts.isBinaryExpression(e) || equal === undefined) {
    throw unread(e, "is written in");
  }
  return (evaluate(e.left, args) === evaluate(e.right, args)) === equal;
}

const statementsOf = (s: ts.Statement | undefined) => {
  if (!s) return [];
  return ts.isBlock(s) ? s.statements : [s];
};

function execute(
  statements: readonly ts.Statement[],
  args: Args,
): string | undefined {
  for (const s of statements) {
    if (ts.isReturnStatement(s) && s.expression) {
      const out = evaluate(s.expression, args);
      if (typeof out !== "string") throw unread(s, "returns a non-string from");
      return out;
    }
    if (!ts.isIfStatement(s)) throw unread(s, "runs");
    const taken = evaluate(s.expression, args);
    const out = execute(
      statementsOf(taken ? s.thenStatement : s.elseStatement),
      args,
    );
    if (out !== undefined) return out;
  }
  return undefined;
}

// Each of modalClass's parameters, the Modal prop the call site hands it, the
// values its type admits (`undefined` for an optional prop) and its default.
type Param = {
  name: string;
  prop: string;
  domain: (string | undefined)[];
  preset?: string;
};

const literalsOf = (type: ts.TypeNode | undefined) => {
  const members = type && ts.isUnionTypeNode(type) ? type.types : [type];
  const each = members.map((m) =>
    m?.kind === ts.SyntaxKind.UndefinedKeyword ? [undefined] : literalTypes(m),
  );
  return each.every((m) => m) ? each.flatMap((m) => m ?? []) : [];
};

type Shape = { fn: ts.FunctionDeclaration; params: Param[] };

const shapes = new Map<string, Shape>();

function shapeOfModal(path: string): Shape {
  const known = shapes.get(path);
  if (known) return known;
  const file = sourceFileAt(path);
  const fn = declarations(file).get("modalClass");
  const modal = declarations(file).get("Modal");
  const call =
    modal &&
    descendants(modal).find(
      (n): n is ts.CallExpression =>
        ts.isCallExpression(n) && n.expression.getText() === "modalClass",
    );
  if (!fn || !ts.isFunctionDeclaration(fn) || !fn.body || !modal || !call) {
    throw new Error(
      `${path}: Modal no longer draws its box with className={modalClass(…)}; teach shapeOfModal() in dialoglayout.ts the new shape`,
    );
  }
  const params = fn.parameters.map((p, i): Param => {
    const arg = call.arguments[i];
    const domain = literalsOf(p.type);
    if (!arg || !ts.isIdentifier(arg) || domain.length === 0) {
      throw unread(
        p,
        "takes, other than a Modal prop typed as string literals,",
      );
    }
    const preset = presetOf(modal, arg.text);
    return { name: p.name.getText(), prop: arg.text, domain, preset };
  });
  shapes.set(path, { fn, params });
  return { fn, params };
}

/** The Modal props whose value decides the box classes. */
export const boxProps = (path = MODAL_FILE) =>
  shapeOfModal(path).params.map((p) => p.prop);

/** A prop's possible values: listed, `undefined` for unset, or any it admits. */
export type Values = readonly (string | undefined)[] | "any";

/** The box classes `Modal` draws for each combination of the values given,
 * read off `modalClass` and the props' defaults in modal.tsx. */
export function modalBoxes(
  given: ReadonlyMap<string, Values>,
  path = MODAL_FILE,
): string[][] {
  const { fn, params } = shapeOfModal(path);
  const choices = params.map((p) => {
    const v = given.get(p.prop) ?? [undefined];
    return (v === "any" ? p.domain : v).map((x) => x ?? p.preset);
  });
  const combos = choices.reduce<(string | undefined)[][]>(
    (all, values) => all.flatMap((a) => values.map((v) => [...a, v])),
    [[]],
  );
  const boxes = combos.map((combo) => {
    const args = new Map(params.map((p, i) => [p.name, combo[i]]));
    const out = execute(fn.body?.statements ?? [], args);
    if (!out) throw unread(fn, `returns no class for ${combo.join("/")} in`);
    return out;
  });
  // Combinations that draw the same box are judged once.
  return [...new Set(boxes)].map((out) => out.split(/\s+/));
}

const CAP = 64;

export const whereOf = (n: ts.Node) => {
  const file = n.getSourceFile();
  const { line } = file.getLineAndCharacterOfPosition(n.getStart());
  return `${relative(srcDir, file.fileName)}:${line + 1}`;
};

function capped<T>(all: T[], at: ts.Node): T[] {
  if (all.length <= CAP) return all;
  throw new Error(
    `${whereOf(at)}: more than ${CAP} class combinations to judge; name the branches so fewer combine`,
  );
}

export const cross = <T>(a: T[][], b: T[][], at: ts.Node) =>
  capped(
    a.flatMap((x) => b.map((y) => [...x, ...y])),
    at,
  );

/** Ancestor chains cut to the classes some rule needs on an ancestor, each
 * chain once: a class no context names cannot change which rule applies. */
export function contextOf(owners: Owners, chains: string[][][]) {
  const cut = chains.map((chain) =>
    chain.map((box) => box.filter((c) => owners.context.has(c))),
  );
  return [...new Map(cut.map((c) => [JSON.stringify(c), c])).values()];
}

const escaped = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// A pattern class (`ds-gap-*`) reads as each class it can be, one at a time.
export function expanded(
  owners: Owners,
  cls: string[],
  at: ts.Node,
): string[][] {
  const known = [...owners.placed.keys()];
  return cls.reduce<string[][]>(
    (all, c) => {
      if (!c.includes("*")) return all.map((a) => [...a, c]);
      const shape = new RegExp(
        `^${c.split("*").map(escaped).join("[\\w-]*")}$`,
      );
      const hits = known.filter((k) => shape.test(k)).map((k) => [k]);
      return hits.length > 0 ? cross(all, hits, at) : all;
    },
    [[]],
  );
}
