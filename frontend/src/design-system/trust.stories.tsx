import type { Meta, StoryObj } from "@storybook/react-vite";
import { type CSSProperties, type ReactNode, useState } from "react";
import { expect, within } from "storybook/test";
import { identifierNumber } from "../format/format";
import { LocaleProvider } from "../i18n";
import {
  AutonomyDot,
  ConfidenceMeter,
  confidenceLevel,
  EvidenceChip,
  FieldDiff,
  ProvenanceTag,
  StagingCard,
} from "./trust";

// The whole trust vocabulary in one catalog (design-language §4): where a
// value came from, how sure the system is, and whether it is real yet. These
// primitives only mean something next to each other — a confidence dot alone
// says nothing, three of them side by side say what the scale is.
//
// Every component here reads its copy through useT, so the locale is pinned
// rather than left to the reviewing machine's browser: the catalog has to say
// the same words on every screenshot.
const meta: Meta = {
  title: "Components/AI and provenance/Trust",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};
const row: CSSProperties = {
  display: "flex",
  gap: "1rem",
  alignItems: "center",
  flexWrap: "wrap",
};

// Each primitive is a bare glyph or chip, so the catalog names it — otherwise
// the two autonomy dots are two dots.
function Specimen({
  caption,
  children,
}: Readonly<{ caption: string; children: ReactNode }>) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
      {children}
      <span className="t-caption">{caption}</span>
    </div>
  );
}

// The two glyphs a reader scans before reading anything: what the system is
// allowed to do on its own, and how much it trusts what it found. Low
// confidence is shown as low, never hidden (§4.2) — there is no prop to
// suppress it, which is why all three levels belong in one row.
export const Signals: Story = {
  render: () => (
    <div style={stack}>
      <div style={row}>
        <Specimen caption="auto — executes without asking">
          <AutonomyDot tier="auto" />
        </Specimen>
        <Specimen caption="confirm — stages for approval">
          <AutonomyDot tier="confirm" />
        </Specimen>
      </div>
      {/* The bands as a wire value reads them. `confidenceLevel` is the ONE
          spelling of the 0.8 / 0.5 thresholds, and an unrecorded confidence
          gives no glyph at all rather than a low one — the frame shows both,
          because "we do not know" and "we are not sure" are different claims. */}
      <div style={row}>
        {[0.92, 0.8, 0.61, 0.5, 0.18, null].map((confidence) => {
          const level = confidenceLevel(confidence);
          return (
            <Specimen
              // The caption NAMES which wire value produced the band beside
              // it, so it has to read as the literal in the array above —
              // grouped or padded it would label the specimen with a number
              // that is not the one being demonstrated.
              caption={
                confidence === null
                  ? "unrecorded"
                  : identifierNumber(confidence)
              }
              key={String(confidence)}
            >
              {level ? <ConfidenceMeter level={level} /> : <span>—</span>}
            </Specimen>
          );
        })}
      </div>
    </div>
  ),
};

const WEB_EVIDENCE = {
  snippet: "Series B led by Atlas Ventures, closed 14 May.",
  source: "https://www.example.com/press/2026/series-b-funding-announcement",
};
const INBOX_EVIDENCE = {
  snippet: "We are moving the renewal to October.",
  source: "email 12 Jun",
};
// A source with lines to point at: a run closes into a range, a gap stays
// apart, and a single line is said in the singular.
const TRANSCRIPT_EVIDENCE = {
  snippet: "I'll send the revised quote on Monday.",
  source: "transcript",
  lines: [12, 13, 14],
};
const TRANSCRIPT_ONE_LINE = {
  snippet: "Monday it is.",
  source: "transcript",
  lines: [21],
};

// The three forms of the chip, and the source-shortening with them: the
// collapsed chip strips the scheme and the leading www and truncates in the
// middle, so two snippets from the same site still read apart.
function EvidenceDemo() {
  const [opened, setOpened] = useState<string | null>(null);
  return (
    <div style={stack}>
      <EvidenceChip evidence={WEB_EVIDENCE} />
      <EvidenceChip
        evidence={INBOX_EVIDENCE}
        onOpen={() => setOpened(INBOX_EVIDENCE.source)}
      />
      {opened && <span className="t-caption">Opened source: {opened}</span>}
      <EvidenceChip evidence={TRANSCRIPT_EVIDENCE} />
      <EvidenceChip evidence={TRANSCRIPT_ONE_LINE} />
      <div style={row}>
        <EvidenceChip evidence={WEB_EVIDENCE} collapsed />
        <EvidenceChip evidence={INBOX_EVIDENCE} collapsed />
        <EvidenceChip evidence={TRANSCRIPT_EVIDENCE} collapsed />
      </div>
    </div>
  );
}

// The tier as a settings row's whole answer: the dot and its word, which stay
// on one line however narrow the column holding them gets.
export const AutonomyWithLabel: Story = {
  tags: ["uat-phone"],
  render: () => (
    <div style={{ ...stack, inlineSize: "6rem" }}>
      <AutonomyDot tier="auto" withLabel />
      <AutonomyDot tier="confirm" withLabel />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const word = await within(canvasElement).findByText("Approval first");
    const dot = word.querySelector(".dot");
    if (!dot) throw new Error("the tier's word drew without its dot");
    const line = word.getBoundingClientRect();
    const mark = dot.getBoundingClientRect();
    await expect(mark.top).toBeGreaterThanOrEqual(line.top);
    await expect(mark.bottom).toBeLessThanOrEqual(line.bottom);
    await expect(word.getClientRects()).toHaveLength(1);
  },
};

export const Evidence: Story = {
  render: () => <EvidenceDemo />,
};

// A stored value can be a whole jsonb document, so the long side is capped and
// scrolls — and it becomes a tab stop, because a scroll container a keyboard
// reader cannot reach is content they cannot read at all.
const LONG_VALUE =
  "Renewal terms agreed on the call: 36 months, 8 percent uplift at each " +
  "anniversary, payment 30 days net, and a mutual notice period of 90 days " +
  "before the end of the term. Legal review pending.";

// A null side is an honest marker, never a blank and never a guessed value:
// created and cleared are different facts, and a diff that renders both as an
// empty cell loses the one the reader needs.
export const Diffs: Story = {
  render: () => (
    <div style={stack}>
      <FieldDiff
        oldValue="Globex Renewal"
        newValue="Globex Renewal (updated)"
      />
      <FieldDiff oldValue={null} newValue="Carol Wagner" />
      <FieldDiff oldValue="draft" newValue={null} />
      <FieldDiff oldValue={LONG_VALUE} newValue="Signed" />
    </div>
  ),
};

function StagingDemo() {
  return (
    <StagingCard>
      <div style={row}>
        <ProvenanceTag provenance={{ kind: "agent", agent: "enrich" }} />
        <ConfidenceMeter level="med" />
      </div>
      <p style={{ marginTop: "var(--space-2)" }}>
        Headquarters: <span className="staged-value">Munich, Germany</span>
      </p>
      <EvidenceChip evidence={WEB_EVIDENCE} />
    </StagingCard>
  );
}

export const Staging: Story = {
  render: () => <StagingDemo />,
};
