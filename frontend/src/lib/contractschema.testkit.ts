import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { sourceFileAt } from "../../scripts/lib/source-tree";

// Reads the generated contract types as syntax, for the censuses that derive
// their corpus from the contract rather than from a list kept beside it.

const schemaPath = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "api",
  "schema.d.ts",
);

export function schemaSource(): ts.SourceFile {
  return sourceFileAt(schemaPath);
}

// The property of one named type in the contract, wherever it is declared.
export function propertyOf(
  source: ts.SourceFile,
  owner: string,
  property: string,
): ts.PropertySignature | undefined {
  let found: ts.PropertySignature | undefined;
  const walk = (node: ts.Node) => {
    if (
      ts.isPropertySignature(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === owner &&
      node.type
    ) {
      for (const member of membersOf(node.type)) {
        if (ts.isIdentifier(member.name) && member.name.text === property) {
          found = member;
        }
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return found;
}

export function membersOf(node: ts.TypeNode): ts.PropertySignature[] {
  if (ts.isIntersectionTypeNode(node)) {
    return node.types.flatMap(membersOf);
  }
  if (!ts.isTypeLiteralNode(node)) {
    return [];
  }
  return node.members.filter(ts.isPropertySignature);
}

/** The members of every type literal declared under `owner`, intersections included. */
export function typeMembers(
  source: ts.SourceFile,
  owner: string,
): ts.PropertySignature[] {
  const found: ts.PropertySignature[] = [];
  const walk = (node: ts.Node) => {
    if (
      ts.isPropertySignature(node) &&
      ts.isIdentifier(node.name) &&
      node.name.text === owner &&
      node.type
    ) {
      found.push(...membersOf(node.type));
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return found;
}

function stringCodes(node: ts.TypeNode): string[] | undefined {
  if (ts.isParenthesizedTypeNode(node)) return stringCodes(node.type);
  if (ts.isLiteralTypeNode(node) && ts.isStringLiteral(node.literal)) {
    return [node.literal.text];
  }
  if (!ts.isUnionTypeNode(node)) return undefined;
  const parts = node.types
    .filter(
      (member) =>
        !(
          ts.isLiteralTypeNode(member) &&
          member.literal.kind === ts.SyntaxKind.NullKeyword
        ),
    )
    .map(stringCodes);
  return parts.every((part) => part !== undefined) ? parts.flat() : undefined;
}

/**
 * The codes a property is limited to, or undefined when it is open text. A
 * reference to a named schema is followed to the schema's own codes.
 */
export function enumCodes(
  source: ts.SourceFile,
  member: ts.PropertySignature,
): string[] | undefined {
  const type = member.type;
  if (!type) return undefined;
  const direct = stringCodes(type);
  if (direct) return direct;
  if (
    ts.isIndexedAccessTypeNode(type) &&
    ts.isLiteralTypeNode(type.indexType) &&
    ts.isStringLiteral(type.indexType.literal)
  ) {
    const named = propertyOf(source, "schemas", type.indexType.literal.text);
    return named?.type ? stringCodes(named.type) : undefined;
  }
  return undefined;
}
