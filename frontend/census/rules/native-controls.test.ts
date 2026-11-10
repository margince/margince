// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  existsSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterAll, describe, expect, it } from "vitest";
import { deriveDateHosts } from "../../scripts/lib/input-type";
import { plantedFindings, srcDir } from "../tree";
import {
  clearedSpreads,
  dateHosts,
  findNativeControls,
  nativeControls,
  notHosts,
  prefilter,
  rule,
  worthParsing,
} from "./native-controls.census";

// A template substitution, built rather than written: a literal `${…}` inside
// one of the probe sources below reads to biome as a template curly somebody
// forgot to make a template, and it is right to say so — these are deliberate,
// and building them says which.
const SUBST = "$" + "{id}";

describe("the files the gate points authors at", () => {
  for (const replacement of ["dateinput", "timeinput", "monthinput"]) {
    it(`holds ${replacement}.tsx to the same rule as everything else`, () => {
      const path = join(srcDir, "design-system", `${replacement}.tsx`);
      expect(existsSync(path)).toBe(true);
      expect(findNativeControls(path, readFileSync(path, "utf8"))).toEqual([]);
    });
  }

  it("holds select.tsx to the same rule as everything else", () => {
    // The file this gate points contacts AT is scanned like any other, and it
    // passes — which is the evidence that the shell gate's exemption was
    // unnecessary rather than merely unused. Its `<select>` mentions are
    // comments citing what it replaces, and a parser drops those for free.
    const path = join(srcDir, "design-system", "select.tsx");
    expect(existsSync(path)).toBe(true);
    expect(findNativeControls(path, readFileSync(path, "utf8"))).toEqual([]);
  });
});

// The half that makes the census mean anything. Each case is a shape the rule
// must judge, so a detector that stopped detecting fails here and not in silence.
describe("the native-control detector sees what it claims to", () => {
  // `expect` is the report LINE, not merely "something fired". Four separate
  // line-number rules survived mutation because every case asserted only
  // `hits.length > 0` — including the one NAMED "reports ITS line". A case
  // that cannot fail on the thing it is named for is the defect this file
  // exists to stop shipping.
  const cases: {
    name: string;
    fires: boolean;
    src: string;
    file?: string;
    expect?: string[];
  }[] = [
    { name: "a native select", fires: true, src: "const a = <select />;" },
    {
      name: "an option",
      fires: true,
      src: "const a = <div><option>x</option></div>;",
    },
    {
      name: "an optgroup",
      fires: true,
      src: "const a = <optgroup label='g' />;",
    },
    {
      name: "attributes wrapped onto the next line",
      fires: true,
      src: "const a = (\n  <select\n    id='x'\n  />\n);",
    },
    // The parser tells an intrinsic element from a component by KIND, so the
    // design-system control is not a hit and needs no naming rule.
    {
      name: "the design-system Select",
      fires: false,
      src: "const a = <Select />;",
    },
    {
      name: "a tag merely starting with the same letters",
      fires: false,
      src: "const a = <selectable />;",
    },
    // Comments are not AST nodes, so a cross-reference costs nothing — which is
    // thirty lines of hand-written comment stripping the shell gate needed.
    {
      name: "a cross-reference in a line comment",
      fires: false,
      src: "// replaced <select> with Select\nconst a = 1;",
    },
    {
      name: "a cross-reference in a block comment",
      fires: false,
      src: "/* was <select /> */\nconst a = 1;",
    },
    {
      name: "a cross-reference in a JSX comment",
      fires: false,
      src: "const a = <div>{/* was <select /> */}</div>;",
    },
    // The residues the shell gate's header had to enumerate, each now a
    // property of the parser rather than of the scanner.
    {
      name: "a URL, whose // is not a comment",
      fires: true,
      src: "const u = 'https://h/a//b';\nconst a = <select />;",
    },
    {
      name: "a multi-line template literal followed by markup",
      fires: true,
      src: "const t = `line one\n// not a comment\n`;\nconst a = <select />;",
    },
    {
      name: "an unterminated quote does not blank the file",
      fires: true,
      src: "const s = 'oops;\nconst a = <select />;",
    },
    // Markup built in a string ships, so it is a hit — deliberately, and this
    // is the one rule the parser does not decide.
    {
      name: "markup built in a template literal",
      fires: true,
      src: "const html = `<select><option>x</option></select>`;",
    },
    {
      name: "prose in a string that merely mentions selection",
      fires: false,
      src: "const s = 'pick a selection';",
    },
    // A .ts file has no JSX, and asking for it would be a parse error rather
    // than a finding.
    // A .js file holding JSX is JSX. Parsed as TS it yields no nodes at all,
    // which is a silent miss rather than a false negative you can see.
    {
      name: "JSX in a plain .js file",
      fires: true,
      src: "export const A = () => <select />;",
      file: "probe.js",
    },
    // A hit inside a multi-line template reports ITS line, not the template's.
    {
      name: "a select on the second line of a template",
      fires: true,
      src: "const html = `<form>\n  <select>\n</form>`;",
      expect: ["2: <select> inside a string"],
    },
    {
      name: "a plain single-quoted string holding markup",
      fires: true,
      src: "const html = '<select>';",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "a template with a substitution",
      fires: true,
      // biome-ignore lint/style/useTemplate: a template literal cannot hold the backticks this probe source is made of, and SUBST exists so the file never spells a live ${.
      src: "const html = `<select id=" + SUBST + ">`;",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "a substitution's TAIL, on its own line",
      fires: true,
      // biome-ignore lint/style/useTemplate: a template literal cannot hold the backticks this probe source is made of, and SUBST exists so the file never spells a live ${.
      src: "const html = `" + SUBST + "\n<option>`;",
      expect: ["2: <option> inside a string"],
    },
    // The word boundary in the string scan: prose is not markup.
    {
      name: "a string mentioning selectable",
      fires: false,
      src: "const s = '<selectable/>';",
    },
    // A regex literal is neither a string node nor JSX, and the shell gate
    // saw it because it read text.
    {
      name: "an escape that evaluates to markup",
      fires: true,
      // The raw text has no `<` at all; the cooked text renders a native
      // dropdown. Scanning raw missed it entirely.
      src: "const html = `\\u003cselect>`;",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "an escape that spells the NAME, not just the bracket",
      fires: true,
      // The pre-filter reads raw text and the scan reads cooked, so escaping
      // the name is a way past a filter that only looks for the name. Nothing
      // here is exotic: `\e` cooks to `e` too, which is why the filter treats
      // any backslash as reason enough to parse.
      src: "const html = `<sel\\u0065ct>`;",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "the same tag twice, on different lines of one template",
      fires: true,
      // search() returned only the first, so the second line was dropped
      // silently — a report naming one of two is one a reader trusts to be
      // complete.
      src: "const html = `<select>\n<select>`;",
      expect: ["1: <select> inside a string", "2: <select> inside a string"],
    },
    {
      name: "a newline escape in a single-line string does not move the line",
      fires: true,
      // Cooked newlines that do not exist in the FILE must not be counted,
      // or the report names a line the reader cannot find.
      src: "const a = 1;\nconst html = '\\n<select>';",
      expect: ["2: <select> inside a string"],
    },
    // A hyphen CONTINUES an element name, so these are custom elements —
    // the opposite of native ones. The first boundary spelling accepted a
    // hyphen and reported all three.
    {
      name: "a custom element named select-menu",
      fires: false,
      src: "const a = <select-menu />;",
    },
    {
      name: "a custom element in a string",
      fires: false,
      src: "const h = '<option-item></option-item>';",
    },
    // Two parses see the same string node at the same offset, so a hit in a
    // non-TSX file was reported twice.
    {
      name: "a string hit in a .ts file, reported once",
      fires: true,
      src: "export const h = '<select>';",
      file: "probe.ts",
      expect: ["1: <select> inside a string"],
    },
    {
      name: "markup inside a regex literal",
      fires: true,
      src: "const re = /<select[^>]*>/;",
      expect: ["1: <select> inside a string"],
    },
    // A .ts file cannot legally hold JSX; parsed as TS it yields zero nodes,
    // which is a silent pass rather than a finding.
    {
      name: "JSX in a .ts file, which cannot legally hold it",
      fires: true,
      src: "export const a = <select />;",
      file: "probe.ts",
    },
    {
      name: "a plain .ts file with no markup",
      fires: false,
      src: "export const a = 1;",
      file: "probe.ts",
    },
    {
      name: "a native date input",
      fires: true,
      src: 'const a = <input type="date" />;',
      expect: ['1: <input type="date">'],
    },
    {
      name: "a TextInput of type date",
      fires: true,
      src: 'const a = <TextInput type="date" value={v} />;',
      expect: ['1: <TextInput type="date">'],
    },
    {
      name: "a SearchField of type date, which passes its type on",
      fires: true,
      src: 'const a = <SearchField type="date" />;',
      expect: ['1: <SearchField type="date">'],
    },
    {
      name: "a TextInput reached through a namespace",
      fires: true,
      src: 'const a = <ds.TextInput type="date" />;',
    },
    {
      name: "a renamed import of TextInput",
      fires: true,
      src: 'import { TextInput as Box } from "@margince/frontend/design-system";\nconst a = <Box type="date" />;',
      expect: ['2: <Box type="date">'],
    },
    {
      name: "a date type held in a const",
      fires: true,
      src: 'const DAY = "date";\nconst a = <input type={DAY} />;',
      expect: ['2: <input type="date">'],
    },
    {
      name: "a date type spread from a const object",
      fires: true,
      src: 'const day = { type: "date" } as const;\nconst a = <input {...day} />;',
    },
    {
      name: "a date input built with createElement",
      fires: true,
      src: 'const a = createElement("input", { type: "date" });',
      expect: ['1: <input type="date">'],
    },
    {
      name: "a TextInput built with React.createElement",
      fires: true,
      src: 'const a = React.createElement(TextInput, { type: DAY });\nconst DAY = "date";',
    },
    {
      name: "a date input built by the JSX runtime",
      fires: true,
      src: 'const a = jsxs("input", { ...{ type: "date" } });',
    },
    {
      name: "a native select built with createElement",
      fires: true,
      src: 'const a = createElement("select", null);',
      expect: ["1: <select>"],
    },
    {
      name: "a spread the gate cannot resolve, which it does not judge",
      fires: false,
      src: "const field = useField();\nconst a = <input {...field} />;",
    },
    {
      name: "a const holding another type",
      fires: false,
      src: 'const KIND = "text";\nconst a = <input type={KIND} />;',
    },
    {
      name: "an input of another type built with createElement",
      fires: false,
      src: 'const a = createElement("input", { type: "text" });',
    },
    {
      name: "DateInput of type date, which renders a text box",
      fires: false,
      src: 'const a = <DateInput type="date" />;',
    },
    {
      name: "a date type in braces",
      fires: true,
      src: 'const a = <input type={"date"} />;',
    },
    {
      name: "a date type as a template literal",
      fires: true,
      src: "const a = <input type={`date`} />;",
    },
    {
      name: "a date type from a ternary",
      fires: true,
      src: 'const a = <TextInput type={day ? "date" : "text"} />;',
    },
    {
      name: "a date type as a fallback",
      fires: true,
      src: 'const a = <input type={(kind ?? "date") as string} />;',
    },
    {
      name: "a date type in capitals, which the browser reads the same",
      fires: true,
      src: 'const a = <input type="DATE" />;',
    },
    {
      name: "a date type spread from an object literal",
      fires: true,
      src: 'const a = <input {...{ type: "date" }} />;',
    },
    {
      name: "a date type escaped in a string",
      fires: true,
      src: 'const a = <input type={"d\\u0061te"} />;',
    },
    {
      name: "a date type on the line after the tag",
      fires: true,
      src: 'const a = (\n  <input\n    type="date"\n  />\n);',
      expect: ['2: <input type="date">'],
    },
    {
      name: "a date input built in a string",
      fires: true,
      src: 'const html = \'<input name="d" type="date">\';',
      expect: ['1: <input type="date"> inside a string'],
    },
    {
      name: "a date input in JSX in a .ts file",
      fires: true,
      src: 'export const a = <input type="date" />;',
      file: "probe.ts",
      expect: ['1: <input type="date">'],
    },
    {
      name: "a text input",
      fires: false,
      src: 'const a = <input type="text" />;',
    },
    {
      name: "a type held in a name",
      fires: false,
      src: "const a = <TextInput type={kind} />;",
    },
    {
      name: "a comparison with date, which is not the value",
      fires: false,
      src: 'const a = <input type={kind === "date" ? "text" : "search"} />;',
    },
    {
      name: "a native date and time input",
      fires: true,
      src: 'const a = <input type="datetime-local" />;',
      expect: ['1: <input type="datetime-local">'],
    },
    {
      name: "a TextInput of type time",
      fires: true,
      src: 'const a = <TextInput type="time" value={v} />;',
      expect: ['1: <TextInput type="time">'],
    },
    {
      name: "a native month input",
      fires: true,
      src: 'const a = <input type="month" />;',
      expect: ['1: <input type="month">'],
    },
    {
      name: "a time type spread from a const object",
      fires: true,
      src: 'const clock = { type: "time" } as const;\nconst a = <input {...clock} />;',
      expect: ['2: <input type="time">'],
    },
    {
      name: "a type that only starts like a clock type",
      fires: false,
      src: 'const a = <input type="timeline" />;',
    },
    {
      name: "a date attribute that is not the type",
      fires: false,
      src: 'const a = <input data-type="date" />;',
    },
    {
      name: "InlineText of type date, which renders DateInput",
      fires: false,
      src: 'const a = <InlineText type="date" />;',
    },
    {
      name: "a field model naming a date type",
      fires: false,
      src: 'const field = { key: "close", type: "date" };',
    },
    {
      name: "a date and time input built in a string",
      fires: true,
      src: "const html = '<input type=\"datetime-local\">';",
      expect: ['1: <input type="datetime-local"> inside a string'],
    },
  ];

  for (const tc of cases) {
    it(`${tc.fires ? "reports" : "ignores"} ${tc.name}`, () => {
      const hits = findNativeControls(tc.file ?? "probe.tsx", tc.src);
      expect(hits.length > 0, `hits: ${JSON.stringify(hits)}`).toBe(tc.fires);
      if (tc.expect) expect(hits).toEqual(tc.expect);
    });
  }

  // Every element the rule declares is one the census can actually reach.
  //
  // Asserted through `findNativeControls` — the real entry point — rather than
  // against the pre-filter directly, so it holds whatever stops a declared
  // element being seen: the pre-filter, the parse, the boundary rule. A test of
  // one component passes while the path through it is broken.
  //
  // Keyed on the SET, so it is three assertions today and four the moment
  // somebody adds a fourth, naming the new element without anybody remembering
  // to write a case for it. That is the property worth having: the pre-filter
  // was a hand-restated copy of this list, and the failure it produced was a
  // file skipped before it was ever parsed — no finding, no error, and
  // indistinguishable from a clean tree.
  //
  // What it does NOT do, stated because this file's standard is to say so:
  // while the two lists agree it cannot detect that one is a restatement of the
  // other. It goes red on the new element the instant the set grows.
  it("can reach every element its own rule declares", () => {
    for (const tag of nativeControls) {
      expect(
        findNativeControls("probe.tsx", `const a = <${tag} />;`),
        `<${tag}> is in the rule, but the census would never parse a file that only mentions it`,
      ).not.toEqual([]);
    }
    for (const host of ["input", ...dateHosts.names]) {
      for (const spelling of ['"date"', '{"date"}', "{`date`}"]) {
        expect(
          findNativeControls(
            "probe.tsx",
            `const a = <${host} type=${spelling} />;`,
          ),
          `<${host} type=${spelling}> is in the rule, but the census would never find it`,
        ).not.toEqual([]);
      }
    }
  });

  // One host per path a `type` prop takes: whole props, a rest spread, a
  // forwarded `props.type`, a host inside a host, a private host inside one.
  it("derives its date hosts from the design system", () => {
    for (const host of [
      "TextInput",
      "SearchField",
      "ComboBox",
      "MoneyInput",
      "Checkbox",
    ]) {
      expect(dateHosts.names, `${host} passes its type on`).toContain(host);
    }
    // A literal `type` after the spread wins, so DateInput is a text box.
    expect(dateHosts.names).not.toContain("DateInput");
    expect(dateHosts.names).not.toContain("InlineText");
    expect(
      [...dateHosts.exempted],
      "a name in notHosts that the derivation no longer finds as a host",
    ).toEqual([...notHosts]);
    expect(
      [...dateHosts.opaque].sort(),
      "a component spreads a value of unknown shape onto an input: make the spread resolvable, or clear it in clearedSpreads with its reason",
    ).toEqual(clearedSpreads);
  });

  const probes = mkdtempSync(join(tmpdir(), "date-host-"));
  afterAll(() => rmSync(probes, { recursive: true, force: true }));

  it.each([
    [
      "a rest spread before its type",
      "({ type, ...rest }) => <input {...rest} type={type} />",
      true,
    ],
    [
      "a rest spread after its type",
      "({ type, ...rest }) => <input type={type} {...rest} />",
      true,
    ],
    [
      "a local object holding its type",
      "(p) => { const extra = { type: p.type }; return <input {...extra} />; }",
      true,
    ],
    [
      "a let holding its type",
      "({ type }) => { let held = type; return <input type={held} />; }",
      true,
    ],
    [
      "a const holding its type",
      "({ type }) => { const held = type; return <input type={held} />; }",
      true,
    ],
    [
      "a local object with a type of its own",
      '({ type }) => { const extra = { type: "text" }; return <input {...extra} />; }',
      false,
    ],
    [
      "a let holding a type of its own",
      '({ type }) => { let held = "text"; return <input type={held} />; }',
      false,
    ],
  ])("finds a host through %s", (_shape, component, host) => {
    const file = join(mkdtempSync(join(probes, "case-")), "probe.tsx");
    writeFileSync(file, `export const Probe = ${component};\n`);
    expect(deriveDateHosts([file], new Set()).names.has("Probe")).toBe(host);
  });

  // The line between what is derived and what is hand-written, which is the
  // whole of this fix and belongs here rather than only in the pull request:
  // DERIVE what is a restatement of the rule, HAND-WRITE what is a property of
  // the language.
  //
  // The names are the rule, so the pre-filter derives them. The backslash is
  // not: it is a property of ESCAPES. `'<sel\u0065ct>'` holds no verbatim
  // `select` in its raw text but cooks to one, and every escape begins with a
  // backslash — so a file with neither a name nor a backslash cannot spell one.
  // Deriving that arm from the set would be a second bug wearing the fix's
  // clothes.
  it("admits an escaped spelling, and does not admit an unrelated element", () => {
    expect(
      findNativeControls("probe.tsx", "const a = '<sel\\u0065ct>';"),
    ).not.toEqual([]);
    // Without this, "derived" would be satisfied by a pre-filter matching
    // everything.
    expect(prefilter.test("const a = <div />;")).toBe(false);
    expect(
      worthParsing('const a = <input type="text" onBlur={update} />;'),
    ).toBe(false);
    expect(worthParsing("const options = selected; // type a date")).toBe(
      false,
    );
    expect(worthParsing("const html = `<input type=\n date>`;")).toBe(true);
  });
});

describe("the native-controls rule", () => {
  it("reports a planted dropdown through its own corpus and visit", () => {
    const path = join(srcDir, "features", "planted.tsx");
    const text = "export const A = () => <select />;\n";
    expect(plantedFindings(rule, [{ path, area: "src", text }])).toEqual([
      "src/features/planted.tsx:1: <select>",
    ]);
  });
});
