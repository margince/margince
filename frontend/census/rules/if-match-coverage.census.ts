// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A write to an endpoint that takes an `If-Match` precondition sends one.
// Without it, two users moving one deal at once both succeed, and the second
// replaces a change its author never saw. Which endpoints take it comes out of
// `schema.d.ts`, so a new versioned endpoint is enrolled with no list to extend.
//
// Not caught: a call whose path is an expression (`api.POST(path, …)`). The
// approvals inbox is the one such caller, and its endpoints declare no
// `If-Match`, since the server re-admits the edited payload from scratch.

import { join } from "node:path";
import ts from "typescript";
import { stable } from "../../src/format/collate";
import {
  type CensusFile,
  type CensusRule,
  type CensusTree,
  srcDir,
} from "../tree";

export const schemaPath = join(srcDir, "api", "schema.d.ts");

// The writes that do not pin the row they overwrite yet, as a path from src/
// and the endpoint. A new unpinned write is a finding, and so is an entry
// whose write now pins its row, so the list only falls.
export const UNPINNED_WRITES: readonly string[] = [
  // A proposal's list entry carries no version to pin; withdrawing archives it,
  // so two withdrawals racing end in the one state either would reach alone.
  "features/contacts/contactmeetings.waiting.tsx DELETE /activities/{id}",
  // Relationship archives still need their record version threaded through.
  "features/contacts/contactemployers.tsx DELETE /relationships/{id}",
  "components/relationshiprows.tsx DELETE /relationships/{id}",
  "features/projects/contractwrites.ts PATCH /contracts/{id}",
  "features/records/customfields.tsx PATCH /custom-fields/{id}",
  "features/voice-knowledge/voice-dna.tsx PATCH /voice-profiles/{id}",
];

const HTTP_METHODS: readonly string[] = [
  "get",
  "put",
  "post",
  "patch",
  "delete",
];

function declaration(
  schemaSource: ts.SourceFile,
  name: string,
): ts.InterfaceDeclaration {
  let found: ts.InterfaceDeclaration | undefined;
  schemaSource.forEachChild((node) => {
    if (ts.isInterfaceDeclaration(node) && node.name.text === name) {
      found = node;
    }
  });
  if (!found) {
    throw new Error(`schema.d.ts declares no \`${name}\` interface`);
  }
  return found;
}

function memberName(member: ts.TypeElement): string | null {
  const name = member.name;
  if (!name) {
    return null;
  }
  return ts.isIdentifier(name) || ts.isStringLiteral(name) ? name.text : null;
}

function memberType(
  member: ts.TypeElement | undefined,
): ts.TypeNode | undefined {
  return member && ts.isPropertySignature(member) ? member.type : undefined;
}

/** The members of `<member>: { … }`, and none where it is not an object type. */
function fields(member: ts.TypeElement | undefined): readonly ts.TypeElement[] {
  const type = memberType(member);
  return type && ts.isTypeLiteralNode(type) ? type.members : [];
}

function field(
  member: ts.TypeElement | undefined,
  name: string,
): ts.TypeElement | undefined {
  return fields(member).find((inner) => memberName(inner) === name);
}

/** The operation ids whose contract declares the `If-Match` request header. */
function conditionalOperations(schema: ts.SourceFile): ReadonlySet<string> {
  const ids = new Set<string>();
  for (const operation of declaration(schema, "operations").members) {
    const name = memberName(operation);
    const header = field(field(operation, "parameters"), "header");
    if (name && field(header, "If-Match")) {
      ids.add(name);
    }
  }
  return ids;
}

/** `METHOD /path` for every route whose operation takes the precondition. */
export function conditionalEndpoints(
  schema: ts.SourceFile,
): ReadonlySet<string> {
  const operations = conditionalOperations(schema);
  const endpoints = new Set<string>();
  for (const route of declaration(schema, "paths").members) {
    const path = memberName(route);
    for (const verb of fields(route)) {
      const method = memberName(verb);
      const type = memberType(verb);
      if (!path || !method || !type || !ts.isIndexedAccessTypeNode(type)) {
        continue;
      }
      const index = type.indexType;
      if (!ts.isLiteralTypeNode(index) || !ts.isStringLiteral(index.literal)) {
        continue;
      }
      if (operations.has(index.literal.text)) {
        endpoints.add(`${method.toUpperCase()} ${path}`);
      }
    }
  }
  return endpoints;
}

/**
 * `METHOD /path` when this node is an `api.<METHOD>("<literal>", …)` call that
 * spreads no `ifMatch(…)` into its parameters, and null otherwise.
 */
function calledEndpoint(node: ts.Node): string | null {
  if (!ts.isCallExpression(node) || node.arguments.length === 0) {
    return null;
  }
  const callee = node.expression;
  if (
    !ts.isPropertyAccessExpression(callee) ||
    !ts.isIdentifier(callee.expression) ||
    callee.expression.text !== "api"
  ) {
    return null;
  }
  const [path] = node.arguments;
  const method = callee.name.text;
  if (
    !ts.isStringLiteral(path) ||
    !HTTP_METHODS.includes(method.toLowerCase())
  ) {
    return null;
  }
  return spreadsPrecondition(node.arguments[1])
    ? null
    : `${method.toUpperCase()} ${path.text}`;
}

/**
 * Whether a call's options object spreads a precondition into `params`.
 *
 * Read off the syntax tree, so a comment or string naming `ifMatch(` cannot
 * stand in for the header. The spread is searched, not required to be the call.
 * The partner upsert picks a precondition or none at the spread, since its
 * create arm has no prior row to pin.
 */
function spreadsPrecondition(options: ts.Expression | undefined): boolean {
  if (options === undefined || !ts.isObjectLiteralExpression(options)) {
    return false;
  }
  const params = options.properties.find(
    (property): property is ts.PropertyAssignment =>
      ts.isPropertyAssignment(property) && property.name.getText() === "params",
  );
  if (
    params === undefined ||
    !ts.isObjectLiteralExpression(params.initializer)
  ) {
    return false;
  }
  return params.initializer.properties.some(
    (property) =>
      ts.isSpreadAssignment(property) && callsIfMatch(property.expression),
  );
}

/** Whether an expression contains a call to `ifMatch`. */
function callsIfMatch(node: ts.Node): boolean {
  if (
    ts.isCallExpression(node) &&
    ts.isIdentifier(node.expression) &&
    node.expression.text === "ifMatch"
  ) {
    return true;
  }
  return node.getChildren().some(callsIfMatch);
}

/** `<file> METHOD /path` for every call in one file that sends no precondition. */
function unpinnedIn(
  file: CensusFile,
  conditional: ReadonlySet<string>,
): string[] {
  const source = file.source;
  if (!source) return [];
  const shown = file.rel.replace(/^src\//, "");
  const found: string[] = [];
  const visit = (node: ts.Node) => {
    const endpoint = calledEndpoint(node);
    if (endpoint && conditional.has(endpoint)) {
      found.push(`${shown} ${endpoint}`);
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

const conditionalIn = (tree: CensusTree): ReadonlySet<string> => {
  const schema = tree.at(schemaPath)?.source;
  return schema ? conditionalEndpoints(schema) : new Set();
};

/** The unlisted unpinned writes, then the listed writes that now pin. */
export function writeFindings(
  corpus: readonly CensusFile[],
  tree: CensusTree,
  listed: readonly string[],
): string[] {
  const conditional = conditionalIn(tree);
  const found = corpus
    .flatMap((file) => unpinnedIn(file, conditional))
    .sort(stable);
  const unlisted = found.filter((write) => !listed.includes(write));
  const fixed = listed
    .filter((write) => !found.includes(write))
    .map(
      (write) => `${write} now sends If-Match; delete its UNPINNED_WRITES line`,
    );
  return [...unlisted, ...fixed];
}

export const rule: CensusRule = {
  name: "if-match-coverage",
  // A file that never names the client cannot call it.
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        file.area === "src" &&
        /\.tsx?$/.test(file.path) &&
        file.text.includes("api."),
    ),
  // A contract that yields no endpoints compares nothing and looks green.
  floors: (_corpus, tree) =>
    conditionalIn(tree).size > 0
      ? []
      : ["schema.d.ts yielded no endpoint that takes If-Match"],
  across: (corpus, tree) => writeFindings(corpus, tree, UNPINNED_WRITES),
  advice:
    "A write to an endpoint that takes If-Match sends none. Spread " +
    "ifMatch(version) into its params, so a concurrent change answers 409 " +
    "instead of being overwritten.",
};
