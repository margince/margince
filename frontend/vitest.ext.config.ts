import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";
import base from "./vite.config.ts";

// An ABSOLUTE glob for the coverage include below. The provider resolves a
// relative pattern against the project root, which is frontend/, and this lane
// deliberately reaches out of it — a `../extensions/**` pattern matched nothing
// and the report came back empty, which the path gate caught.
const unitScreens = resolve(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "extensions",
  "*",
  "frontend",
  "**",
);

// The unit-screen test lane: vitest over extensions/*/frontend/**/*.test.tsx.
//
// WHY A SECOND CONFIG rather than widening `test.include` in vite.config.ts, or
// declaring two `test.projects` there:
//
//   - Widening the default lane's include is wrong because these suites only
//     PASS composed. A unit screen reads its copy through "@composition/copy",
//     which the vanilla alias resolves to the committed empty registry — so
//     a unit's `t("extOpenchannel.…")` returns the key rather than its copy
//     and every copy assertion below fails. The same is true of the routes it
//     calls: they exist in the merged contract and nowhere else. The unit lane
//     is the COMPOSED lane, exactly as `make fe-typecheck-composed` is, and a
//     lane with a precondition should not be smuggled into the one that has
//     none.
//   - `test.projects` in vite.config.ts would work, but a project entry does
//     not inherit the root config's `plugins`/`resolve` — so expressing it that
//     way means restating react(), tailwindcss() and the whole alias array for
//     the core project too, and the 2230-test lane silently changes shape if
//     one of those restatements drifts. mergeConfig keeps ONE definition of the
//     aliases (`@composition/*` and `@margince-ext/*`) and the setup file, and
//     changes only what this lane needs: which files it collects.
//
// Every alias the base config defines is an ABSOLUTE path built from
// vite.config.ts's own URL, so moving the root below changes nothing about how
// a unit screen resolves `@composition/*`, `@margince-ext/<unit>` or the host's
// deduped React — it resolves through exactly the map the SPA build uses.
//
// Run it with `make fe-test-ext` (composes first), which `make check-fe` calls.
const composed = process.env.MARGINCE_COMPOSITION_FRONTEND;
if (!composed) {
  throw new Error(
    "vitest.ext.config.ts: MARGINCE_COMPOSITION_FRONTEND is unset — a unit screen's suite reads copy and routes that exist only in a composed tree, so this lane must point at build/composition/frontend/. Run `make fe-test-ext`, which composes first.",
  );
}

// A SPREAD rather than mergeConfig: mergeConfig concatenates arrays, so an
// override of `include` or `setupFiles` would be appended to the base's rather
// than replacing it — which is how this lane first collected the whole core
// suite. Spreading states exactly which keys differ.
export default defineConfig({
  ...base,
  // The root STAYS frontend/ — inherited from the base, and it has to be. The
  // base sets `resolve.dedupe: ["react", "react-dom", "@tanstack/react-query"]`,
  // which makes Vite resolve those three from the ROOT so a unit gets the host's
  // single copy of the hook dispatcher and the QueryClient context. Rooting this
  // lane at the repository root instead breaks exactly that: nothing is
  // installed there, and the suite dies on "Failed to resolve import
  // react/jsx-dev-runtime" before a single case runs. So the include glob
  // reaches out of the root, and the exclusions below account for it.
  test: {
    ...base.test,
    name: "extensions",
    // A unit's tests are the only files this lane collects; src/ is the default
    // lane's job. A glob rather than a list, so a unit joins this gate by
    // existing — the same presence-is-enablement every other layer has.
    include: ["../extensions/*/frontend/**/*.test.{ts,tsx}"],
    // ITS OWN REPORT DIRECTORY, and that is the whole of what this lane needed
    // to be measured. Everything else about coverage is the base's and stays
    // the base's — the v8 provider, the exclusions, and the lcov reporter's
    // `projectRoot` pointing at the repository root, which is what makes a
    // record name a path the SonarCloud scanner can open.
    //
    // Without a directory of its own it writes frontend/coverage/lcov.info,
    // which is the CORE lane's report: whichever ran last would be the whole
    // measurement, and the other tier would read as untested. Naming a second
    // path is what lets both be handed to the scanner, which takes a list.
    coverage: {
      ...base.test?.coverage,
      reportsDirectory: "coverage-ext",
      // INCLUDE, and this is the line that makes the report measure this tier
      // rather than the last one. The v8 provider reports what the run loaded
      // and then filters by the project root, which is frontend/ — so a unit's
      // screens, which live at ../extensions/, were dropped and the report came
      // out describing 61 files of core src/ that the core lane already
      // measures. That is a report which passes every check and measures the
      // wrong tier, which is worse than none.
      include: [unitScreens],
      // The screens live OUTSIDE the project root, which the provider filters
      // by. Without this the report comes back empty however the include is
      // spelled — the root filter runs first.
      allowExternal: true,
    },
    exclude: [
      ...(base.test?.exclude ?? []),
      // The base's `**/node_modules/**` does NOT cover these paths, and the
      // failure is silent rather than loud: picomatch's leading `**` will not
      // match a `..` segment, so a pattern anchored that way never fires
      // against `../extensions/…`. It matters because a unit's frontend/ is a
      // workspace member — pnpm links the host into
      // extensions/<unit>/frontend/node_modules/@margince/frontend, and through
      // that symlink the glob above finds the ENTIRE core suite and runs it a
      // second time under the composed alias. That is 193 extra files and three
      // false failures from core tests whose whole point is that the VANILLA
      // registry is empty.
      "../extensions/*/frontend/node_modules/**",
    ],
  },
});
