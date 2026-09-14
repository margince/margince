// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type CSSProperties, useState } from "react";
import { Field } from "./atoms";
import { Select, type SelectOption } from "./select";
import { TAG_TONES } from "./tagpill";
import "./tagpill.css";

/**
 * The select, which is a button and a portalled listbox rather than a native
 * `<select>` — the one control a browser draws for itself, in the platform's own
 * idiom, on a screen built entirely from ours.
 *
 * What these stories are for, in order of what they actually catch: the closed
 * face has to sit on the same baseline as a `TextInput` beside it (the Fields
 * story), the popup has to stay unclipped when the control lives in a toolbar
 * inside a scroller (In A Scroller), and it has to flip above the trigger near
 * the bottom of the window (Near The Bottom). Flip the Theme toolbar to see the
 * dark rendering — every value here is a token, so all of it re-resolves.
 */
const meta = {
  title: "Design System/Select",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

// The tag palette, read from the array the product itself offers rather than
// typed out here: a story that restates the list stops showing the real one the
// first time a tone is added.
const TONES: readonly SelectOption[] = [
  { value: "", label: "No colour" },
  ...TAG_TONES.map((tone) => ({
    value: tone,
    label: tone[0].toUpperCase() + tone.slice(1),
    adornment: (
      <span className={`tagpill-dot tagpill-dot-${tone}`} aria-hidden />
    ),
  })),
];

const STAGES: readonly SelectOption[] = [
  { value: "qualify", label: "Qualify" },
  { value: "proposal", label: "Proposal" },
  { value: "negotiation", label: "Negotiation" },
  { value: "won", label: "Won" },
  { value: "lost", label: "Lost" },
];

// Long enough that the popup has to scroll rather than grow past the viewport,
// and long enough to hold a real IANA list's worth of near-identical labels.
const ZONES: readonly SelectOption[] = [
  "Europe/Berlin",
  "Europe/Brussels",
  "Europe/Bucharest",
  "Europe/Budapest",
  "Europe/Copenhagen",
  "Europe/Dublin",
  "Europe/Helsinki",
  "Europe/Lisbon",
  "Europe/London",
  "Europe/Madrid",
  "Europe/Oslo",
  "Europe/Paris",
  "Europe/Prague",
  "Europe/Riga",
  "Europe/Rome",
  "Europe/Sofia",
  "Europe/Stockholm",
  "Europe/Tallinn",
  "Europe/Vienna",
  "Europe/Vilnius",
  "Europe/Warsaw",
  "Europe/Zurich",
].map((zone) => ({ value: zone, label: zone }));

// A roster: one short label and the long ones a real workspace carries. The
// point of the set is the DISTANCE between the two, which is what a list sized
// to its trigger destroys.
const CONTACTS: readonly SelectOption[] = [
  { value: "mine", label: "Mine" },
  { value: "kr", label: "Dr. Katharina Reinhardt-Vogel" },
  { value: "jb", label: "Jean-Baptiste Moreau-Lefèvre" },
  { value: "nn", label: "Nguyễn Thị Minh Khai" },
];

const column: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-4)",
  maxWidth: "22rem",
};

/** The control is controlled, so every story owns the value it shows. */
function Demo({
  options,
  start = "",
  label,
  placeholder,
  disabled,
  required,
  hint,
}: Readonly<{
  options: readonly SelectOption[];
  start?: string;
  label: string;
  placeholder?: string;
  disabled?: boolean;
  required?: boolean;
  hint?: string;
}>) {
  const [value, setValue] = useState(start);
  return (
    <Field label={label} hint={hint} required={required}>
      {(control) => (
        <Select
          {...control}
          options={options}
          value={value}
          onChange={setValue}
          placeholder={placeholder}
          disabled={disabled}
        />
      )}
    </Field>
  );
}

export const Default: Story = {
  render: () => (
    <div style={column}>
      <Demo label="Stage" options={STAGES} start="proposal" />
    </div>
  ),
};

/** Nothing chosen yet: the face is the placeholder, and it does not read as a value. */
export const WithPlaceholder: Story = {
  render: () => (
    <div style={column}>
      <Demo
        label="Stage"
        options={STAGES}
        placeholder="Pick a stage"
        required
        hint="A deal has to sit somewhere in the pipeline."
      />
    </div>
  ),
};

/**
 * A SHORT TRIGGER OVER LONG OPTIONS, which is the case the list's own width
 * exists for. "Mine" is the chosen value, so the closed face is narrow; every
 * option behind it is a full name. Sized to the trigger, the list read
 * "Dr. Kathari…" for all of them and the reader could not tell one colleague
 * from another in the one place the control is asked to.
 *
 * Open it: the face stays narrow — a trigger that grew with its value would
 * move whatever sits beside it — and the list stands out past the trigger to
 * the width its longest label needs, capped at 24rem.
 */
export const ShortTriggerLongOptions: Story = {
  render: () => (
    <div style={{ ...column, maxWidth: "9rem" }}>
      <Demo label="Viewing" options={CONTACTS} start="mine" />
    </div>
  ),
};

/**
 * The same list on a control at the TRAILING EDGE of the page, which is where
 * the Worklist's own dial sits. The list may not run off the screen to reach
 * its content width, so the cap is the room measured from its leading edge
 * rather than a fraction of the viewport.
 */
export const ContentSizedAtTheEdge: Story = {
  render: () => (
    <div style={{ display: "flex", justifyContent: "flex-end" }}>
      <div style={{ ...column, maxWidth: "9rem" }}>
        <Demo label="Viewing" options={CONTACTS} start="mine" />
      </div>
    </div>
  ),
};

/** A list past the popup's height cap, which then scrolls inside its own box. */
export const LongList: Story = {
  render: () => (
    <div style={column}>
      <Demo label="Time zone" options={ZONES} start="Europe/Berlin" />
    </div>
  ),
};

export const Disabled: Story = {
  render: () => (
    <div style={column}>
      <Demo label="Stage" options={STAGES} start="won" disabled />
      <Demo
        label="Stage"
        options={STAGES}
        placeholder="Pick a stage"
        disabled
      />
    </div>
  ),
};

/**
 * A choice this workspace cannot make right now stays LISTED and stays readable —
 * that it exists is information — but it takes no hover highlight, the keyboard
 * steps over it, and a click on it does nothing.
 */
export const DisabledOption: Story = {
  render: () => (
    <div style={column}>
      <Demo
        label="Stage"
        start="qualify"
        options={[
          { value: "qualify", label: "Qualify" },
          { value: "proposal", label: "Proposal" },
          { value: "won", label: "Won — needs an approval", disabled: true },
        ]}
      />
    </div>
  ),
};

/**
 * An option whose label is not in the page's language declares its own, through
 * `lang` — WCAG 2.2 AA 3.1.2 (Language of Parts). The language picker in
 * Settings → Account is the case: the names are proper nouns and stay
 * untranslated, so without the attribute a screen reader reads "Tiếng Việt" with
 * the phonemes of whichever locale the page is on. Nothing is visible here — the
 * whole effect is audible — so read it in the DOM: each option's label span and
 * the trigger face carry the tag. An option already in the page's language, like
 * the last one, declares nothing.
 */
export const OptionsInOtherLanguages: Story = {
  render: () => (
    <div style={column}>
      <Demo
        label="Language"
        start="vi"
        options={[
          { value: "en", label: "English", lang: "en" },
          { value: "de", label: "Deutsch", lang: "de" },
          { value: "vi", label: "Tiếng Việt", lang: "vi" },
          { value: "auto", label: "Match my browser" },
        ]}
      />
    </div>
  ),
};

/**
 * The case the whole positioning design exists for. `.scroll` in app/shell.css is
 * `overflow-y: auto; position: relative`, and most of these controls sit in a
 * toolbar inside it — an absolutely positioned popup is clipped by that box and
 * scrolls away from its own trigger. Open the select, then scroll the frame: the
 * popup stays on the trigger and closes once the trigger is gone.
 */
export const InAScroller: Story = {
  render: () => (
    <div
      style={{
        height: "180px",
        overflowY: "auto",
        border: "1px solid var(--borderSubtle)",
        borderRadius: "var(--r-sm)",
        padding: "var(--space-3)",
        position: "relative",
      }}
    >
      <div style={{ ...column, paddingBottom: "var(--space-6)" }}>
        <Demo label="Stage" options={STAGES} start="proposal" />
        <p className="t-caption">
          Scroll this frame with the list open — the popup follows its trigger
          and is never clipped by this box.
        </p>
        <div style={{ height: "320px" }} />
      </div>
    </div>
  ),
};

/** No room below, so the popup opens upwards instead of off the window. */
export const NearTheBottom: Story = {
  parameters: { layout: "fullscreen" },
  render: () => (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "flex-end",
        padding: "var(--space-4)",
      }}
    >
      <div style={column}>
        <Demo label="Time zone" options={ZONES} start="Europe/Vienna" />
      </div>
    </div>
  ),
};

/** The dark rendering, pinned as its own story rather than left to the toolbar. */
export const Dark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <div style={column}>
      <Demo label="Stage" options={STAGES} start="proposal" />
      <Demo label="Time zone" options={ZONES} placeholder="Pick a zone" />
    </div>
  ),
};

/**
 * An option can carry a mark before its label. The tag picker's colour dots are
 * the case this exists for, where the swatch IS what a reader picks by and the
 * label only names it.
 *
 * Two renders to look at, because they are two different paths: the mark in the
 * open list, and the same mark repeated on the CLOSED face, which is what tells
 * a reader which one they chose once the list is shut. "No colour" carries none,
 * so a list mixing marked and bare options has to stay legible too.
 */
export const WithAdornments: Story = {
  render: () => (
    <div style={column}>
      <Demo label="Colour" options={TONES} start="rose" />
      <Demo label="Colour" options={TONES} placeholder="Pick a colour" />
    </div>
  ),
};

/** The same list on a dark ground. The tag tones carry their own dark values,
 * so each dot has to stay tellable from its neighbours in both themes. */
export const AdornmentsDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <div style={column}>
      <Demo label="Colour" options={TONES} start="violet" />
    </div>
  ),
};
