// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A screen held past a move still runs its effects and late callbacks, and a
// module-level writer there rewrites the entry the reader arrived on.
//
// Not caught: a writer behind a prop, a member call, or a callback handed to a
// component or hook, such as ClosesWithPage's `close` (guarded at runtime).

import { join } from "node:path";
import ts from "typescript";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";

const URLSTATE = join(srcDir, "lib", "urlstate.ts");
const ROUTER = join(srcDir, "lib", "router.tsx");
const EFFECTS = new Set([
  "useEffect",
  "useLayoutEffect",
  "useInsertionEffect",
  "useEffectEvent",
]);
const KEPT_CALLBACKS = new Set(["useCallback", "useEffectEvent"]);
const LATE_PROPERTIES = new Set(["onSuccess", "onSettled", "onError"]);
const LATE_METHODS = new Set(["then", "catch", "finally"]);

const isProductSource = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.rel) &&
  !/\.(test|stories)\.tsx?$/.test(file.rel) &&
  !file.rel.endsWith(".d.ts");

type Writers = Readonly<{
  byFunction: ReadonlyMap<ts.Symbol, string>;
  floors: readonly string[];
}>;

const unaliased = (checker: ts.TypeChecker, symbol: ts.Symbol | undefined) =>
  symbol && symbol.flags & ts.SymbolFlags.Alias
    ? checker.getAliasedSymbol(symbol)
    : symbol;

function exportsOf(program: ts.Program, path: string): ts.Symbol[] {
  const checker = program.getTypeChecker();
  const source = program.getSourceFile(path);
  const module = source && checker.getSymbolAtLocation(source);
  return module ? checker.getExportsOfModule(module) : [];
}

/** The module-level writers, named by the keys of `AddressWriters`. */
export function addressWriters(program: ts.Program): Writers {
  const checker = program.getTypeChecker();
  const shape = exportsOf(program, URLSTATE).find(
    (symbol) => symbol.name === "AddressWriters",
  );
  if (!shape) {
    return {
      byFunction: new Map(),
      floors: ["lib/urlstate.ts exports no AddressWriters to name the writers"],
    };
  }
  const exported = [
    ...exportsOf(program, URLSTATE),
    ...exportsOf(program, ROUTER),
  ].map((symbol) => unaliased(checker, symbol));
  const byFunction = new Map<ts.Symbol, string>();
  const floors: string[] = [];
  for (const key of checker.getDeclaredTypeOfSymbol(shape).getProperties()) {
    const writer = exported.find(
      (symbol) =>
        symbol?.name === key.name &&
        symbol.declarations?.some(ts.isFunctionDeclaration),
    );
    if (writer) byFunction.set(writer, key.name);
    else
      floors.push(
        `\`${key.name}\` is no exported function of lib/urlstate.ts or lib/router.tsx`,
      );
  }
  return { byFunction, floors };
}

// A function written where it is called (a callback, an IIFE) runs with the
// code around it. One returned or stored runs later, at someone else's call.
function runsWhereWritten(fn: ts.Node): boolean {
  let at = fn;
  while (ts.isParenthesizedExpression(at.parent)) at = at.parent;
  const parent = at.parent;
  const argument = (arg: ts.Node) => arg === at;
  return (
    (ts.isCallExpression(parent) &&
      (parent.expression === at || parent.arguments.some(argument))) ||
    (ts.isNewExpression(parent) && (parent.arguments?.some(argument) ?? false))
  );
}

const isFunction = (node: ts.Node): boolean =>
  ts.isArrowFunction(node) ||
  ts.isFunctionExpression(node) ||
  ts.isFunctionDeclaration(node) ||
  ts.isMethodDeclaration(node);

function judge(program: ts.Program, writers: Writers) {
  const checker = program.getTypeChecker();
  const writerNames: ReadonlySet<string> = new Set(writers.byFunction.values());
  // A member name costs its object's type, so only a writer's name is asked.
  const symbolAt = (id: ts.Identifier, names = writerNames) => {
    if (
      ts.isPropertyAccessExpression(id.parent) &&
      id.parent.name === id &&
      !names.has(id.text)
    )
      return undefined;
    return unaliased(
      checker,
      ts.isShorthandPropertyAssignment(id.parent) && id.parent.name === id
        ? checker.getShorthandAssignmentValueSymbol(id.parent)
        : checker.getSymbolAtLocation(id),
    );
  };
  const named = (call: ts.CallExpression, names: ReadonlySet<string>) => {
    const callee = ts.isPropertyAccessExpression(call.expression)
      ? call.expression.name
      : call.expression;
    const symbol = ts.isIdentifier(callee)
      ? symbolAt(callee, names)
      : undefined;
    return (
      symbol !== undefined &&
      names.has(symbol.name) &&
      (symbol.declarations ?? []).some((declaration) =>
        /\/node_modules\/(@types\/)?react\//.test(
          declaration.getSourceFile().fileName,
        ),
      )
    );
  };

  // The body a name runs when called, for a function this tree declares. A
  // component or hook named in late code is drawn or called by React instead.
  const bodyOf = (symbol: ts.Symbol | undefined): ts.Node | undefined => {
    const declaration = symbol?.declarations?.[0];
    if (
      !declaration?.getSourceFile().fileName.startsWith(srcDir) ||
      /^(use)?[A-Z]/.test(symbol?.name ?? "")
    )
      return undefined;
    if (ts.isFunctionDeclaration(declaration)) return declaration.body;
    if (!ts.isVariableDeclaration(declaration)) return undefined;
    const init = declaration.initializer;
    if (init && (ts.isArrowFunction(init) || ts.isFunctionExpression(init)))
      return init;
    if (init && ts.isCallExpression(init) && named(init, KEPT_CALLBACKS)) {
      const kept = init.arguments[0];
      return kept && isFunction(kept) ? kept : undefined;
    }
    return undefined;
  };

  const reached = new Map<ts.Symbol, string | undefined>();
  const reachOf = (symbol: ts.Symbol | undefined): string | undefined => {
    if (!symbol) return undefined;
    const writer = writers.byFunction.get(symbol);
    if (writer) return writer;
    if (reached.has(symbol)) return reached.get(symbol);
    reached.set(symbol, undefined);
    const body = bodyOf(symbol);
    let found: string | undefined;
    if (body) {
      walk(body, false, (id) => {
        found ??= reachOf(symbolAt(id));
      });
    }
    reached.set(symbol, found);
    return found;
  };

  // Every name the code at `root` uses. `whole` takes in the functions it only
  // writes down, which a host's own code runs later from the same place.
  function walk(
    root: ts.Node,
    whole: boolean,
    onName: (id: ts.Identifier) => void,
  ): void {
    const visit = (node: ts.Node) => {
      if (
        node !== root &&
        isFunction(node) &&
        !whole &&
        !runsWhereWritten(node)
      )
        return;
      if (ts.isIdentifier(node)) onName(node);
      ts.forEachChild(node, visit);
    };
    visit(root);
  }

  // The code that runs after its render: effect bodies, settled requests, and
  // whatever follows an `await`.
  function hosts(source: ts.SourceFile): ts.Node[] {
    const found: ts.Node[] = [];
    const take = (node: ts.Node | undefined) => {
      if (!node) return;
      if (isFunction(node)) found.push(node);
      else if (ts.isIdentifier(node)) {
        const body = bodyOf(symbolAt(node));
        if (body) found.push(body);
      }
    };
    const visit = (node: ts.Node) => {
      if (ts.isCallExpression(node)) {
        if (named(node, EFFECTS)) take(node.arguments[0]);
        if (
          ts.isPropertyAccessExpression(node.expression) &&
          LATE_METHODS.has(node.expression.name.text)
        )
          node.arguments.forEach(take);
      }
      if (
        (ts.isPropertyAssignment(node) || ts.isMethodDeclaration(node)) &&
        ts.isIdentifier(node.name) &&
        LATE_PROPERTIES.has(node.name.text)
      )
        take(ts.isPropertyAssignment(node) ? node.initializer : node);
      if (ts.isBlock(node)) {
        let awaited = false;
        for (const statement of node.statements) {
          if (awaited) found.push(statement);
          awaited ||= awaits(statement);
        }
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
    return found;
  }

  return (file: CensusFile): string[] => {
    const source = program.getSourceFile(file.path);
    if (!source) return [`${file.rel} is not in the type-checked program`];
    const lines = new Map<number, string>();
    for (const host of hosts(source)) {
      walk(host, true, (id) => {
        const writer = reachOf(symbolAt(id));
        if (!writer) return;
        const line =
          source.getLineAndCharacterOfPosition(id.getStart()).line + 1;
        const through = id.text === writer ? "" : ` through \`${id.text}\``;
        lines.set(
          line,
          `${file.rel}:${line} — runs the module-level \`${writer}\`${through} after its render; take it from useAddressWriters()`,
        );
      });
    }
    return [...lines.values()];
  };
}

function awaits(node: ts.Node): boolean {
  if (ts.isAwaitExpression(node)) return true;
  if (isFunction(node)) return false;
  return ts.forEachChild(node, awaits) ?? false;
}

export const rule: CensusRule = {
  name: "effect-address-writers",
  corpus: (tree) => tree.files.filter(isProductSource),
  floors: (_corpus, tree: CensusTree) => addressWriters(tree.checked()).floors,
  across: (corpus, tree: CensusTree) => {
    const program = tree.checked();
    const findingsOf = judge(program, addressWriters(program));
    return corpus.flatMap(findingsOf);
  },
  advice:
    "Code that runs after its render (an effect, a settled request, the rest " +
    "after an await) writes the address through useAddressWriters(), which " +
    "drops the write once the bar has left the screen's page.",
};
