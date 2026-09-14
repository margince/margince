// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * What a stylesheet DECLARES, for the gates that read one.
 *
 * Every CSS gate in this tree opens with the same problem: the property it
 * hunts for appears in the paragraph explaining the rule at least as often as
 * in the rule, and a commented-out declaration declares nothing. Three of them
 * were each carrying their own copy of the answer.
 */

/**
 * The same text with every comment blanked to spaces of its own length.
 *
 * Blanked rather than deleted, so every offset into the result still points at
 * the same character of the original: a gate that reads an in-line waiver finds
 * it in the comment beside the declaration, which it can only do while the two
 * texts still line up.
 */
export function withoutComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, (comment) =>
    " ".repeat(comment.length),
  );
}

/**
 * One CSS rule: its selector, its OWN declarations, and where it starts.
 *
 * `parents` are the selectors of the style rules it is nested inside,
 * outermost first. At-rules are not among them: a breakpoint scopes a rule
 * without naming what it selects. A gate that asks what a nested rule TARGETS
 * reads it through them; one that asks only what a rule says can ignore them.
 */
export type CssRule = {
  selector: string;
  parents: string[];
  body: string;
  raw: string;
  line: number;
};

/**
 * The rules in one sheet, read by brace-matching rather than by regex over the
 * whole file — a regex spanning `{...}` cannot tell a rule from the gap between
 * two. At-rules (`@media`, `@supports`) are descended into rather than
 * skipped: a copy inside a breakpoint is a copy. A nested rule's declarations
 * belong to the nested rule, not to its parent.
 *
 * `body` has comments blanked (line breaks kept, so a reported line still
 * points at the rule); `raw` keeps them, because the waiver lives in one.
 * `line` is the line of the rule's opening brace and `raw` starts just after
 * it, so a line counted inside `raw` is an offset from `line`.
 */
export function rulesIn(source: string): CssRule[] {
  const css = source.replaceAll(/\/\*[\s\S]*?\*\//g, (block) =>
    block.replaceAll(/[^\n]/g, " "),
  );
  const out: CssRule[] = [];
  let selectorStart = 0;
  const opens: { selector: string; at: number; line: number }[] = [];
  for (let i = 0; i < css.length; i++) {
    if (css[i] === "{") {
      opens.push({
        selector: css.slice(selectorStart, i).trim(),
        at: i + 1,
        line: css.slice(0, i).split("\n").length,
      });
      selectorStart = i + 1;
    } else if (css[i] === "}") {
      const open = opens.pop();
      if (open && !open.selector.startsWith("@")) {
        out.push({
          selector: open.selector,
          parents: opens
            .map((enclosing) => enclosing.selector)
            .filter((selector) => !selector.startsWith("@")),
          body: css.slice(open.at, i).replaceAll(/\{[^{}]*\}/g, ""),
          raw: source.slice(open.at, i),
          line: open.line,
        });
      }
      selectorStart = i + 1;
    } else if (css[i] === ";") {
      // Inside a rule this ends a declaration, and what follows up to the next
      // `{` is a NESTED rule's selector; at the top it ends an at-rule.
      selectorStart = i + 1;
    }
  }
  return out;
}
