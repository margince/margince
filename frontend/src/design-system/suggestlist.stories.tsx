// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef, useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Field, TextInput } from "./atoms";
import { type Suggestion, SuggestPopup, useSuggestList } from "./suggestlist";

/**
 * The list `ComboBox` and `TokenInput` both open, drawn on its own so the rows
 * can be read without either host's half in the frame.
 *
 * What these stories are for: a row's label, its dimmed hint and the active
 * highlight have to read as one list whichever vocabulary fills it, and a
 * machine name reads in the body face beside someone's name — there is no
 * face to pick. The host below is the thinnest text box that can drive the
 * hook; a real surface reaches for `ComboBox` or `TokenInput`, never this.
 */
const meta = {
  title: "Design System/SuggestPopup",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

// Recipients, where the label is the half a reader remembers and the address
// is what a pick puts into the field.
const RECIPIENTS: readonly Suggestion[] = [
  {
    value: "dana@nordwand.example",
    label: "Dana Weber",
    hint: "dana@nordwand.example",
  },
  {
    value: "jonas@nordwand.example",
    label: "Jonas Keller",
    hint: "jonas@nordwand.example",
  },
  {
    value: "mira@halde.example",
    label: "Mira Brandt",
    hint: "mira@halde.example",
  },
];

// Model ids are their own label: long, namespaced, and read rather than run.
const MODELS: readonly Suggestion[] = [
  { value: "gemini-3.1-flash-lite", hint: "US$0.25 → US$1.50" },
  {
    value: "mistralai/mistral-small-3.2-24b-instruct",
    hint: "US$0.10 → US$0.30",
  },
  { value: "openai/gpt-oss-120b" },
];

function Host({
  label,
  suggestions,
  selected,
  maxWidth = "26rem",
}: Readonly<{
  label: string;
  suggestions: readonly Suggestion[];
  selected?: string;
  maxWidth?: string;
}>) {
  const [typed, setTyped] = useState("");
  const anchorRef = useRef<HTMLInputElement>(null);
  const popupRef = useRef<HTMLDivElement>(null);
  const list = useSuggestList({
    anchorRef,
    popupRef,
    suggestions,
    typed,
    onPick: setTyped,
  });
  return (
    <div style={{ maxWidth }}>
      <Field label={label}>
        {(control) => (
          <TextInput
            {...control}
            ref={anchorRef}
            type="text"
            autoComplete="off"
            {...list.fieldAria}
            value={typed}
            onChange={(event) => {
              setTyped(event.target.value);
              list.retype();
            }}
            onFocus={list.show}
            onKeyDown={list.navigate}
          />
        )}
      </Field>
      <SuggestPopup list={list} selected={selected} />
    </div>
  );
}

/** Opens the list the way a reader does, then walks onto the first row. */
async function openAndWalk(canvasElement: HTMLElement, label: string) {
  const box = await within(canvasElement).findByRole("combobox", {
    name: label,
  });
  await userEvent.click(box);
  // The popup is portalled to the body, so the canvas cannot see it.
  const page = within(canvasElement.ownerDocument.body);
  const listbox = await page.findByRole("listbox");
  const options = await within(listbox).findAllByRole("option");
  expect(options.length).toBeGreaterThan(0);
  await userEvent.keyboard("{ArrowDown}");
}

/** Label, hint and the active row, as a set-collecting host shows them. */
export const Recipients: Story = {
  render: () => <Host label="To" suggestions={RECIPIENTS} />,
  play: async ({ canvasElement }) => openAndWalk(canvasElement, "To"),
};

/**
 * A value-binding host, with the value already held marked selected. The ids
 * read in the body face at the list's own size.
 */
export const ModelIds: Story = {
  render: () => (
    <Host
      label="Model"
      suggestions={MODELS}
      selected="mistralai/mistral-small-3.2-24b-instruct"
    />
  ),
  play: async ({ canvasElement }) => openAndWalk(canvasElement, "Model"),
};

/**
 * A text box as narrow as one field of a three-field form row. The hint drops
 * under the id rather than squeezing it out: a model picker that showed only a
 * price named nothing a reader could pick.
 */
export const NarrowField: Story = {
  render: () => (
    <Host
      label="Model"
      maxWidth="13rem"
      suggestions={[
        { value: "typesafe/jev-1.13", hint: "Input US$0.04 per 1M tokens" },
      ]}
    />
  ),
  play: async ({ canvasElement }) => openAndWalk(canvasElement, "Model"),
};
