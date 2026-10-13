// ESLint runs only the React Compiler's rules. Biome is the linter for
// everything else, and it already holds rules-of-hooks (useHookAtTopLevel) and
// exhaustive-deps (useExhaustiveDependencies), so those two stay off here.
import tsParser from "@typescript-eslint/parser";
import reactHooks from "eslint-plugin-react-hooks";

const classicRules = new Set([
  "react-hooks/rules-of-hooks",
  "react-hooks/exhaustive-deps",
]);

const compilerRules = Object.fromEntries(
  Object.keys(reactHooks.configs.flat.recommended.rules)
    .filter((rule) => !classicRules.has(rule))
    .map((rule) => [rule, "error"]),
);

export default [
  {
    files: ["src/**/*.{ts,tsx}"],
    ignores: [
      "src/**/*.test.{ts,tsx}",
      "src/**/*.stories.{ts,tsx}",
      "src/testing/**",
      "src/**/*.d.ts",
      "src/api/public-events.ts",
    ],
    languageOptions: {
      parser: tsParser,
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    linterOptions: { reportUnusedDisableDirectives: "error" },
    plugins: { "react-hooks": reactHooks },
    rules: compilerRules,
  },
];
