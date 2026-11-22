// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import ts from "typescript";
import { classVariants } from "../../src/testing/classnames";
import { parseSource } from "./source-tree";
import type { Written } from "./surface-sheets";

// Classes that meet only in markup: one element's class list built in code,
// such as `stat-card` with the `stat-card-narrow-${narrow}` statcard.tsx adds.
// No stylesheet joins them, so a gate reading selectors alone misses the pair.
// The lists come from the one reader of a `className` the markup gates share.

export type ClassMeeting = {
  exact: readonly string[];
  /** Open ends such as `stat-card-narrow-`, from a token with a computed part. */
  prefixes: readonly string[];
  /** The element's tag when it is a plain HTML element. */
  tag: string | undefined;
};

// A class list can branch a lot; the reader throws past this, and the census fails loudly.
const BRANCHES = 256;

const tagOf = (attribute: ts.Node): string | undefined => {
  const element = attribute.parent.parent;
  if (!ts.isJsxOpeningElement(element) && !ts.isJsxSelfClosingElement(element))
    return undefined;
  const tag = element.tagName.getText();
  return /^[a-z]/.test(tag) ? tag : undefined;
};

function meetingOf(tag: string | undefined, tokens: string[]): ClassMeeting {
  const open = tokens.filter((token) => token.includes("*"));
  return {
    exact: tokens.filter((token) => !token.includes("*")),
    prefixes: open.flatMap((token) =>
      token.endsWith("*") && !token.slice(0, -1).includes("*")
        ? [token.slice(0, -1)]
        : [],
    ),
    tag,
  };
}

/** Each class list the file's `className`s can produce, one per branch. */
export function classMeetings(path: string, text: string): ClassMeeting[] {
  if (!text.includes("className")) return [];
  const source = parseSource(path, text);
  const meetings = new Map<string, ClassMeeting>();
  const walk = (node: ts.Node) => {
    if (ts.isJsxAttribute(node) && node.name.getText() === "className") {
      const value = node.initializer;
      const read =
        value !== undefined && ts.isJsxExpression(value)
          ? value.expression
          : value;
      for (const tokens of classVariants(source, read, BRANCHES)) {
        const meeting = meetingOf(tagOf(node), tokens);
        if (meeting.exact.length + meeting.prefixes.length > 1) {
          meetings.set(JSON.stringify(meeting), meeting);
        }
      }
    }
    ts.forEachChild(node, walk);
  };
  walk(source);
  return [...meetings.values()];
}

/**
 * Each member, and beside it the same member carrying every class a markup
 * site lists next to one of its own, typed as that site's element. An open
 * prefix adds the sheet classes it reaches. A bare slot such as `panel-${tone}`
 * adds none: it cannot tell a variant from a part like `panel-head`.
 */
export function withMeetings(
  members: readonly Written[],
  meetings: readonly ClassMeeting[],
  known: ReadonlySet<string>,
): Written[] {
  const joined = members.flatMap((member) =>
    meetings
      .filter((meeting) => meeting.exact.some((name) => member.own.has(name)))
      .map((meeting) => {
        const reached = [...known].filter((name) =>
          meeting.prefixes.some(
            (prefix) =>
              name.startsWith(prefix) &&
              ![...member.own].some((own) => prefix === `${own}-`),
          ),
        );
        const names = [...meeting.exact, ...reached];
        return {
          ...member,
          type: member.type || (meeting.tag ?? ""),
          own: new Set([...member.own, ...names]),
          ownSimples: new Set([
            ...member.ownSimples,
            ...names.map((name) => `.${name}`),
          ]),
        };
      }),
  );
  return [...members, ...joined];
}
