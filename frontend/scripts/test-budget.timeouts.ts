import ts from "typescript";

/**
 * Whether a property key names the timeout, however it is written. A source
 * text comparison would read `{ "timeout": 5000 }` and `{ ["timeout"]: 5000 }`
 * as "states no timeout", the silent-default path this file closes.
 */
function isTimeoutKey(name: ts.PropertyName): boolean {
  if (ts.isIdentifier(name) || ts.isStringLiteralLike(name)) {
    return name.text === "timeout";
  }
  if (
    ts.isComputedPropertyName(name) &&
    ts.isStringLiteralLike(name.expression)
  ) {
    return name.expression.text === "timeout";
  }
  return false;
}

/**
 * The `timeout:` of one options object: `{ timeout: 4000 }` and the shorthand
 * `{ timeout }` alike, since the shorthand is the idiomatic spelling once the
 * value has a name. A spread carries properties this reader cannot see, so the
 * whole object is reported unreadable rather than searched and found wanting.
 */
export function timeoutExpression(
  options: ts.ObjectLiteralExpression,
): ts.Expression | typeof OPAQUE | undefined {
  for (const property of options.properties) {
    if (ts.isSpreadAssignment(property)) return OPAQUE;
    if (ts.isShorthandPropertyAssignment(property)) {
      if (property.name.text === "timeout") return property.name;
      continue;
    }
    if (!ts.isPropertyAssignment(property)) continue;
    if (!isTimeoutKey(property.name)) continue;
    return property.initializer;
  }
  return undefined;
}

/**
 * The `timeout:` a waiter states, wherever among its arguments it sits, or
 * OPAQUE when an argument might carry one this reader cannot read. Only
 * arguments after the first are considered: a waiter's first argument is its
 * matcher or callback, and an options object never sits there.
 */
export function statedTimeout(
  call: ts.CallExpression,
): ts.Expression | typeof OPAQUE | undefined {
  let found: ts.Expression | typeof OPAQUE | undefined;
  for (const argument of call.arguments.slice(1)) {
    if (cannotCarryOptions(argument)) continue;
    if (!ts.isObjectLiteralExpression(argument)) return OPAQUE;
    const stated = timeoutExpression(argument);
    if (stated === OPAQUE) return OPAQUE;
    if (stated !== undefined && found === undefined) found = stated;
  }
  return found;
}

/**
 * An options argument this reader cannot see inside: a spread, or an object
 * handed over by name. "States no timeout" means a default budget; this means
 * a budget nobody checked. Merged, an 11000ms test would read as 4000.
 */
export const OPAQUE = Symbol("an options argument this reader cannot read");

/**
 * Shapes that cannot be an options object carrying a timeout: a matcher, a
 * placeholder, a callback. Everything else may carry one, because the default
 * has to be "unreadable", not "absent".
 *
 * A whitelist of understood shapes would send every other shape down the
 * "states no timeout" path. That path ends at the 1000ms default, the smallest
 * number, which keeps the gate green. Property access, a call, a ternary, an
 * `as` cast and a `!` assertion would all reach it.
 */
function cannotCarryOptions(argument: ts.Expression): boolean {
  if (ts.isIdentifier(argument)) return argument.text === "undefined";
  switch (argument.kind) {
    case ts.SyntaxKind.NullKeyword:
    case ts.SyntaxKind.TrueKeyword:
    case ts.SyntaxKind.FalseKeyword:
      return true;
    default:
      break;
  }
  return (
    ts.isStringLiteralLike(argument) ||
    ts.isNumericLiteral(argument) ||
    ts.isRegularExpressionLiteral(argument) ||
    ts.isArrayLiteralExpression(argument) ||
    ts.isArrowFunction(argument) ||
    ts.isFunctionExpression(argument)
  );
}
