// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

// How each AI tool reaches the hosted connector, one entry per tool and plan.
// The picker, the admin guide and the skill README all read this module;
// backend/gates/aiconnectreadme_test.go parses its literals, so keep each entry
// on the `"tool-plan": {` / `help: "…",` lines it reads.

export const AI_TOOLS = ["claude", "chatgpt", "copilot", "gemini"] as const;
export const AI_PLANS = ["company", "own"] as const;

export type AiTool = (typeof AI_TOOLS)[number];
export type AiPlan = (typeof AI_PLANS)[number];
export type ConnectPathId = `${AiTool}-${AiPlan}`;

// Product names are typed as the vendor spells them, in every language.
export const AI_TOOL_NAMES: Readonly<Record<AiTool, string>> = {
  claude: "Claude",
  chatgpt: "ChatGPT",
  copilot: "Microsoft Copilot",
  gemini: "Gemini",
};

export type ConnectPath = Readonly<{
  vendor: "Anthropic" | "OpenAI" | "Microsoft" | "Google";
  help: string;
  // The vendor page explains the admin's half, not the reader's.
  helpForAdmin: boolean;
  unavailable?: MessageKey;
  admin?: Readonly<{ line: MessageKey; steps: readonly MessageKey[] }>;
  steps: readonly MessageKey[];
  // The address goes after this many steps; absent where nobody pastes it.
  pasteAfter?: number;
  note?: MessageKey;
}>;

const CLAUDE_HELP =
  "https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp";
const OPENAI_HELP =
  "https://help.openai.com/en/articles/12584461-developer-mode-apps-and-full-mcp-connectors-in-chatgpt-beta";
const MICROSOFT_HELP =
  "https://learn.microsoft.com/en-us/microsoft-copilot-studio/mcp-add-existing-server-to-agent";

export const CONNECT_PATHS: Readonly<Record<ConnectPathId, ConnectPath>> = {
  "claude-company": {
    vendor: "Anthropic",
    help: CLAUDE_HELP,
    helpForAdmin: false,
    admin: {
      line: "aiAccess.claude.adminLine",
      steps: [
        "aiAccess.claude.admin1",
        "aiAccess.claude.admin2",
        "aiAccess.claude.admin3",
        "aiAccess.claude.admin4",
      ],
    },
    steps: [
      "aiAccess.claude.openConnectors",
      "aiAccess.claude.connect",
      "aiAccess.step.allow",
    ],
  },
  "claude-own": {
    vendor: "Anthropic",
    help: CLAUDE_HELP,
    helpForAdmin: false,
    steps: [
      "aiAccess.claude.openConnectors",
      "aiAccess.claude.addCustom",
      "aiAccess.step.allow",
    ],
    pasteAfter: 2,
    note: "aiAccess.claude.freeNote",
  },
  "chatgpt-company": {
    vendor: "OpenAI",
    help: OPENAI_HELP,
    helpForAdmin: false,
    admin: {
      line: "aiAccess.chatgpt.adminLine",
      steps: [
        "aiAccess.chatgpt.admin1",
        "aiAccess.chatgpt.admin2",
        "aiAccess.chatgpt.admin3",
      ],
    },
    steps: ["aiAccess.chatgpt.turnOn", "aiAccess.step.allow"],
  },
  "chatgpt-own": {
    vendor: "OpenAI",
    help: OPENAI_HELP,
    helpForAdmin: false,
    steps: [
      "aiAccess.chatgpt.developerMode",
      "aiAccess.chatgpt.create",
      "aiAccess.step.allow",
    ],
    pasteAfter: 2,
    note: "aiAccess.chatgpt.ownNote",
  },
  "copilot-company": {
    vendor: "Microsoft",
    help: MICROSOFT_HELP,
    helpForAdmin: true,
    admin: {
      line: "aiAccess.copilot.adminLine",
      steps: [
        "aiAccess.copilot.admin1",
        "aiAccess.copilot.admin2",
        "aiAccess.copilot.admin3",
        "aiAccess.copilot.admin4",
        "aiAccess.copilot.admin5",
      ],
    },
    steps: [
      "aiAccess.copilot.openAgent",
      "aiAccess.copilot.ask",
      "aiAccess.copilot.allow",
    ],
  },
  "copilot-own": {
    vendor: "Microsoft",
    help: MICROSOFT_HELP,
    helpForAdmin: true,
    unavailable: "aiAccess.copilot.ownUnavailable",
    steps: [],
  },
  "gemini-company": {
    vendor: "Google",
    help: "https://support.google.com/g/answer/17106276",
    helpForAdmin: true,
    unavailable: "aiAccess.gemini.companyUnavailable",
    steps: [],
  },
  "gemini-own": {
    vendor: "Google",
    help: "https://support.google.com/gemini/answer/17209137",
    helpForAdmin: false,
    steps: [
      "aiAccess.gemini.openApps",
      "aiAccess.gemini.addApp",
      "aiAccess.step.allow",
    ],
    pasteAfter: 2,
    note: "aiAccess.gemini.ownNote",
  },
};

export function connectPath(tool: AiTool, plan: AiPlan): ConnectPath {
  return CONNECT_PATHS[`${tool}-${plan}`];
}

export type CliPath = Readonly<{
  id: "claude-code" | "codex" | "antigravity";
  name: string;
  help: string;
  command: (mcpUrl: string) => string;
  signIn?: string;
  note?: MessageKey;
}>;

// Gemini CLI is absent: Google replaced it with Antigravity for personal plans.
export const CLI_PATHS: readonly CliPath[] = [
  {
    id: "claude-code",
    name: "Claude Code",
    help: "https://code.claude.com/docs/en/mcp",
    command: (url) => `claude mcp add --transport http margince ${url}`,
    signIn: "/mcp",
  },
  {
    id: "codex",
    name: "Codex",
    help: "https://learn.chatgpt.com/docs/extend/mcp?surface=cli",
    command: (url) => `codex mcp add margince --url ${url}`,
    signIn: "codex mcp login margince",
  },
  {
    id: "antigravity",
    name: "Antigravity",
    help: "https://antigravity.google/docs/mcp",
    // Antigravity refuses the `url`/`httpUrl` spellings its siblings accept.
    command: (url) =>
      `{ "mcpServers": { "margince": { "serverUrl": "${url}" } } }`,
    note: "agents.connectAntigravityPath",
  },
];

// The day these steps were last compared with each vendor's own guide.
export const STEPS_CHECKED_ON = "2026-10-19";

export function adminGuide(
  input: Readonly<{
    tool: AiTool;
    path: ConnectPath;
    mcpUrl: string;
    t: Translator;
    checkedOn: string;
  }>,
): Readonly<{ filename: string; markdown: string }> {
  const { tool, path, mcpUrl, t, checkedOn } = input;
  if (path.admin === undefined) {
    throw new Error(`the ${tool} path has no admin part to write a guide for`);
  }
  const name = AI_TOOL_NAMES[tool];
  const numbered = (keys: readonly MessageKey[]) =>
    keys.map((key, index) => `${index + 1}. ${t(key)}`).join("\n");
  const markdown = [
    `# ${t("aiAccess.guide.title", { tool: name })}`,
    `## ${t("aiAccess.guide.addressHead")}`,
    `\`\`\`\n${mcpUrl}\n\`\`\``,
    `## ${t("aiAccess.guide.adminHead")}`,
    numbered(path.admin.steps),
    `## ${t("aiAccess.guide.teamHead")}`,
    numbered(path.steps),
    `## ${t("aiAccess.guide.officialHead")}`,
    path.help,
    t("aiAccess.guide.checked", { date: checkedOn }),
  ].join("\n\n");
  return { filename: `margince-setup-${tool}.md`, markdown: `${markdown}\n` };
}
