// ESLint runs the React Compiler's rules and the one sender rule below, which
// Biome cannot state. Biome is the linter for everything else, and it already
// holds rules-of-hooks (useHookAtTopLevel) and exhaustive-deps
// (useExhaustiveDependencies), so those two stay off here.
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

const senderRule =
  "Send to the host through sendToHost in bridge.ts, which checks each message's shape.";

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
  // A view reaches its host only through bridge.ts, which checks the shape of
  // every message it sends. story-hosts.tsx plays the host in Storybook.
  {
    files: ["src/mcp-apps/**/*.{ts,tsx}"],
    ignores: [
      "src/mcp-apps/bridge.ts",
      "src/mcp-apps/story-hosts.tsx",
      "src/mcp-apps/**/*.test.{ts,tsx}",
      "src/mcp-apps/**/*.stories.{ts,tsx}",
    ],
    languageOptions: { parser: tsParser },
    rules: {
      "no-restricted-properties": [
        "error",
        { property: "postMessage", message: senderRule },
      ],
      "no-restricted-globals": [
        "error",
        { name: "postMessage", message: senderRule },
      ],
    },
  },
];
