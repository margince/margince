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
 * The parsed document with every bare address in its prose turned into a link
 * labelled with the address itself. Code stays literal and an existing link is
 * left alone, so no link ever nests inside another.
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
    if (node.kind === "strong" || node.kind === "em" || node.kind === "mark") {
      return [{ ...node, children: autolinkInline(node.children) }];
    }
    return [node];
  });
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
