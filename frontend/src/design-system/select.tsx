// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Check, ChevronDown } from "lucide-react";
import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import { contentSizedPopupBox, type PopupFrame } from "./anchoredpopup";
import { usePrefersReducedMotion } from "./motion";
import { type Listbox, useSelectListbox } from "./selectlistbox";
import "./select.css";

/**
 * The Margince select: a button trigger plus a portalled listbox popup.
 *
 * It exists because a native `<select>` is the one control the browser draws
 * itself. Its closed face takes our tokens, and everything the user actually
 * chooses from — the option list, its fill, its type, its highlight, its
 * scrollbar — is painted by the platform in the platform's own idiom. On the
 * same screen as the rest of this design system that reads as a hole, and no
 * amount of CSS closes it: `option` is not stylable in any engine we ship to.
 *
 * Two consequences the caller sees, both deliberate:
 *
 *  - `onChange` reports the VALUE, not an event. A listbox has no
 *    `event.target.value`, and threading a synthetic event through would be a
 *    lie about where the value came from.
 *  - `options` is data, not `<option>` children. The component has to know the
 *    labels to render a trigger face, to run typeahead and to skip a disabled
 *    entry from the keyboard; reading that back out of children would be
 *    guesswork.
 *
 * `required` becomes `aria-required` rather than a `required` attribute: a
 * button carries no constraint validation, and neither does the hidden input
 * that mirrors the value into a real `<form>` (the HTML spec exempts hidden
 * inputs from validation). So a required select announces the requirement and
 * the surrounding form still owns refusing an empty submit — which every screen
 * here already does in its own submit handler.
 */
export type SelectOption = Readonly<{
  value: string;
  label: string;
  disabled?: boolean;
  /**
   * A decorative mark drawn before the label, in the list AND on the closed
   * face — a colour swatch, a provider mark. It is `aria-hidden` by contract:
   * the label still has to say everything the option means, because a reader
   * on a screen reader gets only the label, and a swatch that carried meaning
   * of its own would be a distinction only sighted users could make.
   *
   * `label` stays a plain string precisely so this cannot erode it: typeahead
   * matches it, the trigger falls back to it, and the suite asserts on it.
   */
  adornment?: ReactNode;
  /**
   * A BCP 47 tag when this option's LABEL is written in a language other than
   * the document's — a language picker's endonyms, a locale name, a quoted
   * foreign title. WCAG 2.2 AA 3.1.2 (Language of Parts): without it a screen
   * reader reads "Tiếng Việt" with the phonemes of whichever locale the page is
   * currently in. Omit it when the label is in the page's own language; an
   * attribute that merely repeats the document's is noise, and one that is
   * wrong is worse than one that is absent.
   */
  lang?: string;
}>;

export type SelectProps = Readonly<{
  options: readonly SelectOption[];
  value: string;
  onChange: (value: string) => void;
  /** The trigger face when `value` is "" or matches no option. */
  placeholder?: string;
  id?: string;
  /** Rendered on a hidden input so a real `<form>` still carries the value. */
  name?: string;
  disabled?: boolean;
  required?: boolean;
  className?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
  // Opens the list on the trigger's first render rather than waiting for a
  // second interaction — for a caller that mounts this Select AS the click
  // that started editing (InlineChoice), where the first click already meant
  // "show me the options" and a second one is one press too many.
  openOnMount?: boolean;
  // Fires when the list closes WITHOUT a value having been committed —
  // Escape, a press outside, or the trigger scrolling out of view. Never
  // fires from `commit`, which is the one path a value actually changes on:
  // a caller telling "closed, nothing chosen" apart from "closed, picked" is
  // exactly what InlineChoice needs to revert to its resting view only on
  // the former.
  onCancel?: () => void;
  // Fires when Tab moves focus forward, out of the list, without picking
  // anything. Kept apart from `onCancel`: the reader is moving ON, not
  // backing out, so a caller that pulls focus back to its own trigger on
  // cancel must not also do that here — that would fight the very key that
  // just moved focus away.
  onLeave?: () => void;
}>;

export function Select(props: SelectProps) {
  const {
    options,
    value,
    onChange,
    name,
    disabled,
    openOnMount,
    onCancel,
    onLeave,
  } = props;
  const listbox = useSelectListbox(
    options,
    options.findIndex((option) => option.value === value),
    (option) => {
      onChange(option.value);
      return "close";
    },
    openOnMount ?? false,
    onCancel,
    onLeave,
  );
  const reduced = usePrefersReducedMotion();

  return (
    <>
      <SelectTrigger field={props} listbox={listbox} animate={!reduced} />
      {/* The value a real <form> submits. The trigger is a button, which carries
          no form value of its own, so a screen that posts a form rather than
          calling the typed client keeps working unchanged. The mirror carries
          `disabled` with the control because the browser leaves a disabled
          control out of the form's entry list: a disabled select that still
          submitted its value would make the disabled state a lie about what the
          form sends. */}
      {name !== undefined && (
        <input type="hidden" name={name} value={value} disabled={disabled} />
      )}
      {listbox.open && listbox.frame
        ? createPortal(
            <SelectPopup
              options={options}
              selected={(option) => option.value === value}
              frame={listbox.frame}
              listbox={listbox}
              animate={!reduced}
            />,
            document.body,
          )
        : null}
    </>
  );
}

/**
 * The multi-value sibling: the same trigger-plus-listbox anatomy, where a pick
 * TOGGLES membership and the list stays open, so choosing three of thirty
 * options is three clicks rather than three open-pick-reopen rounds.
 *
 * It exists for an ENUMERABLE vocabulary — a custom field's own options, where
 * a page of thirty checkboxes buries the form around it. `TokenInput` remains
 * the control for a set nobody can enumerate (cities, free tags); the line
 * between them is whether the options list IS the vocabulary.
 *
 * The closed face reads the chosen labels in option order, comma-joined, and
 * ellipsizes like any select face; `placeholder` is the empty face. There is
 * no hidden-input mirror: a set has no single form value, and every caller
 * here submits through its own handler rather than a native form post.
 */
export type MultiSelectProps = Readonly<{
  options: readonly SelectOption[];
  values: readonly string[];
  onChange: (next: string[]) => void;
  /** The trigger face while nothing is chosen. */
  placeholder?: string;
  id?: string;
  disabled?: boolean;
  required?: boolean;
  className?: string;
  "aria-label"?: string;
  "aria-labelledby"?: string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
}>;

export function MultiSelect(props: MultiSelectProps) {
  const { options, values, onChange } = props;
  const chosen = new Set(values);
  const listbox = useSelectListbox(
    options,
    options.findIndex((option) => chosen.has(option.value)),
    (option) => {
      onChange(
        chosen.has(option.value)
          ? values.filter((value) => value !== option.value)
          : [...values, option.value],
      );
      return "stay";
    },
    false,
  );
  const reduced = usePrefersReducedMotion();
  // Option order, not click order: the face is read against the list it came
  // from, and a face that reorders on every toggle jumps under the reader.
  const face = options
    .filter((option) => chosen.has(option.value))
    .map((option) => option.label)
    .join(", ");
  return (
    <>
      <TriggerButton
        field={props}
        listbox={listbox}
        animate={!reduced}
        face={face || (props.placeholder ?? " ")}
        placeholderShown={face === ""}
      />
      {listbox.open && listbox.frame
        ? createPortal(
            <SelectPopup
              options={options}
              selected={(option) => chosen.has(option.value)}
              multiselectable
              frame={listbox.frame}
              listbox={listbox}
              animate={!reduced}
            />,
            document.body,
          )
        : null}
    </>
  );
}

function SelectTrigger({
  field,
  listbox,
  animate,
}: Readonly<{ field: SelectProps; listbox: Listbox; animate: boolean }>) {
  const selected = field.options.find((option) => option.value === field.value);
  // A value that matches no option, with no placeholder to fall back on — a
  // stale query param, a roster that has not landed yet — still has to leave a
  // field a reader recognises as empty rather than a box that has shrunk to its
  // chevron and reads as half-drawn. A non-breaking space rather than a CSS
  // floor: it needs no copy (every user-facing string here comes from the
  // catalog) and the suite can assert it, which it cannot do for a stylesheet
  // jsdom never applies.
  const face = selected?.label ?? field.placeholder ?? "\u00a0";
  return (
    <TriggerButton
      field={field}
      listbox={listbox}
      animate={animate}
      face={face}
      placeholderShown={!selected}
      faceLang={selected?.lang}
      adornment={selected?.adornment}
    />
  );
}

// The one closed face both dropdowns wear: everything here — the combobox
// role, the expanded state, the activedescendant wiring, the chevron's turn —
// is identical between one value and many, and a second copy of it would be a
// second place for the ARIA contract to rot.
function TriggerButton({
  field,
  listbox,
  animate,
  face,
  placeholderShown,
  faceLang,
  adornment,
}: Readonly<{
  field: Readonly<{
    id?: string;
    className?: string;
    disabled?: boolean;
    required?: boolean;
    "aria-label"?: string;
    "aria-labelledby"?: string;
    "aria-describedby"?: string;
    "aria-invalid"?: boolean;
  }>;
  listbox: Listbox;
  animate: boolean;
  face: string;
  placeholderShown: boolean;
  faceLang?: string;
  adornment?: ReactNode;
}>) {
  const { open, active } = listbox;
  return (
    <button
      type="button"
      ref={listbox.trigger}
      id={field.id}
      className={["input", "select-control", field.className ?? ""]
        .filter(Boolean)
        .join(" ")}
      // The chevron's turn resolves here for the same reason the popup's entry
      // does — one decision, in one place the suite can assert — and `none`
      // leaves the END state: the chevron still points at an open list, it just
      // gets there without a tween.
      data-motion={animate ? "in" : "none"}
      // NOSONAR: an ARIA combobox over a native <select>, which no engine lets
      // us style past its closed face — see the module comment.
      role="combobox"
      aria-expanded={open}
      aria-haspopup="listbox"
      // Only while open: an aria-controls pointing at an element that is not in
      // the document is an invalid reference, which axe reports and a screen
      // reader cannot follow.
      aria-controls={open ? listbox.listboxId : undefined}
      aria-activedescendant={
        open && active !== -1 ? listbox.optionDomId(active) : undefined
      }
      aria-label={field["aria-label"]}
      aria-labelledby={field["aria-labelledby"]}
      aria-describedby={field["aria-describedby"]}
      aria-invalid={field["aria-invalid"]}
      aria-required={field.required}
      disabled={field.disabled}
      onClick={listbox.onTriggerClick}
      onKeyDown={listbox.onKeyDown}
    >
      {/* The closed face repeats the selected option's adornment, so a picker
          whose options are told apart BY the mark still shows which one is
          chosen once the list is shut. Hidden from assistive tech for the same
          reason it is in the list: the label carries the meaning. */}
      {adornment && (
        <span className="select-option-adornment" aria-hidden="true">
          {adornment}
        </span>
      )}
      <span
        className={
          placeholderShown
            ? "select-face select-face-placeholder"
            : "select-face"
        }
        // The face repeats a selected option's label, so it inherits that
        // option's language declaration. A placeholder is our own copy and is
        // therefore in the document's language, which is why the caller passes
        // this from the selected option rather than from the face string.
        lang={faceLang}
      >
        {face}
      </span>
      <ChevronDown className="select-chevron" size={16} aria-hidden="true" />
    </button>
  );
}

function SelectPopup({
  options,
  selected,
  multiselectable,
  frame,
  listbox,
  animate,
}: Readonly<{
  options: readonly SelectOption[];
  // Membership, not a value: the single select asks "is this THE value", the
  // multi asks "is this IN the set", and the popup draws both the same way.
  selected: (option: SelectOption) => boolean;
  multiselectable?: boolean;
  frame: PopupFrame;
  listbox: Listbox;
  animate: boolean;
}>) {
  return (
    <div
      ref={listbox.popup}
      className="select-popup"
      // Reduced motion resolves in one place — the hook, not a second media
      // query in the stylesheet — so the decision is assertable by the suite
      // rather than only visible in a browser.
      data-motion={animate ? "in" : "none"}
      data-above={frame.above ? "true" : undefined}
      // The trigger's width is the list's FLOOR, not its width — see
      // `contentSizedPopupBox` and the cap in select.css.
      style={contentSizedPopupBox(frame)}
    >
      {/* Divs rather than ul/li: `role="listbox"` and `role="option"` are the
          semantics, and a list element that also claims an interactive role is
          announced twice over. The listbox carries no name of its own either —
          the combobox that owns it is named, and a second name on the popup is
          read out on top of it. */}
      <div
        className="select-list"
        id={listbox.listboxId}
        role="listbox"
        aria-multiselectable={multiselectable}
      >
        {options.map((option, index) => (
          // biome-ignore lint/a11y/useKeyWithClickEvents: the keyboard path is the combobox trigger's own keydown handling
          // biome-ignore lint/a11y/useFocusableInteractive: an option in an aria-activedescendant listbox must NOT be focusable — focus stays on the combobox, which is what makes typeahead and Escape work
          <div // NOSONAR: keyboard path is the combobox's own keydown; an activedescendant option must not be focusable
            key={option.value}
            id={listbox.optionDomId(index)}
            role="option"
            aria-selected={selected(option)}
            aria-disabled={option.disabled}
            className={[
              "select-option",
              index === listbox.active ? "is-active" : "",
              option.disabled ? "is-disabled" : "",
            ]
              .filter(Boolean)
              .join(" ")}
            onClick={option.disabled ? undefined : () => listbox.pick(index)}
            // A press on an option must not steal focus from the combobox: in
            // the multi list, which stays open after a toggle, Escape and the
            // arrow keys have to keep reaching the trigger's own handler. On
            // the rows rather than the container, so the list's scrollbar
            // still drags.
            onMouseDown={(event) => event.preventDefault()}
            onMouseEnter={
              option.disabled ? undefined : () => listbox.hover(index)
            }
          >
            {option.adornment && (
              <span className="select-option-adornment" aria-hidden="true">
                {option.adornment}
              </span>
            )}
            <span className="select-option-label" lang={option.lang}>
              {option.label}
            </span>
            {selected(option) && (
              <Check className="select-option-check" size={14} aria-hidden />
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
