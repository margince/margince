import { describe, expect, it } from "vitest";
import { type Locale, translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  AI_PLANS,
  AI_TOOLS,
  adminGuide,
  CLI_PATHS,
  CONNECT_PATHS,
  connectPath,
} from "./aiconnect";

const URL_FROM_SERVER = "https://crm.acme.test/mcp";
const translatorFor =
  (locale: Locale) => (key: MessageKey, params?: Record<string, string>) =>
    translate(locale, key, params);
const en = (key: MessageKey) => translate("en", key);

describe("AI connect paths", () => {
  it("has one path for every tool and plan", () => {
    for (const tool of AI_TOOLS) {
      for (const plan of AI_PLANS) {
        expect(connectPath(tool, plan)).toBeDefined();
      }
    }
    expect(Object.keys(CONNECT_PATHS)).toHaveLength(
      AI_TOOLS.length * AI_PLANS.length,
    );
  });

  it("gives an unavailable path no steps, admin or address", () => {
    const unavailable = Object.entries(CONNECT_PATHS).filter(
      ([, path]) => path.unavailable !== undefined,
    );
    expect(unavailable.map(([id]) => id).sort()).toEqual([
      "copilot-own",
      "gemini-company",
    ]);
    for (const [, path] of unavailable) {
      expect(path.steps).toEqual([]);
      expect(path.admin).toBeUndefined();
      expect(path.pasteAfter).toBeUndefined();
    }
  });

  it("gives every available path one to three steps for the reader", () => {
    for (const path of Object.values(CONNECT_PATHS)) {
      if (path.unavailable === undefined) {
        expect(path.steps.length).toBeGreaterThanOrEqual(1);
        expect(path.steps.length).toBeLessThanOrEqual(3);
      }
    }
  });

  it("asks for the address only where the reader pastes it, inside the steps", () => {
    const pasting = Object.entries(CONNECT_PATHS).filter(
      ([, path]) => path.pasteAfter !== undefined,
    );
    expect(pasting.map(([id]) => id).sort()).toEqual([
      "chatgpt-own",
      "claude-own",
      "gemini-own",
    ]);
    for (const [, path] of pasting) {
      expect(path.pasteAfter).toBeGreaterThanOrEqual(1);
      expect(path.pasteAfter).toBeLessThan(path.steps.length + 1);
    }
  });

  it("links every path and command-line tool to an https page on its vendor's site", () => {
    const vendorSites = [
      "claude.com",
      "openai.com",
      "chatgpt.com",
      "microsoft.com",
      "google.com",
      "antigravity.google",
    ];
    const links = [
      ...Object.values(CONNECT_PATHS).map((path) => path.help),
      ...CLI_PATHS.map((path) => path.help),
    ];
    for (const link of links) {
      const url = new URL(link);
      expect(url.protocol).toBe("https:");
      expect(
        vendorSites.some(
          (site) => url.hostname === site || url.hostname.endsWith(`.${site}`),
        ),
      ).toBe(true);
    }
  });

  it("builds each command from the address the server states", () => {
    expect(CLI_PATHS.map((path) => path.id)).toEqual([
      "claude-code",
      "codex",
      "antigravity",
    ]);
    expect(CLI_PATHS.map((path) => path.command(URL_FROM_SERVER))).toEqual([
      "claude mcp add --transport http margince https://crm.acme.test/mcp",
      "codex mcp add margince --url https://crm.acme.test/mcp",
      '{ "mcpServers": { "margince": { "serverUrl": "https://crm.acme.test/mcp" } } }',
    ]);
  });
});

describe("admin guide", () => {
  it("writes the real address, both step lists, the official link and the date", () => {
    const path = CONNECT_PATHS["claude-company"];
    const guide = adminGuide({
      tool: "claude",
      path,
      mcpUrl: URL_FROM_SERVER,
      t: translatorFor("en"),
      checkedOn: "19 Oct 2026",
    });
    expect(guide.filename).toBe("margince-setup-claude.md");
    expect(guide.markdown.startsWith("# Set up Margince in Claude\n")).toBe(
      true,
    );
    expect(guide.markdown).toContain(`\`\`\`\n${URL_FROM_SERVER}\n\`\`\``);
    for (const key of [...(path.admin?.steps ?? []), ...path.steps]) {
      expect(guide.markdown).toContain(en(key));
    }
    expect(guide.markdown).toContain(path.help);
    expect(guide.markdown).toContain(
      "These steps were checked on 19 Oct 2026.",
    );
    expect(guide.markdown.endsWith("\n")).toBe(true);
  });

  it("numbers the admin's steps and the team's steps apart", () => {
    const guide = adminGuide({
      tool: "chatgpt",
      path: CONNECT_PATHS["chatgpt-company"],
      mcpUrl: URL_FROM_SERVER,
      t: translatorFor("en"),
      checkedOn: "19 Oct 2026",
    });
    const [admin, team] = guide.markdown
      .split("## What your team does next")
      .map((part) => part.match(/^\d+\. /gm) ?? []);
    expect(admin).toEqual(["1. ", "2. ", "3. "]);
    expect(team).toEqual(["1. ", "2. "]);
  });

  it("keeps an admin note out of the numbered steps", () => {
    const path = CONNECT_PATHS["copilot-company"];
    const guide = adminGuide({
      tool: "copilot",
      path,
      mcpUrl: URL_FROM_SERVER,
      t: translatorFor("en"),
      checkedOn: "19 Oct 2026",
    });
    const note = en(path.admin?.note ?? "aiAccess.copilot.admin5");
    expect(guide.markdown).toContain(`\n\n${note}\n\n`);
    expect(guide.markdown).not.toMatch(/^\d+\. Your company’s data policies/m);
  });

  it("writes the guide in the reader's language", () => {
    const guide = adminGuide({
      tool: "copilot",
      path: CONNECT_PATHS["copilot-company"],
      mcpUrl: URL_FROM_SERVER,
      t: translatorFor("de"),
      checkedOn: "19.10.2026",
    });
    expect(guide.filename).toBe("margince-setup-copilot.md");
    expect(guide.markdown).toContain(
      "# Margince in Microsoft Copilot einrichten",
    );
  });

  it("refuses a guide for a path that has no admin part", () => {
    expect(() =>
      adminGuide({
        tool: "claude",
        path: CONNECT_PATHS["claude-own"],
        mcpUrl: URL_FROM_SERVER,
        t: translatorFor("en"),
        checkedOn: "19 Oct 2026",
      }),
    ).toThrow(/no admin part/);
  });
});
