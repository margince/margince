// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// How each AI tool reaches the hosted connector, one entry per tool and plan.
// The picker and the admin guide read this module. A gate holds the skill README
// to it (backend/gates/aiconnectreadme_test.go), so keep the shapes it parses:
// `"tool-plan": {` and `help: "…",` or a `const X_HELP` it names.

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
  admin?: Readonly<{
    line: MessageKey;
    steps: readonly MessageKey[];
    note?: MessageKey;
  }>;
  steps: readonly MessageKey[];
  // The address goes after this many steps; absent where nobody pastes it.
  pasteAfter?: number;
  note?: MessageKey;
}>;

const CLAUDE_HELP =
  "https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp";
const OPENAI_HELP =
  "https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt";
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
      "aiAccess.chatgpt.ownFinish",
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
      ],
      note: "aiAccess.copilot.admin5",
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
    helpForAdmin: false,
    unavailable: "aiAccess.copilot.ownUnavailable",
    steps: [],
  },
  "gemini-company": {
    vendor: "Google",
    help: "https://support.google.com/g/answer/17106276",
    helpForAdmin: true,
    admin: {
      line: "aiAccess.gemini.adminLine",
      steps: [
        "aiAccess.gemini.admin1",
        "aiAccess.gemini.admin2",
        "aiAccess.gemini.admin3",
        "aiAccess.gemini.admin4",
        "aiAccess.gemini.admin5",
      ],
    },
    steps: ["aiAccess.gemini.connect", "aiAccess.step.allow"],
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

// Tools that run on the reader's own machine: no admin and no plan, and every
// one is set up by the reader alone.
export type CliId =
  | "claude-code"
  | "codex"
  | "vscode"
  | "visual-studio"
  | "cursor"
  | "antigravity";

// Product names, typed as the vendor spells them in every language.
export const CLI_NAMES: Readonly<Record<CliId, string>> = {
  "claude-code": "Claude Code",
  codex: "Codex",
  vscode: "VS Code",
  "visual-studio": "Visual Studio",
  cursor: "Cursor",
  antigravity: "Antigravity",
};

/** One step, and what it hands the reader to copy: the address, or a command
 *  or file built from it. */
export type CliStep = Readonly<{
  text: MessageKey;
  copy?: "address" | ((mcpUrl: string) => string);
}>;

export type CliPath = Readonly<{
  id: CliId;
  help: string;
  steps: readonly CliStep[];
}>;

const ALLOW: CliStep = { text: "aiAccess.step.allow" };

// Gemini CLI is absent: Google replaced it with Antigravity for personal plans.
export const CLI_PATHS: readonly CliPath[] = [
  {
    id: "claude-code",
    help: "https://code.claude.com/docs/en/mcp",
    steps: [
      {
        text: "aiAccess.local.claudeCode.add",
        // User scope: one setup serves every project and every editor.
        copy: (url) =>
          `claude mcp add --transport http --scope user margince ${url}`,
      },
      { text: "aiAccess.local.claudeCode.signIn" },
      ALLOW,
    ],
  },
  {
    id: "codex",
    help: "https://learn.chatgpt.com/docs/extend/mcp?surface=cli",
    steps: [
      {
        text: "aiAccess.local.codex.add",
        copy: (url) => `codex mcp add margince --url ${url}`,
      },
      { text: "aiAccess.local.codex.signIn" },
      ALLOW,
    ],
  },
  {
    id: "vscode",
    help: "https://code.visualstudio.com/docs/agent-customization/mcp-servers",
    steps: [
      { text: "aiAccess.local.vscode.add" },
      { text: "aiAccess.local.vscode.paste", copy: "address" },
      { text: "aiAccess.local.vscode.signIn" },
    ],
  },
  {
    id: "visual-studio",
    help: "https://learn.microsoft.com/en-us/visualstudio/ide/mcp-servers",
    steps: [
      {
        text: "aiAccess.local.visualStudio.file",
        copy: (url) => `{ "servers": { "margince": { "url": "${url}" } } }`,
      },
      { text: "aiAccess.local.visualStudio.signIn" },
      { text: "aiAccess.local.visualStudio.tools" },
    ],
  },
  {
    id: "cursor",
    help: "https://cursor.com/docs/mcp",
    steps: [
      {
        text: "aiAccess.local.cursor.file",
        copy: (url) => `{ "mcpServers": { "margince": { "url": "${url}" } } }`,
      },
      { text: "aiAccess.local.cursor.signIn" },
    ],
  },
  {
    id: "antigravity",
    help: "https://antigravity.google/docs/mcp",
    steps: [
      {
        text: "aiAccess.local.antigravity.file",
        // Antigravity refuses the `url`/`httpUrl` spellings its siblings accept.
        copy: (url) =>
          `{ "mcpServers": { "margince": { "serverUrl": "${url}" } } }`,
      },
      { text: "aiAccess.local.antigravity.signIn" },
      ALLOW,
    ],
  },
];

export function cliPath(id: CliId): CliPath {
  const path = CLI_PATHS.find((candidate) => candidate.id === id);
  if (path === undefined) {
    throw new Error(`no command-line path for ${id}`);
  }
  return path;
}

// The reader picks an AI, then where they use it; each pair names one path.
// The same AI reaches Margince differently in its app and in an editor, and
// the same editor differently through each AI.
export const AI_CHOICES = [
  "claude",
  "openai",
  "copilot",
  "gemini",
  "cursor",
  "other",
] as const;
export type AiChoice = (typeof AI_CHOICES)[number];

// Product names, typed as the vendor spells them; "other" is translated.
export const AI_CHOICE_NAMES: Readonly<
  Record<Exclude<AiChoice, "other">, string>
> = {
  claude: "Claude",
  openai: "ChatGPT / Codex",
  copilot: "Copilot",
  gemini: "Gemini",
  cursor: "Cursor",
};

export type AiRoute =
  | Readonly<{ kind: "app"; tool: AiTool }>
  | Readonly<{
      kind: "local";
      cli: CliId;
      /** Where to type the command, for an editor that hosts a terminal. */
      runIn?: MessageKey;
    }>;

export type AiPlace = Readonly<{
  id: string;
  /** Absent where the AI has one place, and the second question is not asked. */
  label?: MessageKey;
  route: AiRoute;
  note?: MessageKey;
}>;

const CLAUDE_IN_COPILOT: MessageKey = "aiAccess.place.claudeInCopilot";

export const AI_PLACES: Readonly<Record<AiChoice, readonly AiPlace[]>> = {
  claude: [
    {
      id: "app",
      label: "aiAccess.place.claudeApp",
      route: { kind: "app", tool: "claude" },
      note: "aiAccess.place.claudeAppNote",
    },
    {
      id: "terminal",
      label: "aiAccess.place.claudeCodeTerminal",
      route: { kind: "local", cli: "claude-code" },
    },
    {
      id: "vscode",
      label: "aiAccess.place.claudeCodeVsCode",
      route: {
        kind: "local",
        cli: "claude-code",
        runIn: "aiAccess.runIn.vscode",
      },
      note: CLAUDE_IN_COPILOT,
    },
    {
      id: "jetbrains",
      label: "aiAccess.place.claudeCodeJetBrains",
      route: {
        kind: "local",
        cli: "claude-code",
        runIn: "aiAccess.runIn.jetbrains",
      },
    },
    {
      id: "visual-studio",
      label: "aiAccess.place.claudeCodeVisualStudio",
      route: {
        kind: "local",
        cli: "claude-code",
        runIn: "aiAccess.runIn.visualStudio",
      },
      note: CLAUDE_IN_COPILOT,
    },
  ],
  openai: [
    {
      id: "app",
      label: "aiAccess.place.chatgptApp",
      route: { kind: "app", tool: "chatgpt" },
    },
    {
      id: "codex",
      label: "aiAccess.place.codex",
      route: { kind: "local", cli: "codex" },
    },
  ],
  copilot: [
    {
      id: "m365",
      label: "aiAccess.place.m365",
      route: { kind: "app", tool: "copilot" },
    },
    {
      id: "vscode",
      label: "aiAccess.place.githubVsCode",
      route: { kind: "local", cli: "vscode" },
    },
    {
      id: "visual-studio",
      label: "aiAccess.place.githubVisualStudio",
      route: { kind: "local", cli: "visual-studio" },
    },
  ],
  gemini: [
    {
      id: "app",
      label: "aiAccess.place.geminiApp",
      route: { kind: "app", tool: "gemini" },
    },
    {
      id: "antigravity",
      label: "aiAccess.place.antigravity",
      route: { kind: "local", cli: "antigravity" },
    },
  ],
  cursor: [{ id: "cursor", route: { kind: "local", cli: "cursor" } }],
  other: [],
};

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
    ...(path.admin.note === undefined ? [] : [t(path.admin.note)]),
    `## ${t("aiAccess.guide.teamHead")}`,
    numbered(path.steps),
    `## ${t("aiAccess.guide.officialHead")}`,
    path.help,
    t("aiAccess.guide.checked", { date: checkedOn }),
  ].join("\n\n");
  return { filename: `margince-setup-${tool}.md`, markdown: `${markdown}\n` };
}
