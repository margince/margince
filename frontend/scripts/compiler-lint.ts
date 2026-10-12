// The React Compiler's rules, held by a ratchet. scripts/compiler-lint-waivers.txt
// freezes each file's count per rule as the tree stood when the gate landed. A
// count may only fall, and the ledger must follow it down, so a fixed violation
// cannot come back.
import { readFileSync } from "node:fs";
import { dirname, join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const frontendDir = join(dirname(fileURLToPath(import.meta.url)), "..");
export const LEDGER = join(frontendDir, "scripts", "compiler-lint-waivers.txt");

type Counts = Map<string, number>;

const key = (path: string, rule: string) => `${path} ${rule}`;

// parseLedger reads "<count> <rule> <path>" lines into a map keyed by path and rule.
export function parseLedger(text: string): Counts {
  const ledger: Counts = new Map();
  for (const [index, raw] of text.split("\n").entries()) {
    const line = raw.trim();
    if (line === "" || line.startsWith("#")) continue;
    const [count, rule, path, extra] = line.split(/\s+/);
    if (!/^[1-9]\d*$/.test(count ?? "") || !rule || !path || extra) {
      throw new Error(
        `${LEDGER}:${index + 1}: expected "<count> <rule> <path>", got "${line}"`,
      );
    }
    ledger.set(key(path, rule), Number(count));
  }
  return ledger;
}

// countFindings turns ESLint results into counts keyed by path and rule.
export function countFindings(
  results: readonly {
    filePath: string;
    messages: readonly { ruleId: string | null }[];
  }[],
  rootDir = frontendDir,
): Counts {
  const counts: Counts = new Map();
  for (const result of results) {
    const path = relative(rootDir, result.filePath).split(sep).join("/");
    for (const message of result.messages) {
      const id = key(path, message.ruleId ?? "parse-error");
      counts.set(id, (counts.get(id) ?? 0) + 1);
    }
  }
  return counts;
}

// judge lists every way the counts and the ledger disagree.
export function judge(counts: Counts, ledger: Counts): string[] {
  const failures: string[] = [];
  for (const [id, count] of counts) {
    const frozen = ledger.get(id) ?? 0;
    if (count > frozen) {
      failures.push(
        `${id}: ${count} finding(s), ledger allows ${frozen}. Fix the new one; do not raise the ledger.`,
      );
    } else if (count < frozen) {
      failures.push(
        `${id}: down to ${count} from ${frozen}. Lower its line in ${relative(frontendDir, LEDGER)} to ${count}.`,
      );
    }
  }
  for (const [id, frozen] of ledger) {
    if (!counts.has(id)) {
      failures.push(
        `${id}: fixed (ledger had ${frozen}). Delete its line from ${relative(frontendDir, LEDGER)}.`,
      );
    }
  }
  return failures.sort();
}

async function main() {
  const { ESLint } = await import("eslint");
  // The compiler's analysis is slow, so a rerun reads only files whose content changed.
  const eslint = new ESLint({
    cwd: frontendDir,
    cache: true,
    cacheLocation: join(frontendDir, "node_modules", ".cache", "compiler-lint"),
    cacheStrategy: "content",
  });
  const results = await eslint.lintFiles(["src"]);
  const counts = countFindings(results);
  const failures = judge(counts, parseLedger(readFileSync(LEDGER, "utf8")));
  if (failures.length > 0) {
    for (const failure of failures) console.error(`FAIL: ${failure}`);
    process.exit(1);
  }
  const total = [...counts.values()].reduce((sum, n) => sum + n, 0);
  process.stdout.write(
    `compiler-lint: ${total} finding(s), all in the ledger\n`,
  );
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  await main();
}
