// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { AI_TOOL_NAMES, type AiTool, type ConnectPath } from "./aiconnect";

// The setup guide an admin hands to their team, written from a connect path.

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
