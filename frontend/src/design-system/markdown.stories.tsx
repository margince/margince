// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Markdown, type MarkdownHighlightOutcome } from "./markdown";

const meta: Meta<typeof Markdown> = {
  title: "Design System/Markdown",
  component: Markdown,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Markdown>;

/**
 * A page of the shipped handbook, trimmed. Every form this reader claims is in
 * here, so a story that renders clean is a claim about the whole grammar
 * rather than about one paragraph.
 */
const HANDBOOK = [
  "# The pipeline: how a deal moves",
  "",
  "Deals live under **Deals** in the navigation, which draws the *pipeline*",
  "board. This page covers what a stage is and what the numbers mean.",
  "",
  "## Pipelines and stages",
  "",
  "A **stage** carries four things:",
  "",
  "- a **name** — yours to choose",
  "- a **position** in the ladder",
  "- a **semantic** — one of **Open**, **Won** or **Lost**",
  "- a **win probability** — a whole number from 0 to 100",
  "",
  "### The pipeline you start with",
  "",
  "| Stage | Semantic | Win probability |",
  "|---|---|---|",
  "| Qualified | Open | 10 |",
  "| Proposal | Open | 50 |",
  "| Won | Won | 100 |",
  "| Lost | Lost | 0 |",
  "",
  "> Only a human can edit the ladder. An agent is refused outright, because",
  "> the stage ladder is the ground truth every decision is judged against.",
  "",
  "Three ways to move a deal:",
  "",
  "1. drag it on the board",
  "2. use the stepper on the deal page",
  "3. select several and use **Move to stage**",
  "",
  "The field is `win_probability`, and the seed values come from:",
  "",
  "```sh",
  "make seed-dev   # writes the Sales pipeline",
  "```",
  "",
  "---",
  "",
  "See also [Capture](capture.md#filing) and",
  "[the product site](https://example.com/handbook).",
].join("\n");

/** The document with nothing cited — how it reads on its own. */
export const Document: Story = { args: { source: HANDBOOK } };

/** The passage a citation quotes, found and marked. */
export const QuoteFound: Story = {
  args: {
    source: HANDBOOK,
    highlight: { quote: "the stage ladder is the ground truth" },
  },
};

/**
 * The quote crossed a line break in the source, so the collapsed match still
 * finds it. This is the ordinary case rather than a clever one — the server
 * wraps a passage wherever it wraps.
 */
export const QuoteAcrossAWrap: Story = {
  args: {
    source: HANDBOOK,
    highlight: {
      quote: "Only a human can edit the ladder. An agent is refused",
    },
  },
};

/**
 * The quote is not in the document at all — a re-worded citation. The block
 * carrying the cited LINE is offered instead, which is a weaker claim and is
 * drawn as a weaker mark: a band down the side rather than a fill.
 */
export const LineFallback: Story = {
  args: {
    source: HANDBOOK,
    highlight: {
      quote: "a sentence the model wrote rather than quoted",
      line: 12,
    },
  },
};

/** Neither landed. The document is still all there, and the caller is told. */
export const NothingFound: Story = {
  render: () => {
    const [outcome, setOutcome] = useState<MarkdownHighlightOutcome>("none");
    return (
      <>
        <p className="t-caption">
          onHighlight reported: <strong>{outcome}</strong>
        </p>
        <Markdown
          source={HANDBOOK}
          highlight={{ quote: "nothing in this document says this" }}
          onHighlight={setOutcome}
        />
      </>
    );
  },
};

/**
 * A hostile document, which is the shape a customer upload can arrive in. Every
 * line of it is on the page as text: no script runs, no image loads, and the
 * two poisoned links are the words without the target.
 */
export const HostileDocument: Story = {
  args: {
    source: [
      "# An uploaded document",
      "",
      "<script>window.stolen = document.cookie;</script>",
      "",
      '<img src=x onerror="window.stolen = 1">',
      "",
      "<div style='position:fixed;inset:0'>A raw block</div>",
      "",
      "[Press for your refund](javascript:alert(1)) and",
      "[open the invoice](data:text/html;base64,PHNjcmlwdD4=).",
      "",
      "| Column | `<b>bold?</b>` |",
      "|---|---|",
      "| &lt;entity&gt; | not decoded |",
    ].join("\n"),
  },
};
