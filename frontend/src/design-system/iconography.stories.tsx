// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  ArrowUpRight,
  Building2,
  CalendarDays,
  Check,
  CheckSquare,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronUp,
  Circle,
  CircleCheck,
  Clock,
  Copy,
  ExternalLink,
  FileText,
  Hash,
  Info,
  KeyRound,
  Lightbulb,
  Link2,
  Lock,
  Mail,
  MapPin,
  MessageSquare,
  Pencil,
  Phone,
  Plug,
  Plus,
  RefreshCw,
  Search,
  Send,
  ShieldCheck,
  Sparkles,
  Trash2,
  UserRound,
  Users,
  X,
} from "lucide-react";
import type { CSSProperties } from "react";
import { identifierNumber } from "../format/format";
import {
  specimenColumn,
  specimenIntro,
  specimenNote,
  specimenToken,
} from "./tokenspecimen";

/**
 * Icons are `lucide-react` and nothing else: `check-icon-glyph.sh` refuses an
 * emoji or a pictographic character in rendered code. A glyph is a line drawing
 * on `currentColor`, so it takes the ink of whatever it sits in and follows
 * the theme without an asset of its own.
 */
const meta = {
  title: "Foundations/Iconography",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const row: CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  alignItems: "flex-end",
  gap: "var(--space-8)",
};
const item: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: "var(--space-2)",
};
const controlSized: CSSProperties = {
  inlineSize: "var(--controlIcon)",
  blockSize: "var(--controlIcon)",
};
const column: CSSProperties = { maxWidth: "720px" };

const SIZES: ReadonlyArray<Readonly<{ size: number; note: string }>> = [
  { size: 12, note: "A sort arrow in a column head" },
  { size: 14, note: "The overflow ellipsis in a list row" },
  { size: 15, note: "The top bar's search" },
  { size: 24, note: "The library's default, which no control uses" },
];

/**
 * A control sizes its own icon. `.btn`, `.iconbtn` and `.link-button` set the
 * glyph to `--controlIcon` in CSS, because the library's own default is 24px
 * and the label beside it is not. The caller passes no `size`, so a verb's mark
 * cannot drift a pixel off the verb beside it. A call site that does pass one
 * is a dense row or an inline mark, drawn between 12 and 15.
 */
export const SizesAndStroke: Story = {
  name: "Sizes and stroke",
  render: () => (
    <div className={specimenColumn} style={column}>
      <p className={specimenIntro}>
        A control sizes its own icon, so the caller passes no size and a verb's
        mark cannot drift off the verb beside it. --controlIcon is 16px because
        the library's default is 24px and the label beside it is not. A dense
        row or inline mark may pass its own size between 12 and 15.
      </p>
      <div style={row}>
        <div style={item}>
          <Search style={controlSized} aria-hidden />
          <code className={specimenToken}>--controlIcon</code>
          <span className={specimenNote}>What a control draws</span>
        </div>
        {SIZES.map(({ size, note }) => (
          <div key={size} style={item}>
            <Search size={size} aria-hidden />
            <code className={specimenToken}>size={identifierNumber(size)}</code>
            <span className={specimenNote}>{note}</span>
          </div>
        ))}
      </div>
      <p className={specimenNote}>
        Stroke is the library's 2 unless a call site says otherwise. Where one
        does, it is 1.5 to 1.8, which suits a glyph set small beside text.
      </p>
      <div style={row}>
        {[1.5, 1.8, 2].map((stroke) => (
          <div key={stroke} style={item}>
            <Search style={controlSized} strokeWidth={stroke} aria-hidden />
            <code className={specimenToken}>
              strokeWidth={identifierNumber(stroke)}
            </code>
          </div>
        ))}
      </div>
    </div>
  ),
};

const INKS: ReadonlyArray<Readonly<{ token: string; usage: string }>> = [
  {
    token: "--textPrimary",
    usage: "Carries: a glyph that is itself the content.",
  },
  {
    token: "--textSecondary",
    usage: "Supports: the quiet glyph beside a label.",
  },
  { token: "--accentText", usage: "Brand and primary action." },
  {
    token: "--aiText",
    usage: "A machine did this. Indigo is a claim about provenance.",
  },
  { token: "--infoText", usage: "Information, and work in flight." },
  { token: "--successText", usage: "A favourable outcome." },
  { token: "--warningText", usage: "Caution before the fact." },
  { token: "--dangerText", usage: "The serious or irreversible one." },
  { token: "--discoveryText", usage: "What is new to this reader." },
];

/**
 * An icon is the ink of its container, so it is coloured by setting `color`
 * and never by a fill of its own. Use the Text token of a state: it was walked
 * until it clears 4.5:1 on every ground it lands on, where the base hue is
 * tuned to be seen at a dot's size. Colour is never the only signal, so the
 * words still say what the glyph means.
 */
export const Color: Story = {
  render: () => (
    <div className={specimenColumn} style={column}>
      <p className={specimenIntro}>
        An icon is the ink of its container, so colour it by setting the text
        colour and never with a fill of its own. Use a Text token: it was walked
        until it clears 4.5:1 where the base hue is tuned to be seen at a dot's
        size. Colour is never the only signal, so the words still say what the
        glyph means.
      </p>
      {INKS.map(({ token, usage }) => (
        <div key={token} style={{ ...row, alignItems: "center" }}>
          <Info
            style={{ ...controlSized, color: `var(${token})` }}
            aria-hidden
          />
          <code className={specimenToken}>{token}</code>
          <span className={specimenNote}>{usage}</span>
        </div>
      ))}
    </div>
  ),
};

const LIBRARY = {
  Sparkles,
  X,
  Mail,
  ChevronDown,
  Check,
  ChevronRight,
  Trash2,
  FileText,
  CheckSquare,
  ShieldCheck,
  Circle,
  Send,
  Search,
  Lock,
  ArrowRight,
  RefreshCw,
  Info,
  ChevronLeft,
  CalendarDays,
  ArrowLeft,
  Users,
  Plus,
  Phone,
  MessageSquare,
  Link2,
  ExternalLink,
  Clock,
  UserRound,
  Plug,
  Pencil,
  MapPin,
  Lightbulb,
  KeyRound,
  Hash,
  Copy,
  CircleCheck,
  ChevronUp,
  Building2,
  ArrowUpRight,
  ArrowDown,
};

const tile: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: "var(--space-1)",
  padding: "var(--space-2)",
  minInlineSize: 0,
  background: "var(--bgElevated)",
  border: "1px solid var(--borderSubtle)",
  borderRadius: "var(--r-sm)",
  color: "var(--textPrimary)",
};
const tileName: CSSProperties = {
  font: "var(--fontBodySmall)",
  maxInlineSize: "100%",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
};

const firstNamed = new Map<unknown, string>();
const ALIASES = new Map(
  Object.entries(LIBRARY).flatMap(([name, Icon]): [string, string][] => {
    const first = firstNamed.get(Icon);
    if (first === undefined) {
      firstNamed.set(Icon, name);
      return [];
    }
    return [[name, first]];
  }),
);

/**
 * The glyphs the product reaches for most, by how many files import them,
 * drawn at 24px, the library's own size. In a control the glyph is
 * `--controlIcon`, 16px, set by the control. Where two names are one drawing
 * the tile says so, because a second name for the same glyph is how two
 * screens end up importing the same icon twice.
 */
export const Library: Story = {
  render: () => (
    <div className={specimenColumn} style={column}>
      <p className={specimenIntro}>
        Icons are lucide-react and nothing else. The library is large and any of
        it is available, so search it before drawing anything new, and pick one
        glyph per verb. These are the most imported in the product, shown at
        24px, the library's default. A control draws its own at --controlIcon.
      </p>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fill, minmax(112px, 1fr))",
          gap: "var(--space-2)",
          maxWidth: "960px",
        }}
      >
        {Object.entries(LIBRARY).map(([name, Icon]) => (
          <div key={name} style={tile}>
            <Icon size={24} aria-hidden />
            <span style={tileName} title={name}>
              {name}
            </span>
            {ALIASES.has(name) ? (
              <span className={specimenNote} style={tileName}>
                alias of {ALIASES.get(name)}
              </span>
            ) : null}
          </div>
        ))}
      </div>
    </div>
  ),
};

/**
 * The rules, each with the thing that holds it.
 */
export const UsageRules: Story = {
  name: "Usage rules",
  render: () => (
    <div className={specimenColumn} style={column}>
      <p className={specimenIntro}>
        Icons stay consistent when the control owns its glyph: its size, its
        name and its colour. The rules below follow from that.
      </p>
      <ul
        className={`${specimenNote} ${specimenColumn}`}
        style={{ ...column, paddingInlineStart: "var(--space-4)" }}
      >
        <li>
          Lucide only. No emoji, no pictographic character, no image standing in
          for a glyph.
        </li>
        <li>
          Mark a decorative glyph <code>aria-hidden</code>. The accessible name
          belongs to the control that carries it.
        </li>
        <li>
          Let the control size its icon. A <code>Button</code> passes no{" "}
          <code>size</code>.
        </li>
        <li>
          An icon-only verb is an <code>IconAction</code>, and only where the
          glyph is the verb: mail, phone, calendar, pencil, link, the overflow
          ellipsis. A verb whose consequence must be read first keeps its words.
        </li>
        <li>Items in an overflow menu are words with no glyph.</li>
        <li>
          Sparkles marks an agent. Indigo glyphs mean a machine did it, and
          nothing a human wrote wears one.
        </li>
        <li>
          Colour a glyph with a text token and let the words say the meaning.
        </li>
      </ul>
    </div>
  ),
};
