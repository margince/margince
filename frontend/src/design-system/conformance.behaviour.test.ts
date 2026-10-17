import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";
import {
  files,
  frontendRoot,
  scanBudget,
} from "../../scripts/lib/conformance-corpus";
import { selectorList } from "../../scripts/lib/css-rules";
import { sourceTextAt } from "../../scripts/lib/source-tree";
import { withoutComments } from "../testing/css";

// ---------------------------------------------------------------------------
// Motion, source-wide. A broken reduced-motion promise is invisible, so both
// gates below exist. The reader who asked for less motion is not the reader
// running the app in development, so nothing catches it by eye.
// ---------------------------------------------------------------------------

const cssFiles = files.filter((file) => file.endsWith(".css"));

/**
 * The rules inside every `prefers-reduced-motion: reduce` block of one
 * stylesheet, as (selector, property) pairs with the offset they close at.
 *
 * A hand-rolled scan rather than a parser. These stylesheets are
 * biome-formatted, so a rule is a selector list, `{`, declarations, `}`. The
 * one nesting that occurs is the media block itself. A parser would suit
 * arbitrary CSS. This scan has to survive only this tree, which the gate also
 * keeps formatted.
 */
function reducedMotionRules(
  source: string,
): { selector: string; property: string; endsAt: number }[] {
  const text = withoutComments(source);
  const out: { selector: string; property: string; endsAt: number }[] = [];
  const opener = /@media[^{]*prefers-reduced-motion:\s*reduce[^{]*\{/g;
  for (let match = opener.exec(text); match; match = opener.exec(text)) {
    // Walk to the matching close brace of the media block.
    let depth = 1;
    let index = match.index + match[0].length;
    const start = index;
    while (index < text.length && depth > 0) {
      if (text[index] === "{") depth += 1;
      if (text[index] === "}") depth -= 1;
      index += 1;
    }
    const body = text.slice(start, index - 1);
    const rule = /([^{}]+)\{([^{}]*)\}/g;
    for (let inner = rule.exec(body); inner; inner = rule.exec(body)) {
      const selectors = selectorList(inner[1]).map((one) =>
        one.replace(/\s+/g, " "),
      );
      const properties = inner[2]
        .split(";")
        .map((line) => line.split(":")[0].trim())
        .filter((name) => /^[a-z-]+$/.test(name));
      for (const selector of selectors) {
        for (const property of properties) {
          out.push({ selector, property, endsAt: index });
        }
      }
    }
  }
  return out;
}

/**
 * The stylesheet with every at-rule block blanked to spaces of the same length,
 * so offsets still line up with the original text.
 *
 * Blanking rather than deleting lets the two collectors here be compared by
 * position. It has to happen before rules are matched. A regex that skipped
 * media blocks by anchoring on the preceding `}` matched only every other rule.
 * The match before it consumes the brace it anchors on. That silently exempted
 * half of every stylesheet from both gates below.
 */
function withoutAtRuleBlocks(text: string): string {
  const out = text.split("");
  const opener = /@[a-z-]+[^{]*\{/g;
  for (let match = opener.exec(text); match; match = opener.exec(text)) {
    let depth = 1;
    let index = match.index + match[0].length;
    while (index < text.length && depth > 0) {
      if (text[index] === "{") depth += 1;
      if (text[index] === "}") depth -= 1;
      index += 1;
    }
    for (let blank = match.index; blank < index; blank += 1) {
      if (out[blank] !== "\n") {
        out[blank] = " ";
      }
    }
    opener.lastIndex = index;
  }
  return out.join("");
}

/** Every top-level rule in a stylesheet, with where its selector starts. */
function plainRules(
  text: string,
): { selector: string; body: string; at: number }[] {
  const out: { selector: string; body: string; at: number }[] = [];
  const rule = /([^{}]+)\{([^{}]*)\}/g;
  const flat = withoutAtRuleBlocks(withoutComments(text));
  for (let match = rule.exec(flat); match; match = rule.exec(flat)) {
    const selectors = selectorList(match[1]).map((one) =>
      one.replace(/\s+/g, " "),
    );
    for (const selector of selectors) {
      out.push({ selector, body: match[2], at: match.index });
    }
  }
  return out;
}

describe("motion", () => {
  // The defect this pins was live twice at once. `.rail .ws-name` and
  // `.accountsub` each had a reduce rule that a later plain rule overrode. The
  // two had identical specificity, because a media query adds none. So the
  // label kept animating its width and the theme flyout kept flying. It did so
  // for the readers who had asked them not to. Both stylesheets read as if the
  // promise were kept.
  it("declares every reduced-motion rule after the rule it has to beat", () => {
    for (const file of cssFiles) {
      const text = sourceTextAt(file);
      const plain = plainRules(text);
      for (const { selector, property, endsAt } of reducedMotionRules(text)) {
        const defeated = plain.find(
          (rule) =>
            rule.selector === selector &&
            rule.at > endsAt &&
            new RegExp(`(^|;|\\s)${property}\\s*:`).test(rule.body),
        );
        expect(
          defeated,
          `${relative(frontendRoot, file)}: the reduced-motion rule for \`${selector}\` ` +
            `sets \`${property}\`, and \`${selector}\` sets it again at the same ` +
            `specificity further down the file — document order settles the tie, so ` +
            `the reduced-motion rule loses. Move it after the rule it removes.`,
        ).toBeUndefined();
      }
    }
  });

  // An infinite animation is the one kind that cannot be waited out. A pulse
  // or a shimmer that nobody asked for runs while the surface is on screen.
  // The finite ones are a smaller promise and are tracked separately.
  it("gives every infinite animation a reduced-motion answer", () => {
    for (const file of cssFiles) {
      const text = sourceTextAt(file);
      // Per selector, not per file: a stylesheet that carries one reduce block
      // and three pulses would otherwise read as covered. The gate asks the
      // question a reader would: is this animation answered?
      const covered = new Set(
        reducedMotionRules(text).map((rule) => rule.selector),
      );
      for (const rule of plainRules(text)) {
        if (!/animation[^;]*\binfinite\b/.test(rule.body)) {
          continue;
        }
        // A component may switch its own motion in JS instead; `data-motion` is
        // the tree's convention for that and is pinned by its own suite.
        if (/data-motion/.test(text)) {
          continue;
        }
        // An ancestor counts, and is usually the better answer. The Core hides
        // the whole mote field (`.core-feed { display: none }`). It does not
        // stop twelve particles one by one, so nothing is left frozen
        // mid-flight.
        const answered = [...covered].some(
          (named) =>
            named === rule.selector || rule.selector.startsWith(`${named} `),
        );
        expect(
          answered,
          `${relative(frontendRoot, file)}: \`${rule.selector}\` runs an infinite ` +
            `animation and no reduced-motion rule names it or an ancestor of it`,
        ).toBe(true);
      }
    }
  });
});

// ---------------------------------------------------------------------------
// The pending vocabulary. Four spellings of "this is loading" grew here before
// they were collapsed into `PendingBody`. There were three inline-styled bars
// in QueryStates and one silent 32px bar in SurfaceState. ListTable had five
// unanimated bone rows. A long tail of hand-rolled bars and bare "Loading…"
// lines followed. They disagreed about the shape and the height. A reader
// could not see the last part: they disagreed about whether anything was
// announced at all. Three screens had bolted their own visually-hidden line
// beside a mute placeholder. That is the tell that the primitive was missing
// something, not that those screens were special.
//
// A fifth spelling is one `<div role="status" aria-busy="true">` away, and it
// looks correct in review. So the gate is that the announcement has one home.
// ---------------------------------------------------------------------------

describe("pending states", () => {
  const PENDING_HOME = join("src", "design-system", "atoms.tsx");

  it("announces a pending region from exactly one component", () => {
    const declared = files
      .filter((file) => file.endsWith(".tsx"))
      .filter((file) => !/\.(test|stories)\.tsx$/.test(file))
      // Both static spellings. Matching only the quoted one let
      // `aria-busy={true}` declare a second pending region and pass. It is
      // the same unconditional claim, written the other way.
      .filter((file) =>
        /aria-busy=(?:["']true["']|\{\s*true\s*\})/.test(sourceTextAt(file)),
      )
      .map((file) => relative(frontendRoot, file));

    // A control's own busy state is a different fact. It reads from a variable
    // (`aria-busy={busy || undefined}`), so it never matches the literal
    // above. Button and Switch need no exception here: they are not pending
    // regions.
    expect(
      declared,
      `a pending region must come from PendingBody (${PENDING_HOME}); ` +
        `these files declare one of their own: ${declared.join(", ")}`,
    ).toEqual([PENDING_HOME]);
  });

  it("keeps the placeholder pulse to one selector", () => {
    // `.lt-bone` was the tell. It painted its own fill and radius and forgot
    // the animation. So a table's five placeholder rows were the one mark in
    // the product that did not move. They looked like a list that had answered
    // with five blank rows. It also missed `.skeleton`'s reduced-motion
    // answer, which the gate above can only check for rules that exist.
    //
    // CSS cannot say whether a given box is "a placeholder". So this asks the
    // answerable half: the pulse belongs to one class. A second placeholder
    // has to carry that class rather than restate it. Restating it is what
    // drops half the behaviour.
    const wearers = cssFiles.flatMap((file) =>
      plainRules(sourceTextAt(file))
        .filter((rule) => /animation[^;]*\bds-pulse\b/.test(rule.body))
        .map((rule) => `${relative(frontendRoot, file)} ${rule.selector}`),
    );
    expect(
      wearers,
      "the placeholder pulse has more than one home; a placeholder carries " +
        "`.skeleton` and adds only its own geometry",
    ).toEqual(["src/design-system/atoms.css .skeleton"]);
  });
});

// ---------------------------------------------------------------------------
// The focus ring. Ten spellings of one promise grew here. Some used 2px and
// some 3px. Most files used `--accent`; three used a transparent outline plus
// a glow. Two controls suppressed the ring and put nothing back. That is a
// WCAG 2.4.7 failure that reads, in a diff, as tidying.
//
// The width and the colour are now tokens. The offset stays per-component: a
// control clipped by its own container has to draw the ring inside itself or
// lose half of it. So the gate asks about the half that is a promise.
// ---------------------------------------------------------------------------

describe("focus", () => {
  it("draws every focus ring from the one token", () => {
    const spelled = cssFiles.flatMap((file) => {
      if (file.endsWith("tokens.css")) {
        return [];
      }
      // Strip comments first, or a sentence about an outline is read as one.
      // The conversation sheet explains why it leaves `outline: 0` unset. The
      // earlier form of this gate reported that paragraph as a finding.
      const text = sourceTextAt(file).replace(/\/\*[\s\S]*?\*\//g, "");
      return (
        [...text.matchAll(/outline:\s*([^;]+);/g)]
          .map((match) => match[1].trim())
          // `none` and `0` are the suppression half of the pattern. They are
          // legitimate wherever something else draws the ring: a field's own
          // boundary, or a wrapper's `:focus-within`. Spelling a new ring by
          // hand is not legitimate.
          .filter((value) => !/^(none|0)$/.test(value))
          .filter((value) => !/var\(--focus-ring(-forced)?\)/.test(value))
          .map((value) => `${relative(frontendRoot, file)}: outline: ${value}`)
      );
    });
    expect(
      spelled,
      "a focus ring reads `outline: var(--focus-ring)`, or " +
        "`var(--focus-ring-forced)` where a glow does the drawing; these spell " +
        "their own width and colour, which is how one promise became ten rules",
    ).toEqual([
      // The outlines in this tree that are not focus, and must not read as it.
      // Two say "you may drop here". They are dashed, and an outline rather
      // than a border, so accepting a drag does not resize the drop target.
      // One marks a refused field on the sign-in surface, which is a
      // different fact from where the keyboard is.
      "src/design-system/composed.css: outline: 2px dashed var(--accent)",
      "src/screens/auth.css: outline: 2px solid var(--danger)",
      "src/screens/onboarding-conversation/conversation.css: outline: 2px dashed var(--aiMed)",
    ]);
  });

  // The other half of the same promise, and the half that was unwatched. The
  // rule above matches `outline:` only. A field's ring is drawn as a
  // box-shadow, which is what --focus-glow is. So six rules across the
  // onboarding sheets hand-spelled `0 0 0 3px var(--aiMed)`. They walked past
  // a gate whose message says one promise must not become ten rules.
  //
  // Only shadows on a focus selector count: a card's resting elevation is a
  // box-shadow too and is nobody's ring.
  it("draws every focus glow from the one token", () => {
    const spelled = cssFiles.flatMap((file) => {
      if (file.endsWith("tokens.css")) {
        return [];
      }
      const text = sourceTextAt(file).replace(/\/\*[\s\S]*?\*\//g, "");
      return [...text.matchAll(/:focus[^{]*\{([^}]*)\}/g)].flatMap((rule) =>
        [...rule[1].matchAll(/box-shadow:\s*([^;]+);/g)]
          .map((match) => match[1].trim())
          .filter((value) => !/^(none|0)$/.test(value))
          .filter((value) => !/var\(--focus-glow(-danger|-ai)?\)/.test(value))
          .map(
            (value) => `${relative(frontendRoot, file)}: box-shadow: ${value}`,
          ),
      );
    });
    expect(
      spelled,
      "a focus glow reads `box-shadow: var(--focus-glow)`, or its " +
        "`-danger` / `-ai` variant; these spell their own width and colour",
    ).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Server refusals. `ProblemError` tells a refusal the server meant from a bug
// worth logging. The global failure sink stays quiet for one and
// `console.error`s anything else. Only a ProblemError carries the RFC-7807
// `details.errors[]` a form needs to put a refusal on the field it is about.
//
// Eleven query and mutation functions threw `new Error(problemMessage(error))`
// instead. Each one flattened the answer to a sentence. So every refused save
// at those sites was also reported as an unexpected error. The per-field
// assertions the API already sends were unreachable. The wrapper reads as
// careful (it does produce a readable message), so only a gate keeps it out.
// ---------------------------------------------------------------------------

describe("problem details", () => {
  it("throws the server's problem, never a sentence about it", () => {
    const flattened = files
      .filter((file) => /\.tsx?$/.test(file))
      .filter((file) => !/\.(test|stories)\.tsx?$/.test(file))
      .flatMap((file) => {
        const text = sourceTextAt(file).replace(
          /\/\*[\s\S]*?\*\/|\/\/[^\n]*/g,
          "",
        );
        return /throw new Error\(\s*problemMessage\(/.test(text)
          ? [relative(frontendRoot, file)]
          : [];
      });
    expect(
      flattened,
      "`throwProblem(error, t)` keeps the ProblemError: without it the global " +
        "sink logs a server refusal as a bug, and `details.errors[]` — the only " +
        "thing that can put a 422 on the field it is about — is discarded",
    ).toEqual([]);
  });
});

describe("unsaved edits", () => {
  it("installs the navigation guard above the screens, never inside one", () => {
    // `UnsavedGuard` can only hold the moves it is still mounted for. Inside a
    // screen it guarded that screen's own tabs and nothing else. A settings
    // draft was safe from one entry to the next. It was still discarded
    // without a word when the reader clicked Contacts. The screen holding the
    // guard unmounted before it could ask. So one place renders it, above the
    // routed screen. A draft anywhere below claims through `useUnsavedGuard`,
    // which is scope-free by design.
    const wearers = files
      .filter((file) => /\.tsx$/.test(file))
      .filter((file) => !/\.(test|stories|testkit)\.tsx$/.test(file))
      .filter((file) => /<UnsavedGuard[\s>]/.test(sourceTextAt(file)))
      .map((file) => relative(frontendRoot, file))
      .sort();
    expect(
      wearers,
      "a second guard is a guard that unmounts with its own screen; render " +
        "the one in App.views.tsx and claim from the card with `useUnsavedGuard`",
    ).toEqual(["src/App.views.tsx"]);
  });
});

describe("the transient confirmation", () => {
  // The defect this is derived from was real, and nothing caught it.
  // `screens/commissiondecide.tsx` called `useToast` and rendered no region.
  // So every approve/pay/void confirmation it showed went into a `useState`
  // nobody read. Nothing failed: the suite only rendered the toast's own
  // harness. A confirmation shown to nobody looks the same as one nobody
  // triggered.
  //
  // The fix has `UnsavedGuard`'s shape: one mount, above the screens, held
  // here. A second region would be a second answer to "where does a
  // confirmation appear". A screen mounting its own is back to the state above.
  it("mounts one provider and one region, above the screens", () => {
    const mounts = (tag: string) =>
      files
        .filter((file) => /\.tsx$/.test(file))
        .filter((file) => !/\.(test|stories|testkit)\.tsx$/.test(file))
        .filter((file) => new RegExp(`<${tag}[\\s/>]`).test(sourceTextAt(file)))
        .map((file) => relative(frontendRoot, file))
        .sort();
    expect(
      mounts("ToastProvider"),
      "a second provider is a second queue, and a screen under the wrong one " +
        "shows its confirmations to nobody; mount the one in main.tsx",
    ).toEqual(["src/main.tsx"]);
    expect(
      mounts("ToastRegion"),
      "a screen rendering its own region is the defect this gate exists for; " +
        "call `useToast().show` and let the region in main.tsx draw it",
    ).toEqual(["src/main.tsx"]);
  });
});

describe("the opened file", () => {
  // `FilePreview` holds the file a reader clicked. A second provider is a
  // second answer to "where does a file open". Say two are mounted and a
  // reader opens a file from a surface under the inner one. That gives two
  // `aria-modal` dialogs and two Escape handlers. `OpenEmailDrawer` exists to
  // prevent that failure; here it arrives by a different route.
  //
  // The mount is also what makes the chip's click mean anything. `FileChip`
  // reads the provider through context and falls back to the plain download
  // when there is none. So if the provider stopped being an ancestor of the
  // screens, the capability would vanish without failing anything.
  it("mounts one provider, above the screens", () => {
    const mounts = files
      .filter((file) => /\.tsx$/.test(file))
      .filter((file) => !/\.(test|stories)\.tsx$/.test(file))
      .filter((file) => /<FilePreviewProvider[\s/>]/.test(sourceTextAt(file)))
      .map((file) => relative(frontendRoot, file))
      .sort();
    expect(
      mounts,
      "a second provider is a second dialog over the same click; mount the " +
        "one in main.tsx and open it with `useFilePreview`",
    ).toEqual(["src/main.tsx"]);
  });
});

describe("the phone nav's clearance", scanBudget, () => {
  // The nav bar belongs to the shell, so where it is belongs to the shell too.
  // Three sheets used to keep their own answer: the record action bar at
  // 720px, the deck tray at 640px, the scroll padding at 700px. Three answers
  // were wrong at every width where they disagreed. The tray hid behind the
  // nav from 641px to 700px. The record page reserved room for a bar that is
  // gone above 700px. The shell now publishes `--stickyBottomInset` at the
  // widths where it draws the bar, and everything sticky reads that. Two
  // files may name the clearance itself: tokens.css declares it, and
  // shell.css is the one that knows when it applies.
  it("is read through --stickyBottomInset everywhere but the shell", () => {
    const readers = files
      .filter((file) => /\.css$/.test(file))
      .filter((file) => /--phoneNavClearance/.test(sourceTextAt(file)))
      .map((file) => relative(frontendRoot, file))
      .sort();
    expect(
      readers,
      "a sticky element that names --phoneNavClearance is a second opinion " +
        "about where the nav bar is; read var(--stickyBottomInset) instead",
    ).toEqual(["src/app/shell.css", "src/design-system/tokens.css"]);
  });
});

// ---------------------------------------------------------------------------
// The empty plate's own type. `EmptyState` draws the sentence a surface shows
// instead of content, and `.empty` sizes it. The one-liner is 13px. The
// instructional variant's measured paragraph is 14px, in `.empty-body`.
// Thirty-one call sites wrapped that sentence in `<p className="t-caption">`
// and drew it at 12px instead. That is the same sentence at two sizes,
// depending on which screen a reader is on.
//
// Nothing made them wrong at the time: the primitive took `children` and any
// node was as valid as any other. So the rule is stated here, not in the prop
// type. What is forbidden is a class on a child, which TypeScript cannot
// describe.
// ---------------------------------------------------------------------------

describe("the empty plate", () => {
  it("sizes its own sentence, and no call site restates that", () => {
    const overrides = files
      .filter((file) => file.endsWith(".tsx"))
      .filter((file) => !/\.(test|stories)\.tsx$/.test(file))
      .flatMap((file) => {
        const source = sourceTextAt(file);
        // The child immediately inside the opening tag, whitespace and
        // newlines crossed. A type class further down the tree may be an
        // intended second line, such as the deck's cleared-at timestamp. Only
        // the element that is the sentence is in scope here.
        const hits = source.matchAll(
          /<EmptyState[^>]*>\s*<(?:p|span|div|blockquote)\s[^>]*className="[^"]*\bt-(?:small|caption|meta|body)\b/g,
        );
        return [...hits].map(
          () =>
            `${relative(frontendRoot, file)}: ${
              source.slice(0, source.indexOf("<EmptyState")).split("\n").length
            }`,
        );
      });
    expect(
      overrides,
      "hand the sentence to EmptyState directly — `<EmptyState>{t(…)}` — " +
        "rather than wrapping it in a type class, which draws one surface's " +
        "empty state a step smaller than every other surface's",
    ).toEqual([]);
  });
});
