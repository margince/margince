// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Plus } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import { Button, Field, TextInput } from "./atoms";
import {
  describeLength,
  specimenColumn,
  specimenIntro,
  specimenNote,
  specimenToken,
  useTokenValue,
} from "./tokenspecimen";

/**
 * A 4px base, spent generously. The rungs say how big an interval is, the
 * roles say what it is for, and the sizing tokens fix the measures a control,
 * a menu and an overlay stand at. Every value on this page is read from the
 * running sheet, so a retune in `tokens.css` moves it.
 */
const meta = {
  title: "Foundations/Spacing",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const rows: CSSProperties = {
  gap: "var(--space-3)",
  maxWidth: "960px",
};
const rowGrid: CSSProperties = {
  display: "grid",
  gridTemplateColumns:
    "minmax(220px, 1.2fr) minmax(160px, 3fr) minmax(150px, 1fr)",
  gap: "var(--space-4)",
  alignItems: "center",
};
const bar: CSSProperties = {
  blockSize: "var(--space-4)",
  flex: "none",
  background: "var(--accent)",
  borderRadius: "var(--r-xs)",
};
const plate: CSSProperties = {
  display: "flex",
  alignItems: "center",
  padding: "var(--space-3)",
  background: "var(--bgElevated)",
  border: "1px solid var(--borderSubtle)",
  borderRadius: "var(--r-sm)",
  overflowX: "auto",
};
const stub: CSSProperties = {
  padding: "var(--space-2)",
  background: "var(--bgInset)",
  border: "1px solid var(--borderStrong)",
  borderRadius: "var(--r-xs)",
};
const contentFill: CSSProperties = {
  padding: "var(--space-2)",
  background: "var(--accentLight)",
  borderRadius: "var(--r-xs)",
};

type Measure = Readonly<{ token: string; usage?: string }>;

function Label({ token, usage }: Measure) {
  return (
    <div className={specimenColumn}>
      <code className={specimenToken}>{token}</code>
      {usage ? <span className={specimenNote}>{usage}</span> : null}
    </div>
  );
}

function ScaleRow({ token, usage }: Measure) {
  const { ref, value, width } = useTokenValue<HTMLDivElement>(token);
  return (
    <div style={rowGrid}>
      <Label token={token} usage={usage} />
      <div style={{ overflowX: "auto" }}>
        <div ref={ref} style={{ ...bar, inlineSize: `var(${token})` }} />
      </div>
      <code>{describeLength(value, width)}</code>
    </div>
  );
}

const SCALE: readonly Measure[] = [
  { token: "--space-1" },
  {
    token: "--space-2",
    usage: "Behind --gapActions, --inputPaddingY and --stickyBottomInset.",
  },
  { token: "--space-3", usage: "Behind --controlPaddingX." },
  {
    token: "--space-4",
    usage: "Behind --gapCards and --padCard.",
  },
  { token: "--space-5", usage: "Behind --padPanel." },
  { token: "--space-6", usage: "Panes sit this far apart." },
  { token: "--space-8", usage: "The page gutter." },
  { token: "--space-10" },
  { token: "--space-12" },
  { token: "--space-16" },
];

/**
 * The rungs. The name is the value in quarter-pixels of the 4px base, so
 * `--space-8` is 32px and there is no rung the name cannot be read off. The
 * rungs above 24px give a page the section-level rhythm it needs.
 */
export const Scale: Story = {
  render: () => (
    <div className={specimenColumn} style={rows}>
      <p className={specimenIntro}>
        A 4px base, spent generously. The rungs say how big an interval is and
        nothing about what it is for, so reach for a role (next page) where one
        exists and for a rung where it does not. The number in the name is the
        value in quarter-pixels of the base, so --space-8 is 32px. Each bar is
        drawn at its token's length.
      </p>
      {SCALE.map((measure) => (
        <ScaleRow key={measure.token} {...measure} />
      ))}
    </div>
  ),
};

function RoleRow({
  token,
  usage,
  children,
}: Measure & { children: ReactNode }) {
  const { ref, value } = useTokenValue<HTMLDivElement>(token);
  return (
    <div style={rowGrid}>
      <Label token={token} usage={usage} />
      <div ref={ref} style={plate}>
        {children}
      </div>
      <code>{describeLength(value, 0)}</code>
    </div>
  );
}

/**
 * What an interval is for, answered once for the whole tree. Each role aliases
 * the rung the house already used, so naming it changed nothing on screen. A
 * screen is held to the role where one exists and to the raw rung elsewhere.
 */
export const Roles: Story = {
  render: () => (
    <div className={specimenColumn} style={rows}>
      <p className={specimenIntro}>
        A role answers what an interval is for, once for the whole tree, where a
        rung only says how big it is. Each aliases the rung the house already
        used, so naming it changed nothing on screen. Use the role in the
        context that has one; everywhere else the raw rung is still the answer.
      </p>
      <RoleRow
        token="--controlGap"
        usage="Icon to label inside a control. Off the 4px scale on purpose, and for a control alone."
      >
        <Button variant="ghost">
          <Plus aria-hidden />
          Add contact
        </Button>
      </RoleRow>
      <RoleRow
        token="--fieldGap"
        usage="A field's label to the control it names: one object, not two stacked."
      >
        <Field label="Company">
          {(control) => <TextInput {...control} defaultValue="Northwind" />}
        </Field>
      </RoleRow>
      <RoleRow
        token="--gapActions"
        usage="Between two buttons that sit side by side."
      >
        <div style={{ display: "flex", gap: "var(--gapActions)" }}>
          <Button variant="ghost">Cancel</Button>
          <Button variant="primary">Save</Button>
        </div>
      </RoleRow>
      <RoleRow token="--gapCards" usage="Between sibling card surfaces.">
        <div style={{ display: "flex", gap: "var(--gapCards)" }}>
          <div style={stub}>Card</div>
          <div style={stub}>Card</div>
        </div>
      </RoleRow>
      <RoleRow token="--padCard" usage="Inside a bare card.">
        <div style={{ ...stub, padding: "var(--padCard)" }}>
          <div style={contentFill}>Content</div>
        </div>
      </RoleRow>
      <RoleRow
        token="--padPanel"
        usage="Inside a panel, a rung above the card: the header band, rows, footer and the hairlines between them share it."
      >
        <div style={{ ...stub, padding: "var(--padPanel)" }}>
          <div style={contentFill}>Content</div>
        </div>
      </RoleRow>
      <RoleRow
        token="--segmentPaddingY"
        usage="The vertical inset of one option in a segmented track, so a pressed one sits centred in a control of --controlHeight."
      >
        <div style={{ ...stub, padding: "var(--segmentPaddingY)" }}>
          <div
            style={{
              ...contentFill,
              paddingBlock: "var(--segmentPaddingY)",
            }}
          >
            Option
          </div>
        </div>
      </RoleRow>
    </div>
  ),
};

const PARAGRAPHS: readonly Measure[] = [
  {
    token: "--paragraphSpacingLarge",
    usage: "Between two blocks of large body prose.",
  },
  {
    token: "--paragraphSpacing",
    usage: "Between two blocks of body prose. The pair the body declares.",
  },
  {
    token: "--paragraphSpacingSmall",
    usage: "Between two blocks of small body prose.",
  },
];

/**
 * The gap between two paragraphs belongs to the level of type the prose is
 * set in, so it is paired with that level rather than chosen at the call site.
 * It is rem, so it follows the reader's own text size.
 */
export const ParagraphSpacing: Story = {
  name: "Paragraph spacing",
  render: () => (
    <div className={specimenColumn} style={rows}>
      <p className={specimenIntro}>
        The gap between two paragraphs belongs to the level of type the prose is
        set in, so each level of body type is paired with its own spacing. It is
        rem, so it follows the reader's own text size. Use the one that matches
        the body level you are writing in.
      </p>
      {PARAGRAPHS.map(({ token, usage }) => (
        <RoleRow key={token} token={token} usage={usage}>
          <div>
            <p style={{ margin: 0, marginBlockEnd: `var(${token})` }}>
              The first paragraph.
            </p>
            <p style={{ margin: 0 }}>The second, one spacing below it.</p>
          </div>
        </RoleRow>
      ))}
    </div>
  ),
};

const SIZING: readonly Measure[] = [
  {
    token: "--controlHeight",
    usage:
      "A button, and the floor for any control. A floor and not a fixed value, so a wrapping label grows it.",
  },
  {
    token: "--controlPaddingX",
    usage: "The inset from a control's edge to its label.",
  },
  {
    token: "--controlIcon",
    usage:
      "The line icon a control carries. Sized by the control, because the icon library's own default is 24px.",
  },
  {
    token: "--inputHeight",
    usage:
      "A text input, and the select's trigger. A floor: the parts inside it sum to less, on purpose.",
  },
  { token: "--inputPaddingX", usage: "Inline inset inside a text input." },
  { token: "--inputPaddingY", usage: "Block inset inside a text input." },
  {
    token: "--panel-head-h",
    usage: "A panel's header band. Fixed: what does not fit goes in the body.",
  },
  {
    token: "--menuMinInlineSize",
    usage:
      "A menu's floor, where a label and its figure stop deciding the width.",
  },
  {
    token: "--menuMaxBlockSize",
    usage:
      "A menu's ceiling. Past it a reader is reading, so the surface scrolls.",
  },
  {
    token: "--settingsColumn",
    usage: "The settings measure: a label and its answer in one glance.",
  },
  { token: "--dialogConfirmWidth", usage: "A confirm dialog." },
  { token: "--dialogFormWidth", usage: "A form dialog." },
  { token: "--drawerWidth", usage: "A drawer." },
  { token: "--drawerReadingWidth", usage: "A drawer made for reading." },
  { token: "--dialogFullWidth", usage: "A full-width dialog." },
  {
    token: "--phoneAgentRise",
    usage: "How far the agent's cell rises above the phone bar's top edge.",
  },
  {
    token: "--phoneNavClearance",
    usage:
      "How far a sticky element stays clear of the foot of a phone viewport.",
  },
  {
    token: "--stickyBottomInset",
    usage: "How far a sticky element stands off the foot of the scroll area.",
  },
];

/**
 * The measures. Each bar is drawn at its token's length. A button and a text
 * input are different heights by design: a button is a verb you hit, an input
 * is a place you put something, and the room inside it says so.
 */
export const Sizing: Story = {
  render: () => (
    <div className={specimenColumn} style={rows}>
      <p className={specimenIntro}>
        These fix the measures that things stand at: a control, an input, a
        menu, a settings page and each kind of overlay. Use the token and never
        the number, because several sheets read the same measure and two
        spellings is how a page ends up disagreeing with itself.
      </p>
      <div
        style={{ display: "flex", gap: "var(--space-3)", alignItems: "end" }}
      >
        <Button variant="ghost">Cancel</Button>
        <Field label="Company" labelHidden>
          {(control) => <TextInput {...control} defaultValue="Northwind" />}
        </Field>
      </div>
      {SIZING.map((measure) => (
        <ScaleRow key={measure.token} {...measure} />
      ))}
    </div>
  ),
};
