// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesUnder,
  plantedProgram,
  programOver,
} from "../../scripts/lib/source-tree";

// A `day` activity has no time of day, so its time reaches a clock only
// through activitywhen.ts, which reads the precision.
//
// A clock is derived, never listed. In format/ it is a function returning text
// that sets Intl's `hour`, `minute` or `timeStyle`, builds a RelativeTimeFormat,
// or puts an hour or minute count into its sentence. Anywhere else it is a
// Date's toLocaleTimeString or toLocaleString, or the format call of an Intl
// formatter built there with a clock option. The value is followed through
// calls, props, fields, bindings, assignments, collections built by `.map`,
// `new Date`, `Date.parse`, `Math`, arithmetic and template text.
//
// A precision handed to the owner, or written into a precision field it
// declares, must trace to `occurred_precision`. A literal passes only where no
// activity's time is read beside it.
//
// Not followed, each a known blind spot:
// - a value kept in a Map or pushed into an array, and read back;
// - an Intl formatter outside format/ that a function returns;
// - ISO text cut by hand, as in `at.slice(11, 16)`;
// - a date-only formatter handed the viewer's zone;
// - a comparison against now, which the owner's predicates answer.

const here = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(here, "..");
const frontendRoot = resolve(srcRoot, "..");
const extensionsDir = resolve(frontendRoot, "..", "extensions");
const owner = join(here, "activitywhen.ts");
// Shipped TypeScript: a JavaScript module carries no record type to read.
const shipped = (path: string) =>
  /\.tsx?$/.test(path) &&
  !/\.(test|stories)\.tsx?$/.test(path) &&
  !path.includes("/src/testing/");

// Intl options and sentence placeholders that put a time of day into text.
const CLOCK_OPTIONS = new Set(["hour", "minute", "timeStyle"]);
const CLOCK_PLACEHOLDERS = new Set(["hours", "minutes", "seconds"]);
const FORMAT_CALLS = new Set([
  "format",
  "formatToParts",
  "formatRange",
  "formatRangeToParts",
]);
// The fields an activity's time is sent in, on a record that carries its
// precision. A `due_at` is one only on a row that can be a meeting.
const ACTIVITY_TIMES = new Set(["occurred_at", "starts_at", "due_at"]);
// Calls whose result carries their receiver's instant on.
const PASS_THROUGH_METHODS = new Set(["getTime", "valueOf", "toISOString"]);

type Site = Readonly<{ file: string; line: number; text: string }>;
type Census = Readonly<{
  findings: Site[];
  unproven: Site[];
  unjudged: Site[];
  clocks: Set<string>;
}>;

// Whether an options literal asks Intl for a time of day.
function asksForClock(options: ts.Expression | undefined): boolean {
  if (options === undefined || !ts.isObjectLiteralExpression(options)) {
    return false;
  }
  return options.properties.some(
    (property) =>
      property.name !== undefined && CLOCK_OPTIONS.has(property.name.getText()),
  );
}

// A function in format/ that prints a clock, judged by its own body.
function printsClock(fn: ts.Node): boolean {
  let found = false;
  const visit = (node: ts.Node): void => {
    if (found) return;
    const named =
      (ts.isPropertyAssignment(node) ||
        ts.isShorthandPropertyAssignment(node)) &&
      ts.isIdentifier(node.name)
        ? node.name.text
        : undefined;
    if (
      (named !== undefined &&
        (CLOCK_OPTIONS.has(named) || CLOCK_PLACEHOLDERS.has(named))) ||
      (ts.isNewExpression(node) &&
        node.expression.getText() === "Intl.RelativeTimeFormat")
    ) {
      found = true;
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(fn);
  return found;
}

// The expressions whose value an expression hands on.
function leaves(node: ts.Expression): ts.Expression[] {
  if (
    ts.isParenthesizedExpression(node) ||
    ts.isAsExpression(node) ||
    ts.isSatisfiesExpression(node) ||
    ts.isNonNullExpression(node) ||
    ts.isTypeAssertionExpression(node)
  ) {
    return leaves(node.expression);
  }
  if (ts.isBinaryExpression(node)) {
    const op = node.operatorToken.kind;
    if (
      op === ts.SyntaxKind.QuestionQuestionToken ||
      op === ts.SyntaxKind.BarBarToken ||
      op === ts.SyntaxKind.MinusToken ||
      op === ts.SyntaxKind.PlusToken
    ) {
      return [...leaves(node.left), ...leaves(node.right)];
    }
    if (op === ts.SyntaxKind.AmpersandAmpersandToken) return leaves(node.right);
  }
  if (ts.isConditionalExpression(node)) {
    return [...leaves(node.whenTrue), ...leaves(node.whenFalse)];
  }
  if (ts.isTemplateExpression(node)) {
    return node.templateSpans.flatMap((span) => leaves(span.expression));
  }
  if (
    (ts.isNewExpression(node) && node.expression.getText() === "Date") ||
    (ts.isCallExpression(node) &&
      /^(Date\.parse|Math\.\w+)$/.test(node.expression.getText()))
  ) {
    return (node.arguments ?? []).flatMap(leaves);
  }
  if (
    ts.isCallExpression(node) &&
    ts.isPropertyAccessExpression(node.expression) &&
    PASS_THROUGH_METHODS.has(node.expression.name.text)
  ) {
    return leaves(node.expression.expression);
  }
  return [node];
}

function census(program: ts.Program, files: string[]): Census {
  const checker = program.getTypeChecker();
  const analyzed = new Set(files.map((path) => resolve(path)));
  const inTree = (node: ts.Node) =>
    analyzed.has(resolve(node.getSourceFile().fileName));
  const inFormat = (node: ts.Node) =>
    dirname(node.getSourceFile().fileName) === here;
  // Data flows from each key to each node in its set.
  const flows = new Map<ts.Node, Set<ts.Node>>();
  const reads: { site: Site; targets: ts.Node[] }[] = [];
  const unjudged: Site[] = [];
  const seeds: ts.Node[] = [];
  const clocks = new Set<string>();
  const precisionArgs: ts.Expression[] = [];

  const flow = (from: ts.Node, to: ts.Node) => {
    if (from === to || !inTree(to)) return;
    const set = flows.get(from) ?? new Set<ts.Node>();
    set.add(to);
    flows.set(from, set);
  };
  const siteOf = (node: ts.Node): Site => {
    const source = node.getSourceFile();
    return {
      file: relative(frontendRoot, source.fileName),
      line: source.getLineAndCharacterOfPosition(node.getStart()).line + 1,
      text: node.getText().replace(/\s+/g, " ").slice(0, 80),
    };
  };
  const declarations = (symbol: ts.Symbol | undefined): ts.Node[] => {
    if (symbol === undefined) return [];
    const real =
      symbol.flags & ts.SymbolFlags.Alias
        ? checker.getAliasedSymbol(symbol)
        : symbol;
    return real.declarations ?? [];
  };
  const bearing = (type: ts.Type, meetingStart: boolean): boolean =>
    (type.isUnion() ? type.types : [type]).some((member) => {
      if (checker.getPropertyOfType(member, "occurred_precision") === undefined)
        return false;
      if (!meetingStart) return true;
      const source = checker.getPropertyOfType(member, "source");
      const sources = source && checker.getTypeOfSymbol(source);
      return (sources?.isUnion() ? sources.types : [sources]).some(
        (kind) => kind?.isStringLiteral() && kind.value === "meeting",
      );
    });

  // Whether `name` read off `record` is an activity's time; null when the
  // record has no type to judge.
  const activityTime = (record: ts.Node, name: string): boolean | null => {
    if (!ACTIVITY_TIMES.has(name)) return false;
    const type = checker.getNonNullableType(checker.getTypeAtLocation(record));
    if (type.flags & (ts.TypeFlags.Any | ts.TypeFlags.Unknown)) return null;
    return bearing(type, name === "due_at");
  };
  const readsActivityTime = (scope: ts.Node): boolean => {
    let found = false;
    const visit = (node: ts.Node): void => {
      if (found) return;
      if (
        ts.isPropertyAccessExpression(node) &&
        activityTime(node.expression, node.name.text) === true
      ) {
        found = true;
        return;
      }
      ts.forEachChild(node, visit);
    };
    visit(scope);
    return found;
  };

  const hand = (value: ts.Expression, targets: ts.Node[]) => {
    for (const leaf of leaves(value)) {
      if (ts.isPropertyAccessExpression(leaf)) {
        const read = activityTime(leaf.expression, leaf.name.text);
        if (read === null) unjudged.push(siteOf(leaf));
        if (read) {
          reads.push({ site: siteOf(leaf), targets });
          continue;
        }
      }
      if (ts.isStringLiteralLike(leaf)) {
        for (const to of targets) flow(leaf, to);
        continue;
      }
      const named =
        ts.isIdentifier(leaf) || ts.isPropertyAccessExpression(leaf);
      const sources = ts.isCallExpression(leaf)
        ? [checker.getResolvedSignature(leaf)?.declaration]
        : named
          ? declarations(checker.getSymbolAtLocation(leaf))
          : [];
      for (const from of sources) {
        if (from !== undefined) for (const to of targets) flow(from, to);
      }
      if (named) handFunction(leaf, targets);
    }
  };
  // A function handed on by name: a call through the target's type passes its
  // arguments to the function's own parameters.
  const handFunction = (leaf: ts.Expression, targets: ts.Node[]) => {
    const own = signatureDeclaration(checker.getTypeAtLocation(leaf));
    if (own === undefined) return;
    for (const target of targets) {
      const through = signatureDeclaration(typeOfTarget(target));
      if (through === undefined || through === own) continue;
      through.parameters.forEach((param, index) => {
        for (const to of parameterAt(own, index)) flow(param, to);
      });
    }
  };
  const signatureDeclaration = (type: ts.Type) => {
    const declaration = type
      .getNonNullableType()
      .getCallSignatures()[0]?.declaration;
    return declaration !== undefined && !ts.isJSDocSignature(declaration)
      ? declaration
      : undefined;
  };
  const typeOfTarget = (target: ts.Node): ts.Type =>
    ts.isPropertySignature(target) ||
    ts.isParameter(target) ||
    ts.isVariableDeclaration(target) ||
    ts.isPropertyAssignment(target) ||
    ts.isJsxAttribute(target)
      ? checker.getTypeAtLocation(target.name)
      : checker.getTypeAtLocation(target);
  const parameterAt = (
    declaration: ts.SignatureDeclaration | undefined,
    index: number,
  ): ts.Node[] => {
    const params = declaration?.parameters ?? ts.factory.createNodeArray();
    const last = params[params.length - 1];
    const param =
      params[index] ?? (last?.dotDotDotToken !== undefined ? last : undefined);
    return param === undefined ? [] : [param];
  };
  const signatureOf = (node: ts.CallExpression | ts.NewExpression) => {
    const declaration = checker.getResolvedSignature(node)?.declaration;
    return declaration !== undefined && !ts.isJSDocSignature(declaration)
      ? declaration
      : undefined;
  };

  // An Intl formatter built with a clock option, here or in a local binding.
  const clockFormatter = (node: ts.Expression): boolean => {
    if (ts.isNewExpression(node)) {
      return (
        node.expression.getText() === "Intl.DateTimeFormat" &&
        asksForClock(node.arguments?.[1])
      );
    }
    if (!ts.isIdentifier(node)) return false;
    return declarations(checker.getSymbolAtLocation(node)).some(
      (declaration) =>
        ts.isVariableDeclaration(declaration) &&
        declaration.initializer !== undefined &&
        ts.isNewExpression(declaration.initializer) &&
        clockFormatter(declaration.initializer),
    );
  };
  const isDate = (node: ts.Expression) =>
    checker.getTypeAtLocation(node).getSymbol()?.getName() === "Date";
  // What a call outside format/ prints as a time of day.
  const printedClock = (node: ts.CallExpression): ts.Expression[] => {
    if (!ts.isPropertyAccessExpression(node.expression)) return [];
    const method = node.expression.name.text;
    const receiver = node.expression.expression;
    if (method === "toLocaleTimeString") return [receiver];
    if (method === "toLocaleString" && isDate(receiver)) {
      const options = node.arguments[1];
      return options === undefined ||
        !ts.isObjectLiteralExpression(options) ||
        asksForClock(options)
        ? [receiver]
        : [];
    }
    if (FORMAT_CALLS.has(method) && clockFormatter(receiver)) {
      return [...node.arguments];
    }
    return [];
  };

  const visitCall = (node: ts.CallExpression | ts.NewExpression) => {
    const declared = signatureOf(node);
    node.arguments?.forEach((arg, index) => {
      hand(arg, parameterAt(declared, index));
    });
    if (declared?.getSourceFile().fileName === owner) {
      declared.parameters.forEach((param, index) => {
        const arg = node.arguments?.[index];
        if (param.name.getText() === "precision" && arg !== undefined) {
          precisionArgs.push(arg);
          hand(arg, [arg]);
        }
      });
    }
    if (ts.isCallExpression(node) && !inFormat(node)) {
      const printed = printedClock(node);
      if (printed.length > 0) seeds.push(node);
      for (const value of printed) hand(value, [node]);
    }
  };
  // A destructured field: an activity's time is read here, and any other
  // field carries on what its declaration holds.
  const bind = (node: ts.BindingElement) => {
    const key = node.propertyName ?? node.name;
    const name = ts.isIdentifier(key) ? key.text : key.getText();
    const read = activityTime(node.parent, name);
    if (read === null) unjudged.push(siteOf(node));
    if (read) {
      reads.push({ site: siteOf(node), targets: [node] });
      return;
    }
    const property = checker.getTypeAtLocation(node.parent).getProperty(name);
    for (const from of declarations(property)) flow(from, node);
  };
  // A callback receives what its contextual signature's caller passes.
  const callback = (node: ts.ArrowFunction | ts.FunctionExpression) => {
    const contextual = checker
      .getContextualType(node)
      ?.getNonNullableType()
      .getCallSignatures()[0]?.declaration;
    node.parameters.forEach((param, index) => {
      for (const from of parameterAt(
        contextual !== undefined && !ts.isJSDocSignature(contextual)
          ? contextual
          : undefined,
        index,
      )) {
        flow(from, param);
      }
    });
  };
  // A format/ function whose own body prints a clock seeds its parameters.
  const derive = (
    node: ts.FunctionDeclaration | ts.ArrowFunction | ts.FunctionExpression,
  ) => {
    if (!inFormat(node) || !returnsText(node) || !printsClock(node)) return;
    seeds.push(...node.parameters);
    const name = ts.isFunctionDeclaration(node)
      ? node.name?.text
      : ts.isVariableDeclaration(node.parent)
        ? node.parent.name.getText()
        : undefined;
    if (name !== undefined) clocks.add(name);
  };

  const visit = (node: ts.Node): void => {
    if (ts.isCallExpression(node) || ts.isNewExpression(node)) {
      visitCall(node);
    } else if (ts.isJsxAttribute(node) && node.initializer !== undefined) {
      const value = ts.isJsxExpression(node.initializer)
        ? node.initializer.expression
        : node.initializer;
      if (value !== undefined && !ts.isJsxElement(value)) {
        hand(value, [node, ...contextualProperty(node)]);
      }
    } else if (ts.isPropertyAssignment(node)) {
      hand(node.initializer, [node, ...contextualProperty(node)]);
    } else if (ts.isShorthandPropertyAssignment(node)) {
      const value = checker.getShorthandAssignmentValueSymbol(node);
      for (const from of declarations(value)) {
        for (const to of [node, ...contextualProperty(node)]) flow(from, to);
      }
    } else if (ts.isVariableDeclaration(node) && node.initializer) {
      hand(node.initializer, [node]);
    } else if (
      ts.isBinaryExpression(node) &&
      node.operatorToken.kind === ts.SyntaxKind.EqualsToken
    ) {
      hand(node.right, declarations(checker.getSymbolAtLocation(node.left)));
    } else if (ts.isReturnStatement(node) && node.expression) {
      const fn = ts.findAncestor(node, ts.isFunctionLike);
      if (fn !== undefined) hand(node.expression, [fn]);
    } else if (ts.isArrowFunction(node) && !ts.isBlock(node.body)) {
      hand(node.body, [node]);
    } else if (
      ts.isBindingElement(node) &&
      ts.isObjectBindingPattern(node.parent)
    ) {
      bind(node);
    }
    if (ts.isArrowFunction(node) || ts.isFunctionExpression(node)) {
      callback(node);
    }
    if (
      ts.isFunctionDeclaration(node) ||
      ts.isArrowFunction(node) ||
      ts.isFunctionExpression(node)
    ) {
      derive(node);
    }
    ts.forEachChild(node, visit);
  };
  // A clock read only to compute a zone's offset is not one a reader sees.
  const returnsText = (fn: ts.SignatureDeclaration): boolean => {
    const signature = checker.getSignatureFromDeclaration(fn);
    return (
      signature !== undefined &&
      (checker.getReturnTypeOfSignature(signature).flags &
        ts.TypeFlags.StringLike) !==
        0
    );
  };
  // The interface fields a property is written into. A literal built inside a
  // callback, as in `.map((row) => ({ ... }))`, meets it at the enclosing call.
  const contextualProperty = (
    node:
      | ts.PropertyAssignment
      | ts.ShorthandPropertyAssignment
      | ts.JsxAttribute,
  ): ts.Node[] => {
    const name = node.name.getText();
    const declaredIn = (type: ts.Type | undefined): ts.Node[] => {
      const members =
        type === undefined ? [] : type.isUnion() ? type.types : [type];
      return members
        .flatMap((member) => {
          const element = checker.getIndexTypeOfType(
            member,
            ts.IndexKind.Number,
          );
          return element === undefined ? [member] : [member, element];
        })
        .flatMap((member) =>
          declarations(checker.getPropertyOfType(member, name)),
        )
        .filter((declaration) => declaration !== node);
    };
    const own = declaredIn(checker.getContextualType(node.parent as never));
    if (own.length > 0) return own;
    const call = ts.findAncestor(node.parent, ts.isCallExpression);
    return call === undefined
      ? []
      : declaredIn(checker.getContextualType(call));
  };

  for (const path of files) {
    if (resolve(path) === owner) continue;
    const source = program.getSourceFile(path);
    if (source === undefined) throw new Error(`${path} is not in the program`);
    visit(source);
  }

  const feeds = new Map<ts.Node, ts.Node[]>();
  for (const [from, tos] of flows) {
    for (const to of tos) feeds.set(to, [...(feeds.get(to) ?? []), from]);
  }
  // Every node whose value reaches one of `start`.
  const upstream = (start: ts.Node[]): Set<ts.Node> => {
    const seen = new Set<ts.Node>(start);
    const queue = [...start];
    for (let next = queue.pop(); next !== undefined; next = queue.pop()) {
      for (const from of feeds.get(next) ?? []) {
        if (!seen.has(from)) {
          seen.add(from);
          queue.push(from);
        }
      }
    }
    return seen;
  };
  const clocked = upstream(seeds);
  const findings = reads
    .filter((read) => read.targets.some((target) => clocked.has(target)))
    .map((read) => read.site);

  // A literal precision is proven only where nothing near it reads an
  // activity's time: the object, call, props or function it is written in.
  const provenOrigin = (node: ts.Node): boolean => {
    if (
      (ts.isPropertySignature(node) || ts.isPropertyDeclaration(node)) &&
      node.name.getText() === "occurred_precision"
    ) {
      return true;
    }
    if (!ts.isStringLiteralLike(node)) return false;
    const scope = ts.findAncestor(
      node.parent,
      (parent) =>
        ts.isObjectLiteralExpression(parent) ||
        ts.isCallExpression(parent) ||
        ts.isNewExpression(parent) ||
        ts.isJsxAttributes(parent) ||
        ts.isFunctionLike(parent),
    );
    return scope !== undefined && !readsActivityTime(scope);
  };
  const unprovenOrigins = (sink: ts.Node) =>
    [...upstream([sink])].filter(
      (node) => (feeds.get(node) ?? []).length === 0 && !provenOrigin(node),
    );
  // A precision field the owner declares is a sink too: every writer of it.
  const fields: ts.Node[] = [];
  const collect = (node: ts.Node): void => {
    if (ts.isPropertySignature(node) && node.name.getText() === "precision") {
      fields.push(node);
    }
    ts.forEachChild(node, collect);
  };
  const ownerSource = program.getSourceFile(owner);
  if (ownerSource === undefined) throw new Error("the owner is not read");
  collect(ownerSource);
  const unproven = [
    ...precisionArgs.filter((arg) => unprovenOrigins(arg).length > 0),
    ...fields.flatMap(unprovenOrigins),
  ].map(siteOf);
  return { findings, unproven, unjudged, clocks };
}

// Every shipped module, extension screens included, and the format/ modules
// the clock formatters are derived from.
function shippedFiles(): string[] {
  return [
    ...filesUnder(srcRoot),
    ...extensionFrontendFiles(extensionsDir),
  ].filter(shipped);
}

describe("an activity's time reaches a clock only through its precision", () => {
  const files = shippedFiles();
  const program = programOver(files, frontendRoot);
  const found = census(program, files);

  it("reads every folder of the tree, extension screens included", () => {
    const read = new Set(
      files
        .filter((path) => path.startsWith(srcRoot))
        .map((path) => relative(srcRoot, path).split(/[/\\]/)[0]),
    );
    const listed = readdirSync(srcRoot, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .filter((name) => filesUnder(join(srcRoot, name)).some(shipped));
    expect(listed.length).toBeGreaterThan(3);
    expect([...read].filter((name) => !name.includes("."))).toEqual(
      expect.arrayContaining(listed),
    );
    expect(files.some((path) => path.includes("/extensions/"))).toBe(true);
  });

  it("derives the formatters that print a clock", () => {
    expect([...found.clocks]).toEqual(
      expect.arrayContaining([
        "formatDateTime",
        "formatTimeOfDay",
        "formatTimeRange",
        "formatRelativeTime",
        "formatElapsed",
      ]),
    );
    expect(found.clocks.has("formatDate")).toBe(false);
  });

  it("judges every activity time it reads", () => {
    expect(found.unjudged).toEqual([]);
  });

  it("sees each shape of the defect, and passes the owner", () => {
    const defects = [
      "formatDateTime(activity.occurred_at, 'en', zone);",
      "const at2 = activity.occurred_at; formatTimeOfDay(at2, 'en', zone);",
      "const when3 = (iso: string) => formatDateTime(iso, 'en', zone); when3(activity.occurred_at);",
      "function Clock4({ at }: { at: string }) { return formatTimeOfDay(at, 'en', zone); } void (<Clock4 at={activity.occurred_at} />);",
      "const row5: { atIso: string } = { atIso: activity.occurred_at }; formatDateTime(row5.atIso, 'en', zone);",
      "formatDateTime(item.occurred_at ?? '', 'en', zone);",
      "formatTimeOfDay(item.due_at ?? '', 'en', zone);",
      "function start8(a: Activity) { return a.occurred_at; } formatDateTime(start8(activity), 'en', zone);",
      "formatRelativeTime(activity.occurred_at ?? activity.created_at, 'en');",
      "function Card10({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } void (<Card10 when={(iso) => formatDateTime(iso, 'en', zone)} />);",
      "const rows11: { atIso: string }[] = [activity].map((a) => ({ atIso: a.occurred_at })); formatDateTime(rows11[0]?.atIso ?? '', 'en', zone);",
      "const { occurred_at: at12 } = activity; formatDateTime(at12, 'en', zone);",
      "let at13 = ''; at13 = activity.occurred_at; formatTimeOfDay(at13, 'en', zone);",
      "formatElapsed(Date.now() - Date.parse(activity.occurred_at), t, 'en');",
      "formatElapsed(Math.max(0, Date.now() - new Date(activity.occurred_at).getTime()), t, 'en');",
      "new Date(activity.occurred_at).toLocaleTimeString('en');",
      "new Date(activity.occurred_at).toLocaleString('en');",
      "const clock18 = new Intl.DateTimeFormat('en', { hour: '2-digit' }); clock18.format(new Date(activity.occurred_at));",
      // biome-ignore lint/suspicious/noTemplateCurlyInString: the planted source holds a template literal
      "formatTimeOfDay(`${activity.occurred_at}`, 'en', zone);",
      "formatActivityWhen(activity.occurred_at, 'instant', 'en', zones);",
      "formatActivityWhen(activity.occurred_at, activity.occurred_precision ?? 'instant', 'en', zones);",
      "const row22 = { atIso: activity.occurred_at, atPrecision: 'instant' as const }; formatActivityWhen(row22.atIso, row22.atPrecision, 'en', zones);",
      "function When23(p: { at: string; precision: OccurredPrecision }) { return formatActivityWhen(p.at, p.precision, 'en', zones); } void (<When23 at={activity.occurred_at} precision='instant' />);",
      "const when24 = (iso: string) => formatDateTime(iso, 'en', zone); function Card24({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } void (<Card24 when={when24} />);",
      "function Inner25({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } function Outer25({ when }: { when: (iso: string) => string }) { return <Inner25 when={when} />; } void (<Outer25 when={(iso) => formatTimeOfDay(iso, 'en', zone)} />);",
      "activityAgo({ at: activity.occurred_at, precision: 'instant' }, Date.now(), t, 'en', zone);",
    ];
    const sound = [
      "formatActivityWhen(activity.occurred_at, activity.occurred_precision, 'en', zones);",
      "activityAgo({ at: activity.occurred_at, precision: activity.occurred_precision }, Date.now(), t, 'en', zone);",
      "formatDate(activity.occurred_at, 'en', zone);",
      "formatDateTime(activity.created_at, 'en', zone);",
      "formatDateTime(activity.due_at ?? '', 'en', zone);",
      "new Date(activity.occurred_at).toLocaleString('en', { month: 'long' });",
      "const row29 = { atIso: activity.created_at, atPrecision: 'instant' as const }; formatActivityWhen(row29.atIso, row29.atPrecision, 'en', zones);",
    ];
    expect(plant([...defects, ...sound])).toEqual(
      defects.map((_, index) => index + 1),
    );
  });

  it("finds no reader left that draws a day entry's clock", () => {
    expect(found.findings).toEqual([]);
  });

  it("finds no owner call handed a precision it cannot trace", () => {
    expect(found.unproven).toEqual([]);
  });
});

// Compiles the planted lines against the real contract and formatters. The
// preamble is one line, so a finding's line is its case's own index.
function plant(lines: string[]): number[] {
  const fixture = join(srcRoot, "activity-precision-fixture.tsx");
  const preamble = [
    'import type { components } from "./api/schema";',
    'import { formatDate, formatDateTime, formatTimeOfDay } from "./format/format";',
    'import { formatRelativeTime } from "./format/relativetime";',
    'import { formatElapsed } from "./format/now";',
    'import { activityAgo, formatActivityWhen, type OccurredPrecision } from "./format/activitywhen";',
    'import type { Translator } from "./i18n";',
    'type Activity = components["schemas"]["Activity"];',
    "declare const activity: Activity;",
    "declare const zone: string;",
    "declare const zones: { record: string; viewer: string };",
    "declare const t: Translator;",
    'declare const item: components["schemas"]["WorklistItem"];',
    "void [formatDate, formatActivityWhen, activityAgo];",
  ].join(" ");
  const source = [preamble, ...lines].join("\n");
  const program = plantedProgram(fixture, source, frontendRoot);
  const broken = program
    .getSemanticDiagnostics(program.getSourceFile(fixture))
    // Unused locals, and the virtual file missing from the project's list.
    .filter(
      (diagnostic) => diagnostic.code !== 6133 && diagnostic.code !== 6307,
    );
  if (broken.length > 0) {
    throw new Error(
      `the planted fixture did not type: ${broken
        .map((d) => ts.flattenDiagnosticMessageText(d.messageText, " "))
        .join("; ")}`,
    );
  }
  const formatFiles = filesUnder(here)
    .filter(shipped)
    .filter((path) => program.getSourceFile(path) !== undefined);
  const { findings, unproven } = census(program, [fixture, ...formatFiles]);
  const caught = [...findings, ...unproven]
    .filter((site) => site.file.endsWith("activity-precision-fixture.tsx"))
    .map((site) => site.line - 1);
  return [...new Set(caught)].sort((a, b) => a - b);
}
