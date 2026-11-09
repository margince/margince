// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Source-wide behaviour promises: reduced motion, one pending vocabulary, one
// focus ring, server refusals kept whole, and one mount for each app-wide
// provider.

import { selectorList } from "../../scripts/lib/css-rules";
import { byCodeUnit } from "../../src/lib/codeunitorder";
import { withoutComments } from "../../src/testing/css";
import type { CensusFile, CensusRule } from "../tree";

type MotionRule = { selector: string; property: string; endsAt: number };
type PlainRule = { selector: string; body: string; at: number };

const selectorsOf = (text: string) =>
  selectorList(text).map((one) => one.replace(/\s+/g, " "));

// The offset after the brace that closes the block opened before `from`.
function blockEnd(text: string, from: number): number {
  let depth = 1;
  let index = from;
  while (index < text.length && depth > 0) {
    if (text[index] === "{") depth += 1;
    if (text[index] === "}") depth -= 1;
    index += 1;
  }
  return index;
}

/**
 * The rules inside every `prefers-reduced-motion: reduce` block of one
 * stylesheet, as (selector, property) pairs with the offset they close at.
 * A hand scan: these sheets are biome-formatted, and the media block is the one
 * nesting that occurs.
 */
export function reducedMotionRules(source: string): MotionRule[] {
  const text = withoutComments(source);
  const out: MotionRule[] = [];
  const opener = /@media[^{]*prefers-reduced-motion:\s*reduce[^{]*\{/g;
  for (let match = opener.exec(text); match; match = opener.exec(text)) {
    const start = match.index + match[0].length;
    const index = blockEnd(text, start);
    const body = text.slice(start, index - 1);
    const rule = /([^{}]+)\{([^{}]*)\}/g;
    for (let inner = rule.exec(body); inner; inner = rule.exec(body)) {
      const properties = inner[2]
        .split(";")
        .map((line) => line.split(":")[0].trim())
        .filter((name) => /^[a-z-]+$/.test(name));
      for (const selector of selectorsOf(inner[1])) {
        for (const property of properties) {
          out.push({ selector, property, endsAt: index });
        }
      }
    }
  }
  return out;
}

// Every at-rule block blanked to spaces, so offsets still line up. A regex that
// skipped media blocks by anchoring on the `}` before them matched only every
// other rule.
function withoutAtRuleBlocks(text: string): string {
  const out = text.split("");
  const opener = /@[a-z-]+[^{]*\{/g;
  for (let match = opener.exec(text); match; match = opener.exec(text)) {
    const index = blockEnd(text, match.index + match[0].length);
    for (let blank = match.index; blank < index; blank += 1) {
      if (out[blank] !== "\n") out[blank] = " ";
    }
    opener.lastIndex = index;
  }
  return out.join("");
}

/** Every top-level rule in a stylesheet, with where its selector starts. */
export function plainRules(text: string): PlainRule[] {
  const out: PlainRule[] = [];
  const rule = /([^{}]+)\{([^{}]*)\}/g;
  const flat = withoutAtRuleBlocks(withoutComments(text));
  for (let match = rule.exec(flat); match; match = rule.exec(flat)) {
    // Where the selector starts: the match also takes the blanked at-rule
    // block before it, which would place the rule ahead of that block.
    const at = match.index + match[1].length - match[1].trimStart().length;
    for (const selector of selectorsOf(match[1])) {
      out.push({ selector, body: match[2], at });
    }
  }
  return out;
}

/**
 * Reduced-motion rules a later plain rule overrides. A media query adds no
 * specificity, so document order settles the tie and the reduce rule loses.
 */
export function defeatedReduceRules(rel: string, text: string): string[] {
  const plain = plainRules(text);
  return reducedMotionRules(text)
    .filter(({ selector, property, endsAt }) =>
      plain.some(
        (rule) =>
          rule.selector === selector &&
          rule.at > endsAt &&
          new RegExp(`(^|;|\\s)${property}\\s*:`).test(rule.body),
      ),
    )
    .map(
      ({ selector, property }) =>
        `${rel}: the reduced-motion rule for \`${selector}\` sets \`${property}\`, ` +
        `and \`${selector}\` sets it again further down the file at the same ` +
        `specificity, so the reduced-motion rule loses. Move it after the rule it removes.`,
    );
}

/**
 * Infinite animations no reduced-motion rule names, on the selector or an
 * ancestor. `data-motion` is the convention for a component that switches its
 * own motion in JS, and its own suite pins it.
 */
export function unansweredAnimations(rel: string, text: string): string[] {
  if (/data-motion/.test(text)) return [];
  const covered = new Set(
    reducedMotionRules(text).map((rule) => rule.selector),
  );
  return plainRules(text)
    .filter((rule) => /animation[^;]*\binfinite\b/.test(rule.body))
    .filter(
      (rule) =>
        ![...covered].some(
          (named) =>
            named === rule.selector || rule.selector.startsWith(`${named} `),
        ),
    )
    .map(
      (rule) =>
        `${rel}: \`${rule.selector}\` runs an infinite animation and no reduced-motion rule names it or an ancestor of it`,
    );
}

const withoutCssComments = (text: string) =>
  text.replace(/\/\*[\s\S]*?\*\//g, "");

/** Focus outlines spelled by hand rather than from `--focus-ring`. */
export function handSpelledOutlines(rel: string, text: string): string[] {
  // `none` and `0` are the suppression half: something else draws the ring.
  return [...withoutCssComments(text).matchAll(/outline:\s*([^;]+);/g)]
    .map((match) => match[1].trim())
    .filter((value) => !/^(none|0)$/.test(value))
    .filter((value) => !/var\(--focus-ring(-forced)?\)/.test(value))
    .map((value) => `${rel}: outline: ${value}`);
}

/** Focus glows spelled by hand, on a focus selector only. */
export function handSpelledGlows(rel: string, text: string): string[] {
  return [
    ...withoutCssComments(text).matchAll(/:focus[^{]*\{([^}]*)\}/g),
  ].flatMap((rule) =>
    [...rule[1].matchAll(/box-shadow:\s*([^;]+);/g)]
      .map((match) => match[1].trim())
      .filter((value) => !/^(none|0)$/.test(value))
      .filter((value) => !/var\(--focus-glow(-danger|-ai)?\)/.test(value))
      .map((value) => `${rel}: box-shadow: ${value}`),
  );
}

/** Line numbers of a type class restated on the sentence inside EmptyState. */
export function emptyPlateOverrides(rel: string, text: string): string[] {
  const hits = text.matchAll(
    /<EmptyState[^>]*>\s*<(?:p|span|div|blockquote)\s[^>]*className="[^"]*\bt-(?:small|caption|meta|body)\b/g,
  );
  return [...hits].map(
    (hit) => `${rel}:${text.slice(0, hit.index).split("\n").length}`,
  );
}

/** A list that must equal `expected`, as one finding per extra or missing entry. */
export function onlyAt(
  found: readonly string[],
  expected: readonly string[],
  why: string,
): string[] {
  return [
    ...found
      .filter((entry) => !expected.includes(entry))
      .map((entry) => `${entry}: ${why}`),
    ...expected
      .filter((entry) => !found.includes(entry))
      .map((entry) => `${entry} is expected and was not found: ${why}`),
  ];
}

const isCss = (file: CensusFile) => file.rel.endsWith(".css");
const isTsx = (file: CensusFile) => file.rel.endsWith(".tsx");
const relsWhere = (
  files: readonly CensusFile[],
  test: (text: string) => boolean,
) =>
  files
    .filter((file) => test(file.text))
    .map((file) => file.rel)
    .sort(byCodeUnit);

// The answer to "where does X mount" has one home, above the screens. A second
// mount is a second queue, guard or dialog that unmounts with its own screen.
function oneMountEach(corpus: readonly CensusFile[]): string[] {
  const tsx = corpus
    .filter(isTsx)
    .filter((file) => !/\.(test|stories)\.tsx$/.test(file.rel));
  const shipped = tsx.filter((file) => !file.rel.endsWith(".testkit.tsx"));
  const mountsIn = (files: readonly CensusFile[], tag: string) =>
    relsWhere(files, (text) => new RegExp(`<${tag}[\\s/>]`).test(text));
  const mounts = (tag: string) => mountsIn(shipped, tag);
  return [
    ...onlyAt(
      relsWhere(tsx, (text) =>
        /aria-busy=(?:["']true["']|\{\s*true\s*\})/.test(text),
      ),
      ["src/design-system/atoms.tsx"],
      "a pending region comes from PendingBody in src/design-system/atoms.tsx",
    ),
    ...onlyAt(
      mounts("UnsavedGuard"),
      ["src/App.views.tsx"],
      "render the one guard in App.views.tsx and claim with `useUnsavedGuard`",
    ),
    ...onlyAt(
      mounts("ToastProvider"),
      ["src/main.tsx"],
      "a second provider is a second queue; mount the one in main.tsx",
    ),
    ...onlyAt(
      mounts("ToastRegion"),
      ["src/main.tsx"],
      "call `useToast().show` and let the region in main.tsx draw it",
    ),
    ...onlyAt(
      mountsIn(tsx, "FilePreviewProvider"),
      ["src/main.tsx"],
      "mount the one in main.tsx and open it with `useFilePreview`",
    ),
  ];
}

function stylesheetPromises(corpus: readonly CensusFile[]): string[] {
  const css = corpus.filter(isCss);
  const sheets = css.filter((file) => !file.rel.endsWith("tokens.css"));
  return [
    ...css.flatMap((file) => defeatedReduceRules(file.rel, file.text)),
    ...css.flatMap((file) => unansweredAnimations(file.rel, file.text)),
    // `.skeleton` carries the pulse; a second placeholder wears that class.
    ...onlyAt(
      css.flatMap((file) =>
        plainRules(file.text)
          .filter((rule) => /animation[^;]*\bds-pulse\b/.test(rule.body))
          .map((rule) => `${file.rel} ${rule.selector}`),
      ),
      ["src/design-system/atoms.css .skeleton"],
      "a placeholder carries `.skeleton` and adds only its own geometry",
    ),
    // The three outlines that are not focus: two drop targets and one refused
    // field on the sign-in surface.
    ...onlyAt(
      sheets.flatMap((file) => handSpelledOutlines(file.rel, file.text)),
      [
        "src/components/auth-core.css: outline: 2px solid var(--danger)",
        "src/design-system/composed.css: outline: 2px dashed var(--accent)",
        "src/features/onboarding/onboarding-conversation/conversation.css: outline: 2px dashed var(--aiMed)",
      ],
      "a focus ring reads `outline: var(--focus-ring)`, or `var(--focus-ring-forced)` where a glow draws it",
    ),
    ...sheets.flatMap((file) =>
      handSpelledGlows(file.rel, file.text).map(
        (line) =>
          `${line}: a focus glow reads \`var(--focus-glow)\` or its \`-danger\` / \`-ai\` variant`,
      ),
    ),
    ...onlyAt(
      relsWhere(css, (text) => /--phoneNavClearance/.test(text)),
      ["src/app/shell.css", "src/design-system/tokens.css"],
      "read var(--stickyBottomInset); only the shell knows where the nav bar is",
    ),
  ];
}

function sourcePromises(corpus: readonly CensusFile[]): string[] {
  const modules = corpus
    .filter((file) => /\.tsx?$/.test(file.rel))
    .filter((file) => !/\.(test|stories)\.tsx?$/.test(file.rel));
  return [
    ...modules
      .filter((file) =>
        /throw new Error\(\s*problemMessage\(/.test(
          file.text.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, ""),
        ),
      )
      .map(
        (file) =>
          `${file.rel}: throws a sentence about the problem; \`throwProblem(error, t)\` keeps the ProblemError and its \`details.errors[]\``,
      ),
    ...modules
      .filter(isTsx)
      .flatMap((file) => emptyPlateOverrides(file.rel, file.text))
      .map(
        (at) =>
          `${at}: hand the sentence to EmptyState directly rather than wrapping it in a type class`,
      ),
  ];
}

export const rule: CensusRule = {
  name: "conformance.behaviour",
  corpus: (tree) =>
    tree.files.filter(
      (file) =>
        (file.area === "src" || file.area === "extensions") &&
        /\.(css|tsx?)$/.test(file.path),
    ),
  floors: (corpus) =>
    corpus.filter(isCss).length > 20
      ? []
      : ["fewer than 20 stylesheets were read"],
  across: (corpus) => [
    ...stylesheetPromises(corpus),
    ...oneMountEach(corpus),
    ...sourcePromises(corpus),
  ],
  advice:
    "Each finding names the promise it breaks: reduced motion, one pending " +
    "region, one placeholder pulse, one focus ring and glow token, a server " +
    "problem thrown whole, one mount for each app-wide provider, and the " +
    "empty plate sizing its own sentence.",
};
