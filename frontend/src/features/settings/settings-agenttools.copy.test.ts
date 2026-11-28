import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import type { Translator } from "../../i18n";
import { en } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { agentToolTitle } from "./settings-agenttools.copy";

it("has a catalog title for every published core tool", () => {
  const manifest = JSON.parse(
    readFileSync(
      new URL("../../../../docs/reference/mcp-info.json", import.meta.url),
      "utf8",
    ),
  );
  const tools = manifest.tools.tools;
  expect(tools.length).toBeGreaterThan(40);
  const keyOf: Translator = (key) => key;
  const english: Translator = (key) => en[key];
  for (const tool of tools) {
    expect(agentToolTitle(english, tool.name, tool.title)).toBe(
      tool.title.replaceAll("'", "’"),
    );
    expect(agentToolTitle(keyOf, tool.name, tool.title)).toBe(
      `tools.name.${tool.name}`,
    );
  }
});

it("translates a known title and preserves an extension's source title", () => {
  const t: Translator = (key) => vi[key];
  expect(agentToolTitle(t, "read_record", "source")).toBe(
    vi["tools.name.read_record"],
  );
  expect(agentToolTitle(t, "ext:demo:read", "Extension title")).toBe(
    "Extension title",
  );
});
