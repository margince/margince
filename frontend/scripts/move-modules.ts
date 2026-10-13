// Moves frontend modules and rewrites every relative import that names them, so
// a move PR is a pure move: no file changes except its import paths.
//
//   node scripts/move-modules.ts <from> <to> [<from> <to> ...]
//
// Paths are relative to frontend/. A <from> directory moves every file under it.
// After the move, the script lists each place outside an import that still names
// an old path, such as a gate or a waiver list, because only a reader can say
// whether that text should follow.
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { dirname, extname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import {
  filesMatching,
  filesUnder,
  moduleAt,
  parseSource,
} from "./lib/source-tree.ts";

const frontendDir = join(dirname(fileURLToPath(import.meta.url)), "..");

// Module specifiers are the arguments of these calls as well as of import and
// export declarations: a test mocks a module by the same path it imports.
const SPECIFIER_CALLS = new Set([
  "vi.mock",
  "vi.doMock",
  "vi.unmock",
  "vi.importActual",
  "vi.importMock",
]);

/** Absolute old path to absolute new path, one entry per moved file. */
export type MoveMap = ReadonlyMap<string, string>;

// planMoves expands each pair into file moves. A directory pair keeps the
// layout under it, and a file pair moves that one file.
export function planMoves(
  pairs: readonly (readonly [string, string])[],
  filesIn: (dir: string) => readonly string[],
  isDirectory: (path: string) => boolean,
): MoveMap {
  const moves = new Map<string, string>();
  for (const [from, to] of pairs) {
    const files = isDirectory(from) ? filesIn(from) : [from];
    for (const file of files) {
      const target = isDirectory(from) ? join(to, relative(from, file)) : to;
      if (moves.has(file)) throw new Error(`${file} is moved twice`);
      moves.set(file, target);
    }
  }
  const targets = [...moves.values()];
  const doubled = targets.find((t, i) => targets.indexOf(t) !== i);
  if (doubled) throw new Error(`two files would move to ${doubled}`);
  return moves;
}

// specifierNodes lists every string literal in `source` that names a module.
export function specifierNodes(source: ts.SourceFile): ts.StringLiteralLike[] {
  const found: ts.StringLiteralLike[] = [];
  const visit = (node: ts.Node): void => {
    if (
      (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
      node.moduleSpecifier &&
      ts.isStringLiteral(node.moduleSpecifier)
    ) {
      found.push(node.moduleSpecifier);
    }
    if (ts.isCallExpression(node)) {
      const [first] = node.arguments;
      const callee =
        node.expression.kind === ts.SyntaxKind.ImportKeyword
          ? "import"
          : node.expression.getText(source);
      if (
        first &&
        ts.isStringLiteralLike(first) &&
        (callee === "import" || SPECIFIER_CALLS.has(callee))
      ) {
        found.push(first);
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

// targetOf is the file a relative specifier names: a module the bundler would
// load, or a file named exactly, such as a stylesheet.
function targetOf(
  fromFile: string,
  specifier: string,
  exists: (path: string) => boolean,
): string | null {
  const base = resolve(dirname(fromFile), specifier);
  return moduleAt(base, exists) ?? (exists(base) ? base : null);
}

// specifierFor spells `target` from `fromFile` the way `original` spelled it:
// with or without the extension, and through `index` only if it said so.
function specifierFor(
  fromFile: string,
  target: string,
  original: string,
  oldTarget: string,
): string {
  const throughIndex =
    /\/index\.[a-z]+$/.test(oldTarget) && !/\/index(\.[a-z]+)?$/.test(original);
  const named = throughIndex ? dirname(target) : target;
  const keepsExtension =
    extname(original) !== "" && extname(original) === extname(oldTarget);
  const path =
    keepsExtension || throughIndex
      ? named
      : named.slice(0, -extname(named).length);
  const spelled = relative(dirname(fromFile), path).split(sep).join("/");
  return spelled.startsWith(".") ? spelled : `./${spelled}`;
}

// rewriteImports returns `text` with every relative specifier that names a
// moved file, or that sits in a moved file, pointed at the new place.
export function rewriteImports(
  file: string,
  text: string,
  moves: MoveMap,
  exists: (path: string) => boolean,
): string {
  const source = parseSource(file, text);
  const newFile = moves.get(file) ?? file;
  const edits: { start: number; end: number; value: string }[] = [];
  for (const node of specifierNodes(source)) {
    if (!node.text.startsWith(".")) continue;
    const oldTarget = targetOf(file, node.text, exists);
    if (oldTarget === null) continue;
    const newTarget = moves.get(oldTarget) ?? oldTarget;
    if (newTarget === oldTarget && newFile === file) continue;
    const value = specifierFor(newFile, newTarget, node.text, oldTarget);
    if (value !== node.text) {
      edits.push({
        start: node.getStart(source) + 1,
        end: node.end - 1,
        value,
      });
    }
  }
  return edits
    .sort((a, b) => b.start - a.start)
    .reduce(
      (out, e) => out.slice(0, e.start) + e.value + out.slice(e.end),
      text,
    );
}

function corpus(): string[] {
  const roots = ["src", "e2e", ".storybook", "scripts"].map((dir) =>
    join(frontendDir, dir),
  );
  return roots.filter(existsSync).flatMap((root) => filesUnder(root));
}

function main(argv: readonly string[]) {
  if (argv.length === 0 || argv.length % 2 !== 0) {
    throw new Error("usage: node scripts/move-modules.ts <from> <to> [...]");
  }
  const pairs = Array.from({ length: argv.length / 2 }, (_, i) => {
    const from = resolve(frontendDir, argv[2 * i] ?? "");
    const to = resolve(frontendDir, argv[2 * i + 1] ?? "");
    if (!existsSync(from)) throw new Error(`${from} does not exist`);
    return [from, to] as const;
  });
  const isDirectory = (path: string) => statSync(path).isDirectory();
  const moves = planMoves(pairs, (dir) => filesMatching(dir, /./), isDirectory);
  const isFile = (path: string) => existsSync(path) && statSync(path).isFile();

  const rewritten: string[] = [];
  for (const file of corpus()) {
    const text = readFileSync(file, "utf8");
    const next = rewriteImports(file, text, moves, isFile);
    if (next !== text) {
      writeFileSync(file, next);
      rewritten.push(moves.get(file) ?? file);
    }
  }
  for (const [from, to] of moves) {
    execFileSync("mkdir", ["-p", dirname(to)]);
    execFileSync("git", ["mv", from, to], { cwd: frontendDir });
  }
  // A new path can sort differently, so the edited files are re-sorted the way
  // fe-lint wants them.
  const sortable = rewritten.filter((file) =>
    file.startsWith(join(frontendDir, "src")),
  );
  if (sortable.length > 0) {
    execFileSync("pnpm", ["exec", "biome", "check", "--write", ...sortable], {
      cwd: frontendDir,
      stdio: "inherit",
    });
  }
  process.stdout.write(
    `moved ${moves.size} file(s), rewrote imports in ${rewritten.length} file(s)\n`,
  );

  const repoDir = join(frontendDir, "..");
  for (const from of moves.keys()) {
    const named = relative(join(frontendDir, "src"), from).replace(
      /\.[a-z]+$/,
      "",
    );
    const hits = gitGrep(repoDir, named);
    for (const hit of hits)
      process.stdout.write(`still names ${named}: ${hit}\n`);
  }
}

function gitGrep(repoDir: string, needle: string): string[] {
  try {
    return execFileSync("git", ["grep", "-n", "-w", "-F", needle], {
      cwd: repoDir,
      encoding: "utf8",
    })
      .split("\n")
      .filter(Boolean);
  } catch (error) {
    // git grep exits 1 when nothing matches, which is the answer hoped for.
    if (error instanceof Error && "status" in error && error.status === 1)
      return [];
    throw error;
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2));
}
