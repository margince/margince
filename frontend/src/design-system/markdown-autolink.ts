// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Block, Inline, Run } from "./markdown-parse";

/**
 * A bare web address in prose a person typed. The timeline's mail bodies and
 * its markdown notes both find links with this one pattern, so a URL that is a
 * link in one row is a link in the row beside it.
 */
export const URL_PATTERN = /https?:\/\/[^\s<>"')\]]+[^\s<>"')\].,;:!?]/g;

/**
 * The parsed document for prose nobody vetted: every bare address becomes a
 * link, and every link's visible text is its own destination. A labelled link
 * keeps its label as text, followed by the real address as the link, so
 * "[Your bank](https://evil.example)" cannot hide where it goes. Code stays
 * literal. The parser already refused every href but http(s) and mailto.
 */
export function autolinkBlocks(blocks: readonly Block[]): Block[] {
  return blocks.map(autolinkBlock);
}

function autolinkBlock(block: Block): Block {
  switch (block.kind) {
    case "heading":
    case "paragraph":
      return { ...block, run: autolinkRun(block.run) };
    case "list":
      return { ...block, items: block.items.map(autolinkRun) };
    case "quote":
      return { ...block, blocks: autolinkBlocks(block.blocks) };
    case "table":
      return {
        ...block,
        header: block.header.map(autolinkRun),
        rows: block.rows.map((row) => row.map(autolinkRun)),
      };
    default:
      return block;
  }
}

function autolinkRun(run: Run): Run {
  return { nodes: autolinkInline(run.nodes) };
}

function autolinkInline(nodes: readonly Inline[]): Inline[] {
  return nodes.flatMap((node): Inline[] => {
    if (node.kind === "text") return splitAddresses(node.text);
    if (node.kind === "link") return showDestination(node);
    if (node.kind === "strong" || node.kind === "em" || node.kind === "mark") {
      return [{ ...node, children: autolinkInline(node.children) }];
    }
    return [node];
  });
}

function showDestination(link: Extract<Inline, { kind: "link" }>): Inline[] {
  const shown: Inline = {
    ...link,
    children: [{ kind: "text", text: link.href }],
  };
  if (link.children.length === 1 && link.children[0].kind === "text") {
    if (link.children[0].text === link.href) return [shown];
  }
  return [
    ...autolinkInline(link.children),
    { kind: "text", text: " (" },
    shown,
    { kind: "text", text: ")" },
  ];
}

function splitAddresses(text: string): Inline[] {
  const out: Inline[] = [];
  let last = 0;
  for (const match of text.matchAll(URL_PATTERN)) {
    if (match.index > last) {
      out.push({ kind: "text", text: text.slice(last, match.index) });
    }
    const href = match[0];
    out.push({
      kind: "link",
      href,
      external: true,
      children: [{ kind: "text", text: href }],
    });
    last = match.index + href.length;
  }
  if (last < text.length) out.push({ kind: "text", text: text.slice(last) });
  return out;
}
