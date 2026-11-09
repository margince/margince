// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A whole-tree census is a rule in census/rules/. A test file elsewhere that
// imports a tree walker parses the tree again in its own module graph.
//
// The import is matched in the syntax tree by the exported name, so an alias
// counts. A walk the file writes itself counts too, and so does a walk a
// helper module does for it. The rule has no waiver list.

import { join, relative, sep } from "node:path";
import ts from "typescript";
import { isModuleOnDisk, resolveRelative } from "../../scripts/lib/source-tree";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  frontendRoot,
  scriptsDir,
  srcDir,
} from "../tree";

const lib = (name: string) => join(scriptsDir, "lib", name);

const walker = (
  module: string,
  ...names: string[]
): [string, ReadonlySet<string>] => [module, new Set(names)];

// Each module that walks the tree, and the exports that do the walking.
export const WALKERS: ReadonlyMap<string, ReadonlySet<string>> = new Map([
  walker(
    lib("source-tree.ts"),
    "filesUnder",
    "filesMatching",
    "extensionFilesMatching",
    "extensionFrontendFiles",
  ),
  walker(join(srcDir, "testing", "screenfiles.ts"), "screenFiles"),
  walker(
    lib("conformance-corpus.ts"),
    "sourceFiles",
    "extensionFrontends",
    "files",
  ),
  walker(lib("css-rules.ts"), "appStylesheets"),
  walker(lib("dialoglayout.corpus.ts"), "markupFiles", "sheetTexts"),
  walker(lib("dialoglayout.ts"), "markupFiles", "sheetTexts"),
  walker(lib("import-reach.ts"), "productionModulesUnder"),
  walker(lib("bundle-reach.ts"), "reachedModules", "documentEntries"),
  walker(lib("story-files.ts"), "storyCensus"),
  walker(lib("essential-copy.ts"), "essentialKeys"),
  walker(
    join(scriptsDir, "ext-imports.ts"),
    "auditLayers",
    "layersThatReadNothing",
  ),
  walker(
    join(scriptsDir, "feature-boundaries.ts"),
    "readGraph",
    "shippedInTree",
  ),
]);

// Tests of a walker itself: each walks a temporary directory it plants, not
// the tree, so it costs no parse of the tree. `findsTheTree` holds that.
const WALKER_TESTS: ReadonlySet<string> = new Set([
  "scripts/ext-imports.test.ts",
  "scripts/feature-boundaries.test.ts",
  "scripts/lib/essential-copy.test.ts",
  "scripts/lib/import-reach.test.ts",
  "scripts/lib/source-tree.test.ts",
  "scripts/lib/story-files.test.ts",
]);

const TEST_FILE = /\.test\.(ts|tsx|mts|cts|js|jsx|mjs|cjs)$/;

const shown = (module: string) =>
  relative(frontendRoot, module).split(sep).join("/");

function importedWalkers(
  module: string,
  bindings: ts.NamedImportBindings,
): string[] {
  if (ts.isNamespaceImport(bindings)) {
    return [`imports all of ${shown(module)}`];
  }
  return bindings.elements
    .filter((element) => !element.isTypeOnly)
    .map((element) => (element.propertyName ?? element.name).text)
    .filter((name) => WALKERS.get(module)?.has(name))
    .map((name) => `imports ${name} from ${shown(module)}`);
}

/**
 * The walker imports in one test file, one line each. A namespace import or an
 * `import()` of a walker module reaches every walker in it, so it counts.
 */
export function strayImports(path: string, source: ts.SourceFile): string[] {
  const found: string[] = [];
  const walkerAt = (specifier: string) => {
    const module = resolveRelative(path, specifier);
    return module !== null && WALKERS.has(module) ? module : undefined;
  };
  const visit = (node: ts.Node) => {
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier) &&
      node.importClause?.phaseModifier !== ts.SyntaxKind.TypeKeyword
    ) {
      const module = walkerAt(node.moduleSpecifier.text);
      const bindings = node.importClause?.namedBindings;
      if (module && bindings) found.push(...importedWalkers(module, bindings));
    }
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments[0] &&
      ts.isStringLiteralLike(node.arguments[0])
    ) {
      const module = walkerAt(node.arguments[0].text);
      if (module) found.push(`imports all of ${shown(module)}`);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const calleeName = (call: ts.CallExpression): string | undefined =>
  ts.isIdentifier(call.expression)
    ? call.expression.text
    : ts.isPropertyAccessExpression(call.expression)
      ? call.expression.name.text
      : undefined;

function callsIn(node: ts.Node): string[] {
  const names: string[] = [];
  const visit = (child: ts.Node) => {
    if (ts.isCallExpression(child)) names.push(calleeName(child) ?? "");
    ts.forEachChild(child, visit);
  };
  ts.forEachChild(node, visit);
  return names;
}

function declaredFunction(node: ts.Node): string | undefined {
  if (ts.isFunctionDeclaration(node)) return node.name?.text;
  if (
    ts.isVariableDeclaration(node) &&
    ts.isIdentifier(node.name) &&
    node.initializer &&
    (ts.isArrowFunction(node.initializer) ||
      ts.isFunctionExpression(node.initializer))
  ) {
    return node.name.text;
  }
  return undefined;
}

// The local names `readdirSync` goes by in this file, aliases included.
function readdirNames(source: ts.SourceFile): Set<string> {
  const names = new Set(["readdirSync"]);
  for (const statement of source.statements) {
    const bindings = ts.isImportDeclaration(statement)
      ? statement.importClause?.namedBindings
      : undefined;
    if (!bindings || !ts.isNamedImports(bindings)) continue;
    for (const element of bindings.elements) {
      if ((element.propertyName ?? element.name).text === "readdirSync") {
        names.add(element.name.text);
      }
    }
  }
  return names;
}

const recurses = (argument: ts.Expression) =>
  ts.isObjectLiteralExpression(argument) &&
  argument.properties.some(
    (property) =>
      ts.isPropertyAssignment(property) &&
      property.name.getText() === "recursive" &&
      property.initializer.kind === ts.SyntaxKind.TrueKeyword,
  );

const isMetaGlob = (call: ts.CallExpression) =>
  ts.isPropertyAccessExpression(call.expression) &&
  call.expression.name.text === "glob" &&
  ts.isMetaProperty(call.expression.expression);

const globsDeep = (argument: ts.Expression): boolean =>
  ts.isStringLiteralLike(argument)
    ? argument.text.includes("**")
    : ts.isArrayLiteralExpression(argument) &&
      argument.elements.some((element) => globsDeep(element));

/**
 * A walk a test file writes for itself: a function that calls `readdirSync`
 * and itself, a `readdirSync` asked to recurse, or an `import.meta.glob` over
 * `**`.
 */
export function ownWalks(
  root: ts.Node,
  { outsideFunctions = false } = {},
): string[] {
  const readdir = readdirNames(root.getSourceFile());
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    const name = outsideFunctions ? undefined : declaredFunction(node);
    const calls = name ? callsIn(node) : [];
    if (name && calls.includes(name) && calls.some((c) => readdir.has(c))) {
      found.push(`walks the tree itself in ${name}()`);
    }
    if (
      ts.isCallExpression(node) &&
      !(outsideFunctions && ts.findAncestor(node.parent, ts.isFunctionLike))
    ) {
      if (
        readdir.has(calleeName(node) ?? "") &&
        node.arguments.some(recurses)
      ) {
        found.push("walks the tree itself with a recursive readdirSync");
      }
      if (isMetaGlob(node) && node.arguments.some(globsDeep)) {
        found.push("walks the tree itself with import.meta.glob over **");
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(root);
  return found;
}

/** Each `import()` in a module whose name is not a literal. Such a name can
 * be a walker's, and only running the module says which. */
export function computedImports(source: ts.SourceFile): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments[0] &&
      !ts.isStringLiteralLike(node.arguments[0])
    ) {
      found.push(
        `imports a module whose name is computed: import(${node.arguments[0].getText(source)})`,
      );
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const walks = (file: CensusFile) =>
  file.source
    ? [
        ...strayImports(file.path, file.source),
        ...computedImports(file.source),
        ...ownWalks(file.source),
      ]
    : [];

type Imported = Readonly<{
  path: string;
  /** The imported file as the tree holds it, if it does. */
  module: CensusFile | undefined;
  /** Each imported name and the local name it binds; `*` for a namespace. */
  names: Map<string, string>;
}>;

// Each module's imports, read once a run: helpers are shared by many tests.
const importsRead = new WeakMap<CensusFile, Imported[]>();

/** The non-type imports of one module by a relative specifier. */
function localImports(file: CensusFile, tree: CensusTree): Imported[] {
  const known = importsRead.get(file);
  if (known) return known;
  const found = readImports(file, tree);
  importsRead.set(file, found);
  return found;
}

function readImports(file: CensusFile, tree: CensusTree): Imported[] {
  if (!file.source) return [];
  const out: Imported[] = [];
  for (const statement of file.source.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteral(statement.moduleSpecifier) ||
      statement.importClause?.phaseModifier === ts.SyntaxKind.TypeKeyword
    ) {
      continue;
    }
    const path = resolveRelative(
      file.path,
      statement.moduleSpecifier.text,
      (candidate) =>
        tree.at(candidate) !== undefined || isModuleOnDisk(candidate),
    );
    if (path === null) continue;
    const names = new Map<string, string>();
    const bindings = statement.importClause?.namedBindings;
    if (bindings && ts.isNamedImports(bindings)) {
      for (const element of bindings.elements) {
        if (element.isTypeOnly) continue;
        names.set(
          (element.propertyName ?? element.name).text,
          element.name.text,
        );
      }
    }
    if (bindings && ts.isNamespaceImport(bindings)) {
      names.set("*", bindings.name.text);
    }
    out.push({ path, module: tree.at(path), names });
  }
  // A literal `import()` reaches the whole module, as a namespace import does.
  const visit = (node: ts.Node) => {
    if (
      ts.isCallExpression(node) &&
      node.expression.kind === ts.SyntaxKind.ImportKeyword &&
      node.arguments[0] &&
      ts.isStringLiteralLike(node.arguments[0])
    ) {
      const path = resolveRelative(
        file.path,
        node.arguments[0].text,
        (candidate) =>
          tree.at(candidate) !== undefined || isModuleOnDisk(candidate),
      );
      if (path !== null) {
        out.push({ path, module: tree.at(path), names: new Map([["*", ""]]) });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(file.source);
  return out;
}

const enclosingFunction = (node: ts.Node): ts.Node | undefined =>
  ts.findAncestor(node.parent, ts.isFunctionLike);

const exported = (node: ts.Node) =>
  (ts.getCombinedModifierFlags(node as ts.Declaration) &
    ts.ModifierFlags.Export) !==
  0;

/** The function a declaration holds, with its name and whether the module
 * exports it. */
function functionOf(
  node: ts.Node,
): { name: string; body: ts.Node; exported: boolean } | undefined {
  if (ts.isFunctionDeclaration(node) && node.name && node.body) {
    return { name: node.name.text, body: node, exported: exported(node) };
  }
  if (
    ts.isVariableDeclaration(node) &&
    ts.isIdentifier(node.name) &&
    node.initializer &&
    (ts.isArrowFunction(node.initializer) ||
      ts.isFunctionExpression(node.initializer))
  ) {
    return { name: node.name.text, body: node, exported: exported(node) };
  }
  return undefined;
}

const calleeRoot = (call: ts.CallExpression): string | undefined => {
  const callee = ts.isPropertyAccessExpression(call.expression)
    ? call.expression.expression
    : call.expression;
  return ts.isIdentifier(callee) ? callee.text : undefined;
};

/** How a module walks the tree: as it loads, and through which exports. */
type Walking = Readonly<{
  onLoad: string | null;
  exports: ReadonlyMap<string, string>;
}>;

type Classified = Map<CensusFile, Walking>;

const NONE: Walking = { onLoad: null, exports: new Map() };

/** The names a module binds that walk the tree when called, with why. */
function walkingImports(
  module: CensusFile,
  tree: CensusTree,
  known: Classified,
): { names: Map<string, string>; onLoad: string | null } {
  const names = new Map<string, string>();
  let onLoad: string | null = null;
  for (const { path, module: imported, names: bound } of localImports(
    module,
    tree,
  )) {
    const walking = WALKERS.get(path);
    if (walking) {
      for (const [name, local] of bound) {
        if (name === "*" || walking.has(name)) {
          names.set(local, `${name} from ${shown(path)}`);
        }
      }
      continue;
    }
    if (!imported || TEST_FILE.test(path)) continue;
    const helper = classify(imported, tree, known);
    onLoad ??= helper.onLoad && `imports ${helper.onLoad}`;
    for (const [name, local] of bound) {
      const why =
        name === "*"
          ? [...helper.exports.values()][0]
          : helper.exports.get(name);
      if (why && local !== "") names.set(local, why);
    }
  }
  return { names, onLoad };
}

/**
 * How one non-test module walks. A function walks when it writes its own walk
 * or calls a name that walks. That name is a walker, a walking export of
 * another module, or another function here that walks. The module walks as it
 * loads when such a call runs outside any function. It also does when it
 * imports a module that walks as it loads, or a module by a computed name.
 */
function classify(
  module: CensusFile,
  tree: CensusTree,
  known: Classified,
): Walking {
  const cached = known.get(module);
  if (cached) return cached;
  known.set(module, NONE);
  const source = module.source;
  if (!source) return NONE;
  const imported = walkingImports(module, tree, known);
  const walking = new Map(imported.names);
  const functions = [...descendantsOf(source)].flatMap(
    (node) => functionOf(node) ?? [],
  );
  for (const fn of functions) {
    const own = ownWalks(fn.body)[0];
    if (own) walking.set(fn.name, `${module.rel} ${own}`);
  }
  for (let grew = true; grew; ) {
    grew = false;
    for (const fn of functions) {
      if (walking.has(fn.name)) continue;
      const called = rootsCalledIn(fn.body).find((name) => walking.has(name));
      if (called) {
        walking.set(
          fn.name,
          `${fn.name}() in ${module.rel}, which calls ${walking.get(called)}`,
        );
        grew = true;
      }
    }
  }
  const loading = [...descendantsOf(source)].find(
    (node) =>
      ts.isCallExpression(node) &&
      enclosingFunction(node) === undefined &&
      !onlyAsAScript(node) &&
      walking.has(calleeRoot(node) ?? ""),
  );
  const topWalk = ownWalks(source, { outsideFunctions: true })[0];
  const computed = computedImports(source)[0];
  const onLoad =
    (loading &&
      `${module.rel}, which calls ${walking.get(calleeRoot(loading as ts.CallExpression) ?? "")} when it loads`) ||
    (topWalk && `${module.rel}, which ${topWalk} when it loads`) ||
    (computed && `${module.rel}, which ${computed}`) ||
    (imported.onLoad && `${module.rel}, which ${imported.onLoad}`) ||
    null;
  const exports = new Map(
    functions
      .filter((fn) => fn.exported && walking.has(fn.name))
      .map((fn) => [fn.name, walking.get(fn.name) ?? ""]),
  );
  const found: Walking = { onLoad, exports };
  known.set(module, found);
  return found;
}

/** Whether a call runs only when the module is run as a script, under
 * `if (process.argv[1] === …)`, and so never when a test imports it. */
const onlyAsAScript = (node: ts.Node): boolean =>
  ts.findAncestor(
    node.parent,
    (at) =>
      ts.isIfStatement(at) && at.expression.getText().includes("process.argv"),
  ) !== undefined;

/** The root name of every call inside a node: `walk` for `walk()`, `ns` for
 * `ns.walk()`. */
const rootsCalledIn = (node: ts.Node): string[] =>
  [...descendantsOf(node)].flatMap((child) =>
    ts.isCallExpression(child) ? (calleeRoot(child) ?? []) : [],
  );

function* descendantsOf(node: ts.Node): Generator<ts.Node> {
  const stack: ts.Node[] = [node];
  for (let at = stack.pop(); at; at = stack.pop()) {
    yield at;
    ts.forEachChild(at, (child) => {
      stack.push(child);
    });
  }
}

/**
 * The walks a test reaches through the non-test modules it imports: one that
 * walks as it loads, or a function of one that walks. A walker module itself
 * counts only for the walking names a test imports, which `walks` reads.
 */
function helperWalks(
  file: CensusFile,
  tree: CensusTree,
  known: Classified,
): string[] {
  return localImports(file, tree).flatMap(({ path, module, names }) => {
    if (!module || WALKERS.has(path) || TEST_FILE.test(path)) return [];
    const helper = classify(module, tree, known);
    if (helper.onLoad) return [`walks the tree through ${helper.onLoad}`];
    return [...names.keys()].flatMap((name) => {
      const why =
        name === "*"
          ? [...helper.exports.values()][0]
          : helper.exports.get(name);
      return why ? [`walks the tree through ${why}`] : [];
    });
  });
}

/**
 * The ways a module finds the real tree: its own location, the working
 * directory, or a root the census names. A walker's own test that walks only
 * what it plants has none of them.
 */
export function findsTheTree(source: ts.SourceFile): string[] {
  const found = new Set<string>();
  const visit = (node: ts.Node) => {
    if (
      ts.isPropertyAccessExpression(node) &&
      ts.isMetaProperty(node.expression) &&
      node.name.text === "url"
    ) {
      found.add("import.meta.url");
    }
    if (
      ts.isIdentifier(node) &&
      ["__dirname", "__filename"].includes(node.text)
    ) {
      found.add(node.text);
    }
    if (
      ts.isCallExpression(node) &&
      node.expression.getText(source) === "process.cwd"
    ) {
      found.add("process.cwd()");
    }
    if (
      ts.isImportDeclaration(node) &&
      ts.isStringLiteral(node.moduleSpecifier) &&
      /(^|\/)census\/tree(\.ts)?$/.test(node.moduleSpecifier.text)
    ) {
      found.add("census/tree.ts");
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return [...found];
}

const strays = (file: CensusFile, tree: CensusTree, known: Classified) => [
  ...walks(file),
  ...helperWalks(file, tree, known),
];

export const rule: CensusRule = {
  name: "no-stray-census",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        TEST_FILE.test(file.path) &&
        !WALKER_TESTS.has(file.rel) &&
        !file.rel.startsWith("census/"),
    ),
  floors: (corpus, tree) => [
    ...(corpus.length > 500 ? [] : ["fewer than 500 test files were read"]),
    ...[...WALKER_TESTS]
      .filter((rel) => !tree.at(join(frontendRoot, rel)))
      .map((rel) => `${rel} is listed as a walker's own test and is gone`),
  ],
  across: (corpus, tree) => {
    const known: Classified = new Map();
    const realTree = tree.files
      .filter((file) => WALKER_TESTS.has(file.rel) && file.source)
      .flatMap((file) =>
        findsTheTree(file.source as ts.SourceFile).map(
          (how) =>
            `${file.rel} is listed as a walker's own test but finds the real tree through ${how}`,
        ),
      );
    return [
      ...realTree,
      ...corpus.flatMap((file) =>
        strays(file, tree, known).map((what) => `${file.rel}: ${what}`),
      ),
    ];
  },
  advice:
    "A test file outside census/ walks the tree. Move the census into a rule in " +
    "census/rules/<name>.census.ts and keep its planted cases beside it in " +
    "census/rules/<name>.test.ts; census/census.test.ts runs it over the one parsed tree.",
};
