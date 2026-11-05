import { describe, expect, it } from "vitest";
import { type Locale, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  AI_CHOICES,
  AI_PLACES,
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
    expect(unavailable.map(([id]) => id).sort()).toEqual(["copilot-own"]);
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
      "visualstudio.com",
      "cursor.com",
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

  it("builds each command and file from the address the server states", () => {
    const copies = CLI_PATHS.map((path) =>
      path.steps.flatMap((step) =>
        typeof step.copy === "function" ? [step.copy(URL_FROM_SERVER)] : [],
      ),
    );
    expect(CLI_PATHS.map((path) => path.id)).toEqual([
      "claude-code",
      "codex",
      "vscode",
      "visual-studio",
      "cursor",
      "antigravity",
    ]);
    expect(copies).toEqual([
      [
        "claude mcp add --transport http --scope user margince https://crm.acme.test/mcp",
      ],
      ["codex mcp add margince --url https://crm.acme.test/mcp"],
      [],
      ['{ "servers": { "margince": { "url": "https://crm.acme.test/mcp" } } }'],
      [
        '{ "mcpServers": { "margince": { "url": "https://crm.acme.test/mcp" } } }',
      ],
      [
        '{ "mcpServers": { "margince": { "serverUrl": "https://crm.acme.test/mcp" } } }',
      ],
    ]);
  });

  it("gives every local tool something to copy, and one to three steps", () => {
    for (const path of CLI_PATHS) {
      expect(path.steps.some((step) => step.copy !== undefined)).toBe(true);
      expect(path.steps.length).toBeGreaterThanOrEqual(2);
      expect(path.steps.length).toBeLessThanOrEqual(3);
    }
  });
});

describe("which AI, and where", () => {
  const routes = Object.values(AI_PLACES).flat();

  it("reaches every app and every local tool from some pair", () => {
    const apps = routes.flatMap((place) =>
      place.route.kind === "app" ? [place.route.tool] : [],
    );
    const locals = routes.flatMap((place) =>
      place.route.kind === "local" ? [place.route.cli] : [],
    );
    expect([...new Set(apps)].sort()).toEqual([...AI_TOOLS].sort());
    expect([...new Set(locals)].sort()).toEqual(
      CLI_PATHS.map((path) => path.id).sort(),
    );
  });

  it("sends each AI only to its own vendor's app and tools", () => {
    const routesOf = (choice: (typeof AI_CHOICES)[number]) =>
      AI_PLACES[choice].map(({ route }) =>
        route.kind === "app" ? `app:${route.tool}` : `local:${route.cli}`,
      );
    expect(Object.fromEntries(AI_CHOICES.map((c) => [c, routesOf(c)]))).toEqual(
      {
        claude: [
          "app:claude",
          "local:claude-code",
          "local:claude-code",
          "local:claude-code",
          "local:claude-code",
        ],
        openai: ["app:chatgpt", "local:codex"],
        copilot: ["app:copilot", "local:vscode", "local:visual-studio"],
        gemini: ["app:gemini", "local:antigravity"],
        cursor: ["local:cursor"],
        other: [],
      },
    );
  });

  it("names every place where an AI has more than one, and none where it has one", () => {
    for (const choice of AI_CHOICES) {
      const places = AI_PLACES[choice];
      for (const place of places) {
        expect(place.label !== undefined).toBe(places.length > 1);
      }
      expect(new Set(places.map((place) => place.id)).size).toBe(places.length);
    }
  });

  it("leaves another tool without places, so it gets the generic steps", () => {
    expect(AI_PLACES.other).toEqual([]);
  });
});

describe("Gemini through work or school", () => {
  it("connects through a client a Margince admin registers with a secret", () => {
    const path = connectPath("gemini", "company");
    expect(path.unavailable).toBeUndefined();
    expect(path.helpForAdmin).toBe(true);
    expect(path.help).toBe("https://support.google.com/g/answer/17106276");
    const adminSteps = (path.admin?.steps ?? []).map(en).join("\n");
    expect(adminSteps).toContain(
      "https://vertexaisearch.cloud.google.com/oauth-redirect",
    );
    expect(adminSteps).toContain("the scopes the Margince admin gave you");
    expect(adminSteps).toContain("“Verify Auth” and “Continue”");
    expect(adminSteps).toContain("turn it on for your team");
    expect(path.steps.at(-1)).toBe("aiAccess.step.allow");
  });

  it("writes its admin guide with the steps for both sides", () => {
    const path = CONNECT_PATHS["gemini-company"];
    const guide = adminGuide({
      tool: "gemini",
      path,
      mcpUrl: URL_FROM_SERVER,
      t: translatorFor("en"),
      checkedOn: "19 Oct 2026",
    });
    const [admin, team] = guide.markdown
      .split("## What your team does next")
      .map((part) => part.match(/^\d+\. /gm) ?? []);
    expect(admin).toEqual(["1. ", "2. ", "3. ", "4. ", "5. "]);
    expect(team).toEqual(["1. ", "2. "]);
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
