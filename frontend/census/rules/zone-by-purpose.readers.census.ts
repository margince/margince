// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Every time a signed-in screen shows is read on the reader's own clock,
// `useViewerZone()`. Each file that uses another source is listed here with its
// reason. A new screen that reaches for one without a reason fails this census.

import { join, relative, sep } from "node:path";
import ts from "typescript";
import { stable } from "../../src/format/collate";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

const REPORTING =
  "Report periods and runs are counted on the company's clock, for every reader alike.";
const SERVER_WEEK =
  "The server buckets this week or day on the company's clock (decision B1 on #7894); naming it in another zone would label the bucket with days it does not hold.";
const SERVER_TODAY =
  "The server reads 'today' on the company's clock, so the form opens on the day the server will count.";
export const companyZoneSites: { file: string; why: string }[] = [
  ...[
    "analytics.decisions.tsx",
    "analytics.forecast.checktable.tsx",
    "analytics.forecast.tsx",
    "analytics.overview.donext.tsx",
    "analytics.overview.initiative.tsx",
    "analytics.overview.interventions.tsx",
    "analytics.overview.prospecting.tsx",
    "analytics.overview.sales.tsx",
    "analytics.overview.team.tsx",
    "analytics.questions.saved.tsx",
    "analytics.questions.answer.tsx",
    "inputcheck.answer.tsx",
    "inputcheck.facts.tsx",
    "reporting.executions.tsx",
    "reporting.filters.tsx",
    "reporting.initiatives.tsx",
    "reporting.library.tsx",
    "reporting.overview.tsx",
    "reporting.questionpicker.tsx",
    "reporting.targets.tsx",
    "reporting.versions.tsx",
  ].map((name) => ({ file: `features/analytics/${name}`, why: REPORTING })),
  { file: "features/companies/companies.hierarchy.tsx", why: REPORTING },
  {
    file: "design-system/boardtotals.tsx",
    why: "The rates day behind a converted total is the company's, counted on its clock like every report figure.",
  },
  { file: "lib/deals.queries.ts", why: SERVER_TODAY },
  { file: "features/settings/rates.tsx", why: SERVER_TODAY },
  { file: "features/settings/taxrates.tsx", why: SERVER_TODAY },
  {
    file: "features/brief/brief.glance.tsx",
    why: "Home holds the server's day and the weekly's week, both bucketed on the company's clock (decision B1), so the day and week it is labelled with are on that clock; the greeting hour is the reader's.",
  },
  { file: "features/brief/brief.plan.tsx", why: SERVER_WEEK },
  { file: "features/brief/brief.weekly.deals.tsx", why: SERVER_WEEK },
  { file: "features/brief/brief.weekly.figures.tsx", why: SERVER_WEEK },
  { file: "features/brief/brief.weekly.activity.tsx", why: SERVER_WEEK },
  { file: "features/brief/brief.weekly.rail.tsx", why: SERVER_WEEK },
  { file: "features/worklist/brief.teamplan.tsx", why: SERVER_WEEK },
  {
    file: "features/worklist/worklist.row.tsx",
    why: "A row's due day is the server's bucket, counted on the company's clock (decision B1), so the day comparison and the due-day text read that clock; every other time on the row is the reader's.",
  },
  {
    file: "lib/worklisthappened.ts",
    why: "A task's due day is the server's bucket, counted on the company's clock (decision B1), so the due-day sentence reads that clock; every other time in the details is the reader's.",
  },
  {
    file: "features/brief/brief.taskcard.tsx",
    why: "A task card's due day is the server's bucket, counted on the company's clock (decision B1), so the due-day text reads that clock; every other time on the row is the reader's.",
  },
  {
    file: "features/brief/brief.intro.tsx",
    why: "Whether today's read has been greeted, and the day the curtain names, follow the server's day on the company's clock (decision B1); the greeting hour is the reader's.",
  },
  {
    file: "mail-history/window-picker.tsx",
    why: "The server supplies the calendar day the import window starts on, counted on the company's clock.",
  },
];

// The browser's zone is for screens with no signed-in reader, the zone module
// and the reader's own fallback. It also serves a calendar day or a browser
// input, which are not instants.
const PUBLIC =
  "A public screen has no signed-in reader, so the browser's clock is the only one it knows.";
const DATE_ONLY =
  "A date-only value is read at its midday, so it names the same calendar day in every zone; no reader's clock changes it.";
export const browserZoneSites: { file: string; why: string }[] = [
  { file: "features/deals/buyerroomframe.tsx", why: PUBLIC },
  {
    file: "features/booking/booking-guest.tsx",
    why: "A guest on the public booking page has only the browser's clock; a host previewing their own page starts from their own zone instead.",
  },
  {
    file: "features/booking/booking-reschedule.tsx",
    why: "A guest on the public token link has only the browser's clock; a signed-in host starts from their own zone instead.",
  },
  { file: "features/privacy/confirmprivacy.tsx", why: PUBLIC },
  { file: "features/privacy/confirmdetailsaction.tsx", why: PUBLIC },
  {
    file: "features/sign-in/magic.tsx",
    why: `${PUBLIC} It is the magic-link sign-in.`,
  },
  {
    file: "features/sign-in/magic.records.tsx",
    why: `${PUBLIC} It is the magic-link sign-in.`,
  },
  {
    file: "features/sign-in/setupclaim.tsx",
    why: "The first admin sets the installation up before any user or zone is stored; the browser's zone is the only one there is.",
  },
  {
    file: "lib/viewerzone.tsx",
    why: "The reader's zone falls back to the browser's until the user has one, and the first sign-in stores the browser's as a guess.",
  },
  { file: "components/customfields.form.ts", why: DATE_ONLY },
  { file: "components/recordfieldvalues.ts", why: DATE_ONLY },
  {
    file: "features/mail/composeschedule.tsx",
    why: "The browser parses a datetime-local input in its own zone, so the zone sent with the instant has to be that one, or the send is moved by the gap.",
  },
];

const ZONE_SOURCES_SKIP =
  /\.test\.|\.stories\.|testkit|storykit|fixture|\/testing\//;

type ZoneSource = "useCompanyZone" | "browserZone";
type ZoneRef = { source: ZoneSource; file: string; line: number };

/**
 * Every spelling an identifier can have when the checker resolves it to a
 * source. A bare name finds its declaration in scope. So it is the source's own
 * name, or the local name of an import or export bound to it. A property name
 * (`zones.bz`, `{ bz: local } = zones`) is a name some module exports it under.
 * Both are collected from every file of the program, not only the swept roots.
 */
function namesDenoting(
  program: ts.Program,
  checker: ts.TypeChecker,
  targets: ReadonlyMap<ts.Symbol, ZoneSource>,
): Set<string> {
  const names = new Set([...targets.keys()].map((target) => target.name));
  const denotes = (symbol: ts.Symbol | undefined): boolean => {
    const resolved =
      symbol && symbol.flags & ts.SymbolFlags.Alias
        ? checker.getAliasedSymbol(symbol)
        : symbol;
    return resolved !== undefined && targets.has(resolved);
  };
  for (const source of program.getSourceFiles()) {
    if (source.isDeclarationFile) continue;
    const module = checker.getSymbolAtLocation(source);
    for (const each of module ? checker.getExportsOfModule(module) : []) {
      if (denotes(each)) names.add(each.name);
    }
    const visit = (node: ts.Node): void => {
      if (
        (ts.isImportSpecifier(node) ||
          ts.isExportSpecifier(node) ||
          ts.isImportClause(node) ||
          ts.isImportEqualsDeclaration(node)) &&
        node.name &&
        ts.isIdentifier(node.name) &&
        denotes(checker.getSymbolAtLocation(node.name))
      ) {
        names.add(node.name.text);
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  return names;
}

/**
 * Every reference to the two exported zone sources in the given files. The
 * checker resolves them, so a name that only looks the same is not counted. A
 * reference can be called, passed uncalled, renamed on import or read off a
 * namespace import.
 */
export function zoneSourceRefs(
  program: ts.Program,
  roots: readonly string[],
): ZoneRef[] {
  const checker = program.getTypeChecker();
  const exported = (file: string, name: string): ts.Symbol | undefined => {
    const source = program.getSourceFile(file);
    const module = source && checker.getSymbolAtLocation(source);
    return module
      ? checker.getExportsOfModule(module).find((each) => each.name === name)
      : undefined;
  };
  const targets = new Map<ts.Symbol, ZoneSource>();
  const company = exported(
    join(srcDir, "lib", "companyzone.tsx"),
    "useCompanyZone",
  );
  const browser = exported(
    join(srcDir, "format", "timezone.ts"),
    "browserZone",
  );
  if (company) targets.set(company, "useCompanyZone");
  if (browser) targets.set(browser, "browserZone");
  // Resolving every identifier makes the checker type the whole tree. Only a
  // name that can denote a source is resolved; see namesDenoting.
  const names = namesDenoting(program, checker, targets);
  const refs: ZoneRef[] = [];
  for (const root of roots) {
    const source = program.getSourceFile(root);
    if (!source) continue;
    const file = relative(srcDir, root).split(sep).join("/");
    const record = (found: ts.Symbol | undefined, at: ts.Node): void => {
      const symbol =
        found && found.flags & ts.SymbolFlags.Alias
          ? checker.getAliasedSymbol(found)
          : found;
      const kind = symbol && targets.get(symbol);
      if (kind) {
        refs.push({
          source: kind,
          file,
          line:
            source.getLineAndCharacterOfPosition(at.getStart(source)).line + 1,
        });
      }
    };
    const visit = (node: ts.Node): void => {
      // An import or export names the source without using it.
      if (
        ts.isImportDeclaration(node) ||
        ts.isExportDeclaration(node) ||
        ts.isImportEqualsDeclaration(node) ||
        ts.isTypeQueryNode(node) ||
        ts.isTypeReferenceNode(node)
      ) {
        return;
      }
      // `zones["browserZone"]()` and `zones[key]()` name the export by a string
      // type, whether written in place or held in a constant.
      if (ts.isElementAccessExpression(node)) {
        const key = checker.getTypeAtLocation(node.argumentExpression);
        const keys = key.isUnion() ? key.types : [key];
        for (const each of keys) {
          if (each.isStringLiteral()) {
            const read = checker.getPropertyOfType(
              checker.getTypeAtLocation(node.expression),
              each.value,
            );
            record(read, node);
          }
        }
      }
      if (ts.isIdentifier(node) && names.has(node.text)) {
        const declares =
          (ts.isFunctionDeclaration(node.parent) ||
            ts.isVariableDeclaration(node.parent)) &&
          node.parent.name === node;
        // A shorthand property `{ browserZone }` names the property first, and
        // the value it reads is a second symbol the checker keeps apart.
        const symbol = declares
          ? undefined
          : ts.isShorthandPropertyAssignment(node.parent)
            ? checker.getShorthandAssignmentValueSymbol(node.parent)
            : checker.getSymbolAtLocation(node);
        record(symbol, node);
      }
      ts.forEachChild(node, visit);
    };
    visit(source);
  }
  return refs;
}

const ZONE_MODULE = "src/format/timezone.ts";

const judged = (file: CensusFile) =>
  file.area === "src" &&
  /\.tsx?$/.test(file.path) &&
  file.rel !== ZONE_MODULE &&
  !ZONE_SOURCES_SKIP.test(file.rel.replace(/^src\//, ""));

// One resolution per program, shared by the floors and the findings.
const refsByProgram = new WeakMap<ts.Program, ZoneRef[]>();
function refsOver(program: ts.Program, corpus: readonly CensusFile[]) {
  let refs = refsByProgram.get(program);
  if (!refs) {
    refs = zoneSourceRefs(
      program,
      corpus.map((file) => file.path),
    );
    refsByProgram.set(program, refs);
  }
  return refs;
}

const listed = {
  useCompanyZone: new Set(companyZoneSites.map(({ file }) => file)),
  browserZone: new Set(browserZoneSites.map(({ file }) => file)),
};

export const rule: CensusRule = {
  name: "zone-by-purpose.readers",
  corpus: (tree) => tree.files.filter(judged),
  // A census that reads nothing passes every check.
  floors: (corpus, tree) => {
    const refs = refsOver(tree.checked(), corpus);
    return (["useCompanyZone", "browserZone"] as const)
      .filter((source) => !refs.some((ref) => ref.source === source))
      .map((source) => `no read of ${source} was found`);
  },
  across: (corpus, tree) => {
    const refs = refsOver(tree.checked(), corpus);
    const reads = (source: ZoneSource, file: string) =>
      refs.some((ref) => ref.source === source && ref.file === file);
    return [
      ...refs
        .filter(({ source, file }) => !listed[source].has(file))
        .map(({ source, file, line }) => `${file}:${line}: ${source}`)
        .sort(stable),
      ...companyZoneSites
        .filter(({ file }) => !reads("useCompanyZone", file))
        .map(({ file }) => `${file}: listed, but reads no useCompanyZone`),
      ...browserZoneSites
        .filter(({ file }) => !reads("browserZone", file))
        .map(({ file }) => `${file}: listed, but reads no browserZone`),
      ...[...companyZoneSites, ...browserZoneSites]
        .filter(({ why }) => why.trim().length < 40)
        .map(({ file }) => `${file}: listed without a reason`),
    ];
  },
  advice:
    "A time on a signed-in screen takes useViewerZone(). Reaching for the " +
    "company's or the browser's clock is a decision: list the file in " +
    "zone-by-purpose.readers.census.ts with the reason, or read the reader's zone.",
};
