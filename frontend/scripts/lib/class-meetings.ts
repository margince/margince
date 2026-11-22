// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { parseSource } from "./source-tree";
import type { Written } from "./surface-sheets";

// Classes that meet only in markup: one element's class list built in code,
// such as `stat-card` with the `stat-card-narrow-${narrow}` statcard.tsx adds.
// No stylesheet joins them, so a gate reading selectors alone misses the pair.

export type ClassMeeting = { exact: readonly string[]; prefixes: string[] };

const CLASS_BUILDERS = new Set(["cn", "clsx", "classNames"]);

function isClassListSite(node: ts.Node): boolean {
  if (ts.isJsxAttribute(node)) return node.name.getText() === "className";
  if (!ts.isCallExpression(node)) return false;
  const callee = node.expression;
  if (ts.isIdentifier(callee)) return CLASS_BUILDERS.has(callee.text);
  const [separator] = node.arguments;
  return (
    ts.isPropertyAccessExpression(callee) &&
    callee.name.text === "join" &&
    separator !== undefined &&
    ts.isStringLiteralLike(separator) &&
    separator.text === " "
  );
}

function fragmentsOf(site: ts.Node): { text: string; open: boolean }[] {
  const found: { text: string; open: boolean }[] = [];
  const walk = (node: ts.Node) => {
    if (ts.isStringLiteralLike(node)) {
      found.push({ text: node.text, open: false });
    } else if (ts.isTemplateHead(node) || ts.isTemplateMiddle(node)) {
      found.push({ text: node.text, open: true });
    } else if (ts.isTemplateTail(node)) {
      found.push({ text: node.text, open: false });
    }
    ts.forEachChild(node, walk);
  };
  walk(site);
  return found;
}

function meetingOf(site: ts.Node): ClassMeeting {
  const exact: string[] = [];
  const prefixes: string[] = [];
  for (const { text, open } of fragmentsOf(site)) {
    const words = text.split(/\s+/).filter((word) => word !== "");
    const last = words.at(-1);
    const openEnded = open && last !== undefined && !/\s$/.test(text);
    exact.push(...(openEnded ? words.slice(0, -1) : words));
    if (openEnded && last !== undefined) prefixes.push(last);
  }
  return { exact, prefixes };
}

/** Each class list this file builds, as the literal classes and open prefixes it names. */
export function classMeetings(path: string, text: string): ClassMeeting[] {
  if (!/className|\bcn\(|clsx\(|classNames\(|\.join\(/.test(text)) return [];
  const meetings: ClassMeeting[] = [];
  const walk = (node: ts.Node) => {
    if (isClassListSite(node)) {
      const meeting = meetingOf(node);
      if (meeting.exact.length + meeting.prefixes.length > 1) {
        meetings.push(meeting);
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(parseSource(path, text));
  return meetings;
}

/**
 * The members, and beside each one the same member carrying the classes of its
 * own family that a markup site lists next to it: `stat-card-alert`, or the
 * sheet classes an open prefix such as `stat-card-narrow-` reaches. A state
 * class like `is-lifted` joins no family, since a sheet uses it on other
 * elements. A prefix that is the member's class and a dash (`panel-${tone}`) is
 * skipped: it cannot tell a variant from a part such as `panel-head`.
 */
export function withMeetings(
  members: readonly Written[],
  meetings: readonly ClassMeeting[],
  known: ReadonlySet<string>,
): Written[] {
  const joined = members.flatMap((member) => {
    const family = (name: string) =>
      [...member.own].some((own) => name.startsWith(`${own}-`));
    const names = meetings
      .filter((meeting) => meeting.exact.some((name) => member.own.has(name)))
      .flatMap((meeting) => [
        ...meeting.exact,
        ...[...known].filter((name) =>
          meeting.prefixes.some(
            (prefix) =>
              name.startsWith(prefix) &&
              ![...member.own].some((own) => prefix === `${own}-`),
          ),
        ),
      ])
      .filter(family);
    if (names.length === 0) return [];
    return [
      {
        ...member,
        own: new Set([...member.own, ...names]),
        ownSimples: new Set([
          ...member.ownSimples,
          ...names.map((name) => `.${name}`),
        ]),
      },
    ];
  });
  return [...members, ...joined];
}
