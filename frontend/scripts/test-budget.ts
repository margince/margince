import ts from "typescript";
// Reads a test file and works out, for every test in it, how long that test is
// ALLOWED to spend waiting — the sum of the budgets of the waiters it runs in
// sequence — and how long it is allowed to take. Issue 1144 is what happens
// when the first number exceeds the second: the test fails while every waiter
// in it is still inside its own budget, and the failure names the test rather
// than the waiter that was slow.
//
// Parsed with the TypeScript compiler rather than matched with a regex. A
// hand-rolled scanner over this tree over-counted a 688-line file as one test
// with 39 waiters, and a guard reporting a number nobody believes is worse than
// no guard: it would demand a ceiling ten times anything real.
//
// EVERY UNCERTAINTY IS REPORTED, NEVER ASSUMED AWAY. A waiter whose timeout
// this cannot fold, or a test shape it does not recognise, is the one case that
// matters — the budget it hides is exactly the budget nobody checked. An
// earlier version defaulted an unfoldable `{ timeout: … }` to the smallest
// possible value and walked straight past a live 11000ms violation.
import { bindingOf } from "../src/testing/classnames";
import { ASYNC_UTIL_TIMEOUT_MS } from "../vitest.budget";
import { importedConsts, parse, resolver } from "./test-budget.consts";
import {
  OPAQUE,
  statedTimeout,
  timeoutExpression,
} from "./test-budget.timeouts";

/** A waiter that states no timeout takes Testing Library's default. */
const DEFAULT_WAITER_MS = ASYNC_UTIL_TIMEOUT_MS;

const WAITER = /^(findBy|findAllBy|waitFor|waitForElementToBeRemoved)/;

/** `it`, and the forms that wrap it: `it.each(cases)(…)`, `it.only`, `it.for`. */
const TEST_CALLEE = /^(it|test)$/;

/**
 * The modifiers that mean vitest never runs the callback. A skipped body spends
 * nothing, so letting one into the population lets a test that cannot fail set
 * the ceiling for the thousands that do — or redden the guard while naming a
 * test nobody executes.
 *
 * `runIf` and `skipIf` are NOT here: they run depending on a condition, so a
 * test written either way really executes and really owes a budget. Treating
 * them as skipped left a live test under no guard at all, which is the more
 * expensive mistake of the two.
 */
const NOT_RUN = /^(skip|todo)$/;

/** Whether any link in the call chain says "do not run this". */
function isSkipped(node: ts.Node): boolean {
  let current: ts.Node = node;
  for (;;) {
    if (ts.isCallExpression(current)) current = current.expression;
    else if (ts.isTaggedTemplateExpression(current)) current = current.tag;
    else if (ts.isPropertyAccessExpression(current)) {
      if (NOT_RUN.test(current.name.text)) return true;
      current = current.expression;
    } else return false;
  }
}

export type TestBudget = {
  file: string;
  line: number;
  name: string;
  /** The sum of the waiter budgets this test can spend, in ms. */
  waiterBudgetMs: number;
  /** The per-test ceiling this test actually runs under, in ms. */
  ceilingMs: number;
  /** Whether that ceiling is the test's own, rather than the suite default. */
  ceilingIsStated: boolean;
  /** Timeouts this reader could not fold, each as written. Must be empty. */
  unresolved: string[];
};

/**
 * The root name a call is made through: `it` for `it`, `it.only`, `it.each(…)`
 * and `it.each(…)(…)` alike. Walking to the root is what keeps the table-driven
 * and focused forms visible — matching only the immediate callee dropped 17
 * call sites in this tree, silently, which is the shape of hole this reader
 * exists to close.
 */
function rootCallee(node: ts.Node): string {
  let current: ts.Node = node;
  for (;;) {
    if (ts.isCallExpression(current)) current = current.expression;
    else if (ts.isPropertyAccessExpression(current))
      current = current.expression;
    else if (ts.isTaggedTemplateExpression(current)) current = current.tag;
    else break;
  }
  return ts.isIdentifier(current) ? current.text : "";
}

/**
 * The immediate name a call uses. Good enough to spot a waiter, which is always
 * reached through an object (`screen.findByText`) or bare (`waitFor`) — but a
 * HELPER is only ever called by its bare name, so the caller checks that too.
 * Without it `userEvent.clear(input)` was charged the whole budget of a
 * file-declared `clear` helper, and phantom budget inflates the very
 * measurement that sets the suite's ceiling.
 */
function directCallee(call: ts.CallExpression): string {
  const expression = call.expression;
  if (ts.isPropertyAccessExpression(expression)) return expression.name.text;
  if (ts.isIdentifier(expression)) return expression.text;
  return "";
}

type Reading = { ms: number; unresolved: string[] };

type Helper = ts.ArrowFunction | ts.FunctionExpression | ts.FunctionDeclaration;

const isHelper = (node: ts.Node | undefined): node is Helper =>
  node !== undefined &&
  (ts.isArrowFunction(node) ||
    ts.isFunctionExpression(node) ||
    ts.isFunctionDeclaration(node));

/**
 * The helper a bare call names, read from the scope of the call. Two suites in
 * one file commonly declare their own `mount` or `settle`, and each call means
 * the one it can see. A helper may also come back from another one, as
 * `const { confirm } = await mount()` takes it, and that is followed too.
 */
function helperNamed(name: ts.Identifier, hops = 3): Helper | undefined {
  const bound = bindingOf(name);
  if (bound && ts.isFunctionDeclaration(bound)) return bound;
  if (!bound || !ts.isVariableDeclaration(bound)) return undefined;
  if (ts.isIdentifier(bound.name)) {
    return isHelper(bound.initializer) ? bound.initializer : undefined;
  }
  return hops > 0 ? handedBack(bound, name.text, hops - 1) : undefined;
}

/** The helper `const { local } = await maker()` takes from what `maker`
 * returns, as a shorthand or a function-valued property. */
function handedBack(
  declared: ts.VariableDeclaration,
  local: string,
  hops: number,
): Helper | undefined {
  if (!ts.isObjectBindingPattern(declared.name)) return undefined;
  const element = declared.name.elements.find(
    (e) => ts.isIdentifier(e.name) && e.name.text === local,
  );
  const key = element && (element.propertyName ?? element.name);
  const maker = makerOf(declared.initializer, hops);
  if (!key || !ts.isIdentifier(key) || !maker) return undefined;
  for (const returned of returnedBy(maker)) {
    const property = returned.properties.find(
      (one) => one.name?.getText() === key.text,
    );
    if (property) return propertyHelper(property, hops);
  }
  return undefined;
}

/** The helper `maker()` or `await maker()` calls. */
function makerOf(
  init: ts.Expression | undefined,
  hops: number,
): Helper | undefined {
  const made = init && ts.isAwaitExpression(init) ? init.expression : init;
  return made && ts.isCallExpression(made) && ts.isIdentifier(made.expression)
    ? helperNamed(made.expression, hops)
    : undefined;
}

function propertyHelper(
  property: ts.ObjectLiteralElementLike,
  hops: number,
): Helper | undefined {
  if (ts.isShorthandPropertyAssignment(property)) {
    return helperNamed(property.name, hops);
  }
  const value = ts.isPropertyAssignment(property)
    ? property.initializer
    : undefined;
  if (isHelper(value)) return value;
  return value && ts.isIdentifier(value) ? helperNamed(value, hops) : undefined;
}

/** The object literals a helper returns, not counting its nested functions. */
function returnedBy(helper: Helper): ts.ObjectLiteralExpression[] {
  const found: ts.ObjectLiteralExpression[] = [];
  const take = (value: ts.Expression | undefined) => {
    const bare =
      value && ts.isParenthesizedExpression(value) ? value.expression : value;
    if (bare && ts.isObjectLiteralExpression(bare)) found.push(bare);
  };
  const visit = (node: ts.Node): void => {
    if (ts.isReturnStatement(node)) take(node.expression);
    if (!ts.isFunctionLike(node)) ts.forEachChild(node, visit);
  };
  const body = helper.body;
  if (body && ts.isBlock(body)) ts.forEachChild(body, visit);
  else take(body);
  return found;
}

/**
 * Whether this node is the DEFINITION of a helper, rather than a call to one.
 * The initializer has to BE a function. A local such as
 * `const renderAs = await screen.findByText(…)` is not a definition, and
 * skipping it would drop its waiter.
 */
function declaresHelper(node: ts.Node): boolean {
  if (ts.isVariableDeclaration(node)) {
    const fn = node.initializer;
    return !!fn && (ts.isArrowFunction(fn) || ts.isFunctionExpression(fn));
  }
  return ts.isFunctionDeclaration(node) && node.body !== undefined;
}

/**
 * The waiter budget of one node, following awaited calls into the helpers that
 * do the waiting — a suite's render helper is usually where they live, and a
 * reader that stops at the test body cannot see them.
 *
 * `path` guards against recursion only, and is unwound on the way out: a helper
 * awaited four times costs four times, which is the whole point. Costs are
 * memoized per helper so that repetition is cheap rather than quadratic.
 */
function budgetOf(
  node: ts.Node,
  imported: ReadonlyMap<string, number>,
  path: Set<ts.Node>,
  memo: Map<ts.Node, Reading>,
): Reading {
  const value = resolver(imported);
  let ms = 0;
  const unresolved: string[] = [];

  const helperCost = (body: ts.Node): Reading => {
    const cached = memo.get(body);
    if (cached) return cached;
    // A helper reached while it is being computed adds nothing here, and that
    // truncated reading is not cached. Mutual recursion still under-counts by
    // call order; none occurs in this tree, and keying the memo on the path
    // would make it exact.
    if (path.has(body)) return { ms: 0, unresolved: [] };
    const outermost = path.size === 0;
    path.add(body);
    const reading = budgetOf(body, imported, path, memo);
    path.delete(body);
    if (outermost) memo.set(body, reading);
    return reading;
  };

  const takeWaiter = (call: ts.CallExpression): void => {
    const stated = statedTimeout(call);
    // Both libraries read `timeout: undefined` as no timeout stated.
    if (
      stated === undefined ||
      (stated !== OPAQUE &&
        ts.isIdentifier(stated) &&
        stated.text === "undefined")
    ) {
      ms += DEFAULT_WAITER_MS;
      return;
    }
    if (stated === OPAQUE) {
      unresolved.push(call.getText().replace(/\s+/g, " ").slice(0, 80));
      return;
    }
    const folded = value(stated);
    if (folded === undefined) unresolved.push(stated.getText());
    else ms += folded;
  };

  const takeHelper = (body: ts.Node): void => {
    const reading = helperCost(body);
    ms += reading.ms;
    for (const item of reading.unresolved) {
      if (!unresolved.includes(item)) unresolved.push(item);
    }
  };

  const visit = (current: ts.Node): void => {
    // A helper DECLARED inside the body being read is counted at each call, so
    // descending into its definition as well would charge the test once more
    // for a waiter it may never reach.
    if (declaresHelper(current)) return;
    if (ts.isCallExpression(current)) {
      const helper = ts.isIdentifier(current.expression)
        ? helperNamed(current.expression)
        : undefined;
      if (WAITER.test(directCallee(current))) takeWaiter(current);
      else if (helper?.body) takeHelper(helper.body);
    }
    ts.forEachChild(current, visit);
  };
  // The node itself, not only its children: a one-line arrow helper's body IS
  // the waiter call, and descending past it counts nothing.
  visit(node);
  return { ms, unresolved };
}

/**
 * The ceiling a test states for itself, in either form vitest accepts:
 * `it(name, fn, 500)` and `it(name, { timeout: 500 }, fn)`. The bare number is
 * only ever AFTER the callback — scanning from argument 0 reads a non-string
 * title as the ceiling, and then reports a test's own name as a timeout the
 * reader cannot fold.
 */
function statedCeiling(
  call: ts.CallExpression,
  body: ts.Expression,
): ts.Expression | typeof OPAQUE | undefined {
  // Any expression after the callback, not only a literal or a name: a ceiling
  // written as `SETTLE_MS * 4` was neither folded NOR reported, so the test
  // silently rejoined the population it had opted out of.
  const positional = call.arguments
    .slice(call.arguments.indexOf(body) + 1)
    .find(
      (argument) =>
        !ts.isObjectLiteralExpression(argument) &&
        !ts.isArrowFunction(argument) &&
        !ts.isFunctionExpression(argument),
    );
  if (positional) return positional;
  return call.arguments
    .filter((argument) => ts.isObjectLiteralExpression(argument))
    .map((argument) => timeoutExpression(argument))
    .find((expression) => expression !== undefined);
}

export function budgetsIn(file: string, globalCeilingMs: number): TestBudget[] {
  const source = parse(file);
  const imported = importedConsts(source, file);
  const value = resolver(imported);
  const found: TestBudget[] = [];

  const record = (call: ts.CallExpression): void => {
    // vitest takes the callback anywhere after the name, and the ceiling either
    // as a bare number or as `{ timeout }` — `it(name, fn, 500)` and
    // `it(name, { timeout: 500 }, fn)` are both supported, so neither position
    // may be assumed.
    const body = call.arguments.find(
      (argument) =>
        ts.isArrowFunction(argument) || ts.isFunctionExpression(argument),
    );
    if (!body) return;

    const unresolved: string[] = [];
    let ceilingMs = globalCeilingMs;
    let ceilingIsStated = false;
    // The bare number is only ever AFTER the callback: `it(name, fn, ms)`.
    // Scanning from argument 0 reads a non-string title as the ceiling, and
    // then reports a test's own name as a timeout it cannot fold.
    const stated = statedCeiling(call, body);
    if (stated === OPAQUE) {
      unresolved.push("an options object this reader cannot read");
    } else if (stated) {
      const folded = value(stated);
      if (folded === undefined) unresolved.push(stated.getText());
      else {
        ceilingMs = folded;
        ceilingIsStated = true;
      }
    }

    const title = call.arguments[0];
    const reading = budgetOf(body, imported, new Set(), new Map());
    found.push({
      file,
      line:
        source.getLineAndCharacterOfPosition(call.getStart(source)).line + 1,
      name: title && ts.isStringLiteralLike(title) ? title.text : "<computed>",
      waiterBudgetMs: reading.ms,
      ceilingMs,
      ceilingIsStated,
      unresolved: [...unresolved, ...reading.unresolved],
    });
  };

  // `skipped` is carried DOWN the tree: `describe.skip` suppresses every test
  // inside it, and a reader that only inspected the `it` chain reddened the
  // guard naming a test vitest never runs — the same failure isSkipped was
  // added to prevent, one level up.
  const visit = (node: ts.Node, skipped: boolean): void => {
    let inherited = skipped;
    if (ts.isCallExpression(node)) {
      const root = rootCallee(node);
      const here = skipped || isSkipped(node);
      if (root === "describe") inherited = here;
      else if (TEST_CALLEE.test(root) && !here) record(node);
    }
    ts.forEachChild(node, (child) => visit(child, inherited));
  };
  visit(source, false);
  return found;
}
