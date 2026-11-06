// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Block, Inline, Run } from "./markdown-parse";

/**
 * The parsed document with every line break inside prose kept as a break. A
 * note's body is written line by line, often by a form ("Name: …" then
 * "Email: …"), and Markdown's soft break would join those lines into one. Code
 * keeps its own lines and is left alone.
 */
export function hardBreakBlocks(blocks: readonly Block[]): Block[] {
  return blocks.map(hardBreakBlock);
}

function hardBreakBlock(block: Block): Block {
  switch (block.kind) {
    case "heading":
    case "paragraph":
      return { ...block, run: hardBreakRun(block.run) };
    case "list":
      return { ...block, items: block.items.map(hardBreakRun) };
    case "quote":
      return { ...block, blocks: hardBreakBlocks(block.blocks) };
    default:
      return block;
  }
}

function hardBreakRun(run: Run): Run {
  return { nodes: hardBreakInline(run.nodes) };
}

function hardBreakInline(nodes: readonly Inline[]): Inline[] {
  return nodes.flatMap((node): Inline[] => {
    if (node.kind === "text") return splitLines(node.text);
    if (node.kind === "strong" || node.kind === "em" || node.kind === "mark") {
      return [{ ...node, children: hardBreakInline(node.children) }];
    }
    if (node.kind === "link") {
      return [{ ...node, children: hardBreakInline(node.children) }];
    }
    return [node];
  });
}

function splitLines(text: string): Inline[] {
  const out: Inline[] = [];
  text.split("\n").forEach((line, at) => {
    if (at > 0) out.push({ kind: "break" });
    if (line !== "") out.push({ kind: "text", text: line });
  });
  return out;
}
