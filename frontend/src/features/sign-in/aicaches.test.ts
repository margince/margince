import { readdirSync } from "node:fs";
import { join } from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  parseSource,
  resolveRelative,
  sourceFileAt,
} from "../../../scripts/lib/source-tree";
import { SCREEN_ROOTS } from "../../testing/screenfiles";
import { LANGUAGE_DEPENDENT_QUERY_PREFIXES } from "./aicaches";

/**
 * A list of caches to invalidate is a thing to forget to add to, and forgetting
 * is the whole failure it prevents: a new model-written surface would render
 * its old language after the setting changed, silently, on a page the admin is
 * looking at while they change it.
 *
 * So this derives the candidates from the tree instead. Every query key whose
 * name reads like a model-written surface must be named in the list, or say why
 * it is not one.
 */

// The screen folders, and lib/ where shared queries live.
const QUERY_ROOTS = [...SCREEN_ROOTS, "lib"];

/**
 * The names that mark a surface as model-written. Matched against the FIRST
 * segment of a query key, which is what identifies the resource — the segments
 * after it are ids.
 */
const MODEL_WRITTEN = /^(.*brief|.*dossier|.*growth-fit|deal-status)$/i;

/**
 * Query keys that match the name pattern and are NOT model-written prose, each
 * with the reason. A bare exclusion list would let somebody silence a real
 * finding by adding a line, so each entry has to say what the surface returns.
 */
const NOT_PROSE: Record<string, string> = {};

// Walks INTO subdirectories, because screens have them: record360/ and
// meetingbrief/ both hold components that query. A reader that stopped at the
// top level saw a smaller tree, reported PASS, and had no failing assertion to
// notice — which is the one way a census must not break.
function everyQueryKeyFirstSegment(
  dir: string,
  prefix: string,
): {
  key: string;
  file: string;
}[] {
  const found: { key: string; file: string }[] = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const name = entry.name;
    if (entry.isDirectory()) {
      found.push(
        ...everyQueryKeyFirstSegment(join(dir, name), `${prefix}${name}/`),
      );
      continue;
    }
    if (!name.endsWith(".tsx") && !name.endsWith(".ts")) continue;
    if (name.endsWith(".test.tsx") || name.endsWith(".test.ts")) continue;
    const source = sourceFileAt(join(dir, name));
    for (const key of queryPrefixes(source)) {
      found.push({ key, file: `${prefix}${name}` });
    }
  }
  return found;
}

type SourceReader = (path: string) => ts.SourceFile;
type ModuleResolver = (from: string, specifier: string) => string | null;

function queryPrefixes(
  source: ts.SourceFile,
  read: SourceReader = sourceFileAt,
  resolve: ModuleResolver = resolveRelative,
): string[] {
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    if (
      ts.isPropertyAssignment(node) &&
      node.name.getText(source) === "queryKey"
    ) {
      const key = queryPrefix(node.initializer, read, resolve, new Set());
      if (key !== null) found.push(key);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function queryPrefix(
  expression: ts.Expression,
  read: SourceReader,
  resolve: ModuleResolver,
  seen: Set<ts.Node>,
): string | null {
  if (seen.has(expression)) return null;
  seen.add(expression);
  if (
    ts.isAsExpression(expression) ||
    ts.isSatisfiesExpression(expression) ||
    ts.isParenthesizedExpression(expression)
  ) {
    return queryPrefix(expression.expression, read, resolve, seen);
  }
  if (ts.isArrayLiteralExpression(expression)) {
    const first = expression.elements[0];
    if (!first) return null;
    if (ts.isStringLiteralLike(first)) return first.text;
    if (ts.isSpreadElement(first))
      return queryPrefix(first.expression, read, resolve, seen);
    return null;
  }
  if (!ts.isIdentifier(expression)) return null;
  const bound = queryKeyBinding(expression.text, expression, read, resolve);
  return bound ? queryPrefix(bound, read, resolve, seen) : null;
}

function queryKeyBinding(
  name: string,
  at: ts.Node,
  read: SourceReader,
  resolve: ModuleResolver,
): ts.Expression | null {
  for (
    let scope: ts.Node | undefined = at.parent;
    scope;
    scope = scope.parent
  ) {
    if (
      ts.isFunctionLike(scope) &&
      scope.parameters.some((parameter) => bindsName(parameter.name, name))
    )
      return null;
    if (
      ts.isCatchClause(scope) &&
      scope.variableDeclaration &&
      bindsName(scope.variableDeclaration.name, name)
    )
      return null;
    if (
      ts.isForStatement(scope) ||
      ts.isForOfStatement(scope) ||
      ts.isForInStatement(scope)
    ) {
      const declarations = scope.initializer;
      if (
        declarations &&
        ts.isVariableDeclarationList(declarations) &&
        declarations.declarations.some((entry) => bindsName(entry.name, name))
      )
        return null;
    }
    if (!ts.isBlock(scope) && !ts.isSourceFile(scope)) continue;
    for (const statement of scope.statements) {
      if (!ts.isVariableStatement(statement)) continue;
      const declaration = statement.declarationList.declarations.find((entry) =>
        bindsName(entry.name, name),
      );
      if (declaration)
        return ts.isIdentifier(declaration.name) &&
          statement.declarationList.flags & ts.NodeFlags.Const
          ? (declaration.initializer ?? null)
          : null;
    }
    if (ts.isSourceFile(scope)) return importedKey(name, scope, read, resolve);
  }
  return null;
}

function bindsName(binding: ts.BindingName, name: string): boolean {
  if (ts.isIdentifier(binding)) return binding.text === name;
  return binding.elements.some(
    (element) =>
      !ts.isOmittedExpression(element) && bindsName(element.name, name),
  );
}

function exportedKey(
  source: ts.SourceFile,
  name: string,
): ts.Expression | null {
  for (const statement of source.statements) {
    if (
      ts.isVariableStatement(statement) &&
      statement.modifiers?.some(
        (modifier) => modifier.kind === ts.SyntaxKind.ExportKeyword,
      )
    ) {
      const value = constantNamed(statement, name);
      if (value) return value;
    }
    if (
      !ts.isExportDeclaration(statement) ||
      statement.isTypeOnly ||
      statement.moduleSpecifier ||
      !statement.exportClause ||
      !ts.isNamedExports(statement.exportClause)
    )
      continue;
    const exported = statement.exportClause.elements.find(
      (entry) => !entry.isTypeOnly && entry.name.text === name,
    );
    if (!exported) continue;
    const local = exported.propertyName?.text ?? exported.name.text;
    for (const declaration of source.statements) {
      if (!ts.isVariableStatement(declaration)) continue;
      const value = constantNamed(declaration, local);
      if (value) return value;
    }
  }
  return null;
}

function constantNamed(
  statement: ts.VariableStatement,
  name: string,
): ts.Expression | null {
  if (!(statement.declarationList.flags & ts.NodeFlags.Const)) return null;
  const declaration = statement.declarationList.declarations.find(
    (entry) => ts.isIdentifier(entry.name) && entry.name.text === name,
  );
  return declaration?.initializer ?? null;
}

function importedKey(
  name: string,
  source: ts.SourceFile,
  read: SourceReader,
  resolve: ModuleResolver,
): ts.Expression | null {
  for (const statement of source.statements) {
    if (
      !ts.isImportDeclaration(statement) ||
      !ts.isStringLiteralLike(statement.moduleSpecifier)
    )
      continue;
    const bindings = statement.importClause?.namedBindings;
    if (!bindings || !ts.isNamedImports(bindings)) continue;
    const binding = bindings.elements.find((entry) => entry.name.text === name);
    if (!binding) continue;
    const path = resolve(source.fileName, statement.moduleSpecifier.text);
    if (!path) return null;
    const owner = read(path);
    const exported = binding.propertyName?.text ?? binding.name.text;
    return exportedKey(owner, exported);
  }
  return null;
}

function everyScreenQueryKey(): { key: string; file: string }[] {
  return QUERY_ROOTS.flatMap((root) =>
    everyQueryKeyFirstSegment(join(__dirname, "../..", root), `${root}/`),
  );
}

describe("the language-dependent cache list", () => {
  it("names every model-written surface the screens query", () => {
    const named = new Set<string>(
      LANGUAGE_DEPENDENT_QUERY_PREFIXES.map((prefix) => prefix[0]),
    );
    const missing = everyScreenQueryKey().filter(
      ({ key }) =>
        MODEL_WRITTEN.test(key) && !named.has(key) && !(key in NOT_PROSE),
    );

    expect(
      missing,
      `these query keys look like model-written surfaces and are not in LANGUAGE_DEPENDENT_QUERY_PREFIXES, ` +
        `so changing the installation's base language would leave them rendering the old one: ` +
        missing.map((m) => `${m.key} (${m.file})`).join(", "),
    ).toEqual([]);
  });

  it("does not name a surface the screens never query", () => {
    // The other direction. A prefix nobody queries invalidates nothing, and
    // reads as coverage that is not there — which is worse than an obviously
    // short list, because it looks complete.
    const queried = new Set(everyScreenQueryKey().map(({ key }) => key));
    const stale = LANGUAGE_DEPENDENT_QUERY_PREFIXES.map(
      (prefix) => prefix[0],
    ).filter((key) => !queried.has(key));

    expect(
      stale,
      `these prefixes are invalidated but no screen queries them: ${stale.join(", ")}`,
    ).toEqual([]);
  });
});

describe("query-prefix census syntax", () => {
  it("resolves literal, imported alias, local alias and spread tuple keys from their owner", () => {
    const files = new Map([
      [
        "/census/keys.ts",
        parseSource(
          "/census/keys.ts",
          `export const briefKey = ["brief"] as const; export const unused = ["unusedBrief"] as const;`,
        ),
      ],
    ]);
    const source = parseSource(
      "/census/screen.ts",
      `
      import { briefKey as imported } from "./keys";
      const local = imported;
      useQuery({ queryKey: ["literalBrief", id] });
      useQuery({ queryKey: imported });
      useQuery({ queryKey: local });
      useQuery({ queryKey: [...local, id] });
      const unused = ["inventedBrief"] as const;
      function shadow(imported) { useQuery({ queryKey: imported }); }
      useQuery({ queryKey: missing });
    `,
    );
    const read = (path: string) => {
      const file = files.get(path);
      if (!file) throw new Error(`Missing fixture module: ${path}`);
      return file;
    };
    expect(
      queryPrefixes(source, read, (from, specifier) =>
        resolveRelative(from, specifier, (path) => files.has(path)),
      ),
    ).toEqual(["literalBrief", "brief", "brief", "brief"]);
  });
});

it("does not borrow an outer tuple through destructuring, loop or catch shadows", () => {
  const source = parseSource(
    "/census/shadows.ts",
    `
    const key = ["outerBrief"] as const;
    function parameter({ key }) { useQuery({ queryKey: key }); }
    function nested({ value: { key } }) { useQuery({ queryKey: key }); }
    function array([key]) { useQuery({ queryKey: key }); }
    { const { key } = options; useQuery({ queryKey: key }); }
    { const [key] = options; useQuery({ queryKey: key }); }
    for (const key of keys) { useQuery({ queryKey: key }); }
    for (const key in keys) { useQuery({ queryKey: key }); }
    for (let key = start; key; key = next) { useQuery({ queryKey: key }); }
    try {} catch (key) { useQuery({ queryKey: key }); }
    try {} catch ({ key }) { useQuery({ queryKey: key }); }
    useQuery({ queryKey: key });
  `,
  );
  expect(queryPrefixes(source)).toEqual(["outerBrief"]);
});

it("follows the actual export alias rather than a same-named private constant", () => {
  const owner = parseSource(
    "/census/exports.ts",
    `
    const briefKey = ["privateBrief"] as const;
    const privateOnly = ["hiddenBrief"] as const;
    const actual = ["publicBrief"] as const;
    export { actual as briefKey };
  `,
  );
  const source = parseSource(
    "/census/consumer.ts",
    `
    import { briefKey as key, privateOnly } from "./exports";
    useQuery({ queryKey: key });
    useQuery({ queryKey: privateOnly });
  `,
  );
  expect(
    queryPrefixes(
      source,
      () => owner,
      (from, specifier) =>
        resolveRelative(from, specifier, (path) => path === owner.fileName),
    ),
  ).toEqual(["publicBrief"]);
});
