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

// A `day` activity knows its date and no time. Its time reaches a reader in a
// zone only through activitywhen.ts, which prints its UTC date and no clock.
//
// A reader in a zone is derived, never listed. It is the format call of an
// Intl.DateTimeFormat, or a Date's toLocale*String. In format/ it is also a
// function that puts an hour or minute count into its sentence, or builds a
// RelativeTimeFormat. The value is followed through calls, props, fields,
// object literals, bindings, assignments, `.map` callbacks and element reads.
// It is followed through `new Date`, `Date.parse`, `Math`, arithmetic and
// template text too. A call returning a boolean hands it nowhere.
//
// A `due_at` is an activity time only on a row that can be a meeting. A
// condition on the same row's `source` that rules the meeting out ends it.
//
// A field `x_at` is an activity time too where its record carries
// `x_precision`. A precision handed to the owner must trace to a wire
// precision. Or it is `instant` for an `x_precision` the wire left out, handed
// beside that record's own `x_at`. Any other literal passes
// only when the time it is paired with reads no activity time. That is the
// sibling reaching the same owner call, or else that call's own time.
//
// Not followed, each a known blind spot:
// - a value kept in a Map or pushed into an array, and read back;
// - ISO text cut by hand, as in `at.slice(0, 10)`;
// - a Date's own getters, as in `getDate()`;
// - a comparison against now or another day, which the owner's predicates answer;
// - a projection that carries an activity time without its precision.

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

// Sentence placeholders that put a count of hours or minutes into text.
const CLOCK_PLACEHOLDERS = new Set(["hours", "minutes", "seconds"]);
// The Date methods that print it in a zone.
const LOCALE_PRINTS = new Set([
  "toLocaleString",
  "toLocaleDateString",
  "toLocaleTimeString",
]);
const FORMAT_CALLS = new Set([
  "format",
  "formatToParts",
  "formatRange",
  "formatRangeToParts",
]);
// The fields an activity's time is sent in, on a record that carries
// `occurred_precision`.
const ACTIVITY_TIMES = new Set(["at", "occurred_at", "starts_at", "due_at"]);
// Calls whose result carries their receiver's instant on.
const PASS_THROUGH_METHODS = new Set([
  "getTime",
  "valueOf",
  "toISOString",
  "at",
  "filter",
  "find",
  "findLast",
  "reverse",
  "slice",
  "sort",
  "toSorted",
]);
// Calls whose result carries what their callback returns.
const MAPPERS = new Set(["map", "flatMap"]);

type Site = Readonly<{ file: string; line: number; text: string }>;
type Census = Readonly<{
  findings: Site[];
  unproven: Site[];
  unjudged: Site[];
  readers: Set<string>;
}>;

// A function in format/ that prints an age, judged by its own body.
function printsAge(fn: ts.Node): boolean {
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
      (named !== undefined && CLOCK_PLACEHOLDERS.has(named)) ||
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
  if (ts.isElementAccessExpression(node)) return leaves(node.expression);
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
  // Each activity time read, with the field it was read from.
  const reads: { site: Site; targets: ts.Node[]; field: string }[] = [];
  const unjudged: Site[] = [];
  const seeds: ts.Node[] = [];
  const formatFunctions: { name: string; params: ts.Node[] }[] = [];
  // Each owner call's time beside the precision it is drawn with. A moment
  // has no precision of its own: its time carries one.
  const pairs: { time: ts.Node; precision?: ts.Node }[] = [];

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

  // Whether a record carries an activity's precision under `name`.
  const pairedPrecision = (type: ts.Type, name: string): boolean => {
    const field = checker.getPropertyOfType(type, name);
    const kinds =
      field && checker.getNonNullableType(checker.getTypeOfSymbol(field));
    return (kinds?.isUnion() ? kinds.types : [kinds]).some(
      (kind) => kind?.isStringLiteral() && kind.value === "instant",
    );
  };
  // Whether `record`'s `name` is an activity's time; null when the record has
  // no type to judge. A `due_at` read where `record.source` rules out a
  // meeting is a deadline.
  const activityTime = (record: ts.Node, name: string): boolean | null => {
    const paired = name.match(/^(\w+)_at$/)?.[1];
    if (!ACTIVITY_TIMES.has(name) && paired === undefined) return false;
    const type = checker.getNonNullableType(checker.getTypeAtLocation(record));
    if (type.flags & (ts.TypeFlags.Any | ts.TypeFlags.Unknown)) {
      return ACTIVITY_TIMES.has(name) ? null : false;
    }
    if (pairedPrecision(type, `${paired}_precision`)) return true;
    if (!ACTIVITY_TIMES.has(name)) return false;
    if (name === "due_at" && meetingRuledOut(record)) return false;
    return bearing(type, name === "due_at");
  };

  const hand = (value: ts.Expression, targets: ts.Node[]) => {
    for (const leaf of leaves(value)) {
      if (ts.isPropertyAccessExpression(leaf)) {
        const read = activityTime(leaf.expression, leaf.name.text);
        if (read === null) unjudged.push(siteOf(leaf));
        if (read) {
          reads.push({ site: siteOf(leaf), targets, field: fieldOf(leaf) });
          continue;
        }
      }
      if (ts.isStringLiteralLike(leaf)) {
        for (const to of targets) flow(leaf, to);
        continue;
      }
      if (ts.isObjectLiteralExpression(leaf)) {
        handObject(leaf, targets);
        continue;
      }
      if (ts.isCallExpression(leaf)) handMapped(leaf, targets);
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
  // An object carries every field it is built from.
  const handObject = (
    object: ts.ObjectLiteralExpression,
    targets: ts.Node[],
  ) => {
    for (const property of object.properties) {
      if (ts.isSpreadAssignment(property)) {
        hand(property.expression, targets);
      } else {
        for (const to of targets) flow(property, to);
      }
    }
  };
  // A `.map` carries what its callback returns.
  const handMapped = (call: ts.CallExpression, targets: ts.Node[]) => {
    const callee = call.expression;
    const mapper = call.arguments[0];
    if (
      !ts.isPropertyAccessExpression(callee) ||
      !MAPPERS.has(callee.name.text) ||
      mapper === undefined ||
      !(ts.isArrowFunction(mapper) || ts.isFunctionExpression(mapper))
    ) {
      return;
    }
    for (const to of targets) flow(mapper, to);
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
  // A yes or no carries no instant on to print.
  const answersYesOrNo = (node: ts.CallExpression | ts.NewExpression) => {
    const signature = checker.getResolvedSignature(node);
    return (
      signature !== undefined &&
      (checker.getReturnTypeOfSignature(signature).flags &
        ts.TypeFlags.BooleanLike) !==
        0
    );
  };

  const typeNamed = (node: ts.Expression, name: string) =>
    checker.getTypeAtLocation(node).getSymbol()?.getName() === name;
  // What a call reads in a zone: a Date it prints, or what a DateTimeFormat
  // formats.
  const printedInZone = (node: ts.CallExpression): ts.Expression[] => {
    if (!ts.isPropertyAccessExpression(node.expression)) return [];
    const method = node.expression.name.text;
    const receiver = node.expression.expression;
    if (LOCALE_PRINTS.has(method) && typeNamed(receiver, "Date")) {
      return [receiver];
    }
    if (FORMAT_CALLS.has(method) && typeNamed(receiver, "DateTimeFormat")) {
      return [...node.arguments];
    }
    return [];
  };

  const visitCall = (node: ts.CallExpression | ts.NewExpression) => {
    const declared = signatureOf(node);
    const decides = answersYesOrNo(node);
    node.arguments?.forEach((arg, index) => {
      const params = decides ? [] : parameterAt(declared, index);
      hand(arg, [arg, ...params]);
      for (const param of params) flow(arg, param);
    });
    if (declared?.getSourceFile().fileName === owner) {
      pairOwnerCall(node, declared);
    }
    if (ts.isCallExpression(node)) {
      const printed = printedInZone(node);
      if (printed.length > 0) seeds.push(node);
      for (const value of printed) hand(value, [node]);
    }
  };
  const pairOwnerCall = (
    node: ts.CallExpression | ts.NewExpression,
    declared: ts.SignatureDeclaration,
  ) => {
    let time: ts.Node | undefined;
    let precision: ts.Node | undefined;
    declared.parameters.forEach((param, index) => {
      const arg = node.arguments?.[index];
      const name = param.name.getText();
      if (arg === undefined) return;
      if (name === "precision") precision = arg;
      else if (name === "at" || name === "start") time = arg;
      else if (checker.getTypeAtLocation(param).getProperty("precision")) {
        pairs.push({ time: arg });
      }
    });
    if (precision !== undefined) {
      pairs.push({ time: time ?? precision, precision });
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
      const from = node.parent.parent;
      const receiver =
        ts.isVariableDeclaration(from) && from.initializer
          ? receiverText(from.initializer)
          : undefined;
      reads.push({
        site: siteOf(node),
        targets: [node],
        field: receiver === undefined ? name : `${receiver}.${name}`,
      });
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
  // A format/ function whose own body prints an age seeds its parameters.
  const derive = (
    node: ts.FunctionDeclaration | ts.ArrowFunction | ts.FunctionExpression,
  ) => {
    if (!inFormat(node)) return;
    const name = ts.isFunctionDeclaration(node)
      ? node.name?.text
      : ts.isVariableDeclaration(node.parent)
        ? node.parent.name.getText()
        : undefined;
    if (name !== undefined) {
      formatFunctions.push({ name, params: [...node.parameters] });
    }
    if (returnsText(node) && printsAge(node)) seeds.push(...node.parameters);
  };
  // Writes a value into `node` and into the declared fields it lands in.
  const handInto = (
    value: ts.Expression,
    node:
      | ts.PropertyAssignment
      | ts.ShorthandPropertyAssignment
      | ts.JsxAttribute,
  ) => {
    const fields = contextualProperty(node);
    hand(value, [node, ...fields]);
    for (const field of fields) flow(node, field);
  };

  const visit = (node: ts.Node): void => {
    if (ts.isCallExpression(node) || ts.isNewExpression(node)) {
      visitCall(node);
    } else if (ts.isJsxAttribute(node) && node.initializer !== undefined) {
      const value = ts.isJsxExpression(node.initializer)
        ? node.initializer.expression
        : node.initializer;
      if (value !== undefined && !ts.isJsxElement(value)) handInto(value, node);
    } else if (ts.isPropertyAssignment(node)) {
      handInto(node.initializer, node);
    } else if (ts.isShorthandPropertyAssignment(node)) {
      const value = checker.getShorthandAssignmentValueSymbol(node);
      const fields = contextualProperty(node);
      for (const from of declarations(value)) {
        for (const to of [node, ...fields]) flow(from, to);
      }
      for (const field of fields) flow(node, field);
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
  // Every node whose value reaches one of `start`, up to the nodes `stops`.
  const upstream = (
    start: ts.Node[],
    stops: (node: ts.Node) => boolean = () => false,
  ): Set<ts.Node> => {
    const seen = new Set<ts.Node>(start);
    const queue = [...start];
    for (let next = queue.pop(); next !== undefined; next = queue.pop()) {
      if (stops(next)) continue;
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
  const readers = new Set(
    formatFunctions
      .filter((fn) => fn.params.some((param) => clocked.has(param)))
      .map((fn) => fn.name),
  );

  const readTargets = new Set(reads.flatMap((read) => read.targets));
  const readsActivity = (node: ts.Node) =>
    [...upstream([node])].some((from) => readTargets.has(from));
  // A literal precision is judged by the time it is paired with. That is the
  // sibling reaching the same owner call, or else that call's own time.
  const pairedClean = (literal: ts.Node, time: ts.Node): boolean => {
    const reaching = upstream([time]);
    const paired = siblingsOf(literal).filter((node) => reaching.has(node));
    return paired.length > 0
      ? !paired.some(readsActivity)
      : !readsActivity(time);
  };
  const writesPrecision = (node: ts.Node) =>
    (ts.isPropertyAssignment(node) || ts.isShorthandPropertyAssignment(node)) &&
    node.name.getText() === "precision";
  // The wire's own field: what a fixture writes into it is not this gate's.
  const wirePrecision = (node: ts.Node) =>
    (ts.isPropertySignature(node) || ts.isPropertyDeclaration(node)) &&
    node.name.getText().endsWith("_precision");
  // `instant` standing in for an optional `x_precision` the wire left out. It
  // must sit beside its own time: `x_at`, read off the same record.
  const wireDefault = (literal: ts.StringLiteralLike, time: ts.Node) => {
    const parent = literal.parent;
    if (
      literal.text !== "instant" ||
      !ts.isBinaryExpression(parent) ||
      parent.operatorToken.kind !== ts.SyntaxKind.QuestionQuestionToken ||
      parent.right !== literal ||
      !ts.isPropertyAccessExpression(parent.left)
    ) {
      return false;
    }
    const field = checker.getSymbolAtLocation(parent.left);
    const sibling = fieldOf(parent.left).match(/^(.*)_precision$/)?.[1];
    if (
      sibling === undefined ||
      field === undefined ||
      (field.flags & ts.SymbolFlags.Optional) === 0
    ) {
      return false;
    }
    const reaching = upstream([time]);
    return reads.some(
      (read) =>
        read.field === `${sibling}_at` &&
        read.targets.some((target) => reaching.has(target)),
    );
  };
  const proven = (origin: ts.Node, time: ts.Node) =>
    wirePrecision(origin) ||
    (ts.isStringLiteralLike(origin) &&
      (wireDefault(origin, time) || pairedClean(origin, time)));
  // A precision field the owner declares is a sink too, paired with its `at`.
  const ownerSource = program.getSourceFile(owner);
  if (ownerSource === undefined) throw new Error("the owner is not read");
  const collect = (node: ts.Node): void => {
    if (ts.isTypeLiteralNode(node)) {
      const field = (name: string) =>
        node.members.find((member) => member.name?.getText() === name);
      const at = field("at");
      const precision = field("precision");
      if (at !== undefined && precision !== undefined) {
        pairs.push({ time: at, precision });
      }
    }
    ts.forEachChild(node, collect);
  };
  collect(ownerSource);
  const unproven = pairs.flatMap(({ time, precision }) => {
    const sinks =
      precision === undefined
        ? [...upstream([time])].filter(writesPrecision)
        : [precision];
    return [...upstream(sinks, wirePrecision)]
      .filter(
        (node) => wirePrecision(node) || (feeds.get(node) ?? []).length === 0,
      )
      .filter((origin) => !proven(origin, time))
      .map((origin) =>
        ts.isStringLiteralLike(origin) ? origin : (precision ?? time),
      );
  });
  return {
    findings,
    unproven: [...new Set(unproven)].map(siteOf),
    unjudged,
    readers,
  };
}

// A field read as `receiver.name`, optional chains spelled as plain reads.
function fieldOf(access: ts.PropertyAccessExpression): string {
  return `${receiverText(access.expression)}.${access.name.text}`;
}

function receiverText(node: ts.Expression): string {
  return node.getText().replace(/\s+/g, "").replaceAll("?.", ".");
}

// The args, fields or props written beside a literal, at the nearest call,
// object or element it is written into.
function siblingsOf(literal: ts.Node): ts.Node[] {
  let child: ts.Node = literal;
  for (
    let parent = literal.parent;
    parent !== undefined;
    parent = parent.parent
  ) {
    if (ts.isCallExpression(parent) || ts.isNewExpression(parent)) {
      return (parent.arguments ?? []).filter((arg) => arg !== child);
    }
    if (
      ts.isPropertyAssignment(parent) &&
      ts.isObjectLiteralExpression(parent.parent)
    ) {
      return parent.parent.properties.filter((field) => field !== parent);
    }
    if (ts.isJsxAttribute(parent)) {
      return parent.parent.properties.filter((prop) => prop !== parent);
    }
    if (
      ts.isFunctionLike(parent) ||
      ts.isReturnStatement(parent) ||
      ts.isVariableDeclaration(parent) ||
      ts.isExpressionStatement(parent)
    ) {
      return [];
    }
    child = parent;
  }
  return [];
}

// Whether a condition on `record.source` around `record` rules out a meeting:
// the `&&` before it, or the branch of a `?:` or an `if` it sits in.
function meetingRuledOut(record: ts.Node): boolean {
  const source = `${record.getText()}.source`;
  const says = (condition: ts.Expression, holds: boolean): boolean => {
    let test = condition;
    while (ts.isParenthesizedExpression(test)) test = test.expression;
    if (ts.isPrefixUnaryExpression(test)) {
      return (
        test.operator === ts.SyntaxKind.ExclamationToken &&
        says(test.operand, !holds)
      );
    }
    if (!ts.isBinaryExpression(test)) return false;
    const op = test.operatorToken.kind;
    if (op === ts.SyntaxKind.AmpersandAmpersandToken) {
      return holds && (says(test.left, true) || says(test.right, true));
    }
    const equal = op === ts.SyntaxKind.EqualsEqualsEqualsToken;
    if (
      (!equal && op !== ts.SyntaxKind.ExclamationEqualsEqualsToken) ||
      test.left.getText() !== source ||
      !ts.isStringLiteralLike(test.right)
    ) {
      return false;
    }
    // When the source equals the literal, it is a meeting only if that is it.
    return (equal === holds) !== (test.right.text === "meeting");
  };
  let child: ts.Node = record;
  for (
    let parent = record.parent;
    parent !== undefined && !ts.isFunctionLike(parent);
    parent = parent.parent
  ) {
    if (
      (ts.isBinaryExpression(parent) &&
        parent.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken &&
        child === parent.right &&
        says(parent.left, true)) ||
      (ts.isConditionalExpression(parent) &&
        child !== parent.condition &&
        says(parent.condition, child === parent.whenTrue)) ||
      (ts.isIfStatement(parent) &&
        child !== parent.expression &&
        says(parent.expression, child === parent.thenStatement))
    ) {
      return true;
    }
    child = parent;
  }
  return false;
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

  it("derives the formatters that read a time in a zone", () => {
    expect([...found.readers]).toEqual(
      expect.arrayContaining([
        "formatDate",
        "formatDayMonth",
        "formatDateTime",
        "formatTimeOfDay",
        "formatTimeRange",
        "formatRelativeTime",
        "formatElapsed",
        "calendarDay",
      ]),
    );
    expect(found.readers.has("formatNumber")).toBe(false);
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
      "formatActivityWhen(activity.occurred_at, 'instant', 'en', zone);",
      "formatActivityWhen(activity.occurred_at, activity.occurred_precision ?? 'instant', 'en', zone);",
      "const row22 = { atIso: activity.occurred_at, atPrecision: 'instant' as const }; formatActivityWhen(row22.atIso, row22.atPrecision, 'en', zone);",
      "function When23(p: { at: string; precision: OccurredPrecision }) { return formatActivityWhen(p.at, p.precision, 'en', zone); } void (<When23 at={activity.occurred_at} precision='instant' />);",
      "const when24 = (iso: string) => formatDateTime(iso, 'en', zone); function Card24({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } void (<Card24 when={when24} />);",
      "function Inner25({ when }: { when: (iso: string) => string }) { return when(activity.occurred_at); } function Outer25({ when }: { when: (iso: string) => string }) { return <Inner25 when={when} />; } void (<Outer25 when={(iso) => formatTimeOfDay(iso, 'en', zone)} />);",
      "activityAgo({ at: activity.occurred_at, precision: 'instant' }, Date.now(), t, 'en');",
      "function instantOf(): OccurredPrecision { return 'instant'; } formatActivityWhen(activity.occurred_at, instantOf(), 'en', zone);",
      "const KNOWN = { precision: 'instant' } as const; formatActivityWhen(activity.occurred_at, KNOWN.precision, 'en', zone);",
      "function moment29(at: string) { return { at, precision: 'instant' as const }; } activityAgo(moment29(activity.occurred_at), Date.now(), t, 'en');",
      "const times30 = [activity].map((a) => a.occurred_at); formatDateTime(times30[0] ?? '', 'en', zone);",
      "formatDate(activity.occurred_at, 'en', zone);",
      "new Date(activity.occurred_at).toLocaleString('en', { month: 'long' });",
      "new Date(activity.occurred_at).toLocaleDateString('en');",
      "calendarDay(new Date(activity.occurred_at), zone);",
      "const day35 = new Intl.DateTimeFormat('en', { day: 'numeric' }); day35.format(new Date(activity.occurred_at));",
      "void (item.source !== 'task' ? formatDate(item.due_at ?? '', 'en', zone) : '');",
      "formatDateTime(touch.last_inbound_at ?? '', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.last_inbound_precision ?? 'day', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.last_outbound_precision ?? 'instant', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.summary?.last_inbound_precision ?? 'instant', 'en', zone);",
    ];
    const sound = [
      "formatActivityWhen(activity.occurred_at, activity.occurred_precision, 'en', zone);",
      "activityAgo({ at: activity.occurred_at, precision: activity.occurred_precision }, Date.now(), t, 'en');",
      "activityDay(activity.occurred_at, activity.occurred_precision, 'en', zone, formatDayMonth);",
      "formatDateTime(activity.created_at, 'en', zone);",
      "formatDateTime(activity.due_at ?? '', 'en', zone);",
      "const row42 = { atIso: activity.created_at, atPrecision: 'instant' as const }; formatActivityWhen(row42.atIso, row42.atPrecision, 'en', zone);",
      "function instantOf43(): OccurredPrecision { return 'instant'; } formatActivityWhen(activity.created_at, instantOf43(), 'en', zone);",
      "void (item.source === 'task' ? formatDateTime(item.due_at ?? '', 'en', zone) : '');",
      "void (item.source === 'meeting' ? '' : formatTimeOfDay(item.due_at ?? '', 'en', zone));",
      "if (item.source === 'weekly_commitment') formatDate(item.due_at ?? '', 'en', zone);",
      "formatActivityWhen(touch.last_inbound_at ?? '', touch.last_inbound_precision ?? 'instant', 'en', zone);",
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
    'import { formatDate, formatDateTime, formatDayMonth, formatTimeOfDay } from "./format/format";',
    'import { calendarDay } from "./format/calendarday";',
    'import { formatRelativeTime } from "./format/relativetime";',
    'import { formatElapsed } from "./format/now";',
    'import { activityAgo, activityDay, formatActivityWhen, type OccurredPrecision } from "./format/activitywhen";',
    'import type { Translator } from "./i18n";',
    'type Activity = components["schemas"]["Activity"];',
    "declare const activity: Activity;",
    "declare const zone: string;",
    "declare const t: Translator;",
    'declare const item: components["schemas"]["WorklistItem"];',
    "declare const touch: { last_inbound_at?: string; last_inbound_precision?: OccurredPrecision; last_outbound_precision?: OccurredPrecision; summary?: { last_inbound_precision?: OccurredPrecision } };",
    "void [formatDate, formatDayMonth, calendarDay, activityDay, formatActivityWhen, activityAgo];",
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
