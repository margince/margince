import { type KeyboardEvent, type RefObject, useEffect } from "react";
import { ChoiceList } from "../design-system/choicelist";
import { useT } from "../i18n";
import {
  setThemeChoice,
  THEME_CHOICES,
  type ThemeChoice,
  useThemeChoice,
} from "../lib/theme";

/** The appearance choices, in the order the submenu shows them, named. */
export const THEME_LABEL_KEYS = {
  light: "theme.light",
  dark: "theme.dark",
  system: "theme.system",
} as const satisfies Record<ThemeChoice, string>;

/**
 * Light / Dark / System: one question with three answers, so it is built on
 * the product's radio group rather than a second copy of one. `ChoiceList`
 * carries the `fieldset` that names the question and the native radios that
 * answer it. That is also the keyboard contract: a radio group already owns
 * the roving tabstop and the arrow walk this hand-rolled before.
 *
 * Picking one does not close the panel. This is the appearance control. All
 * of its feedback is the document repainting under the open flyout, with the
 * tick moving to the chosen answer. Closing on pick would take that away
 * and make the reader re-open it to see what they chose.
 *
 * Escape has no handler here. `usePopoverDismiss` (app/popover.ts) dismisses
 * this layer. The parent menu's copy of the same hook stands down while the
 * row that opened this one still reads `aria-expanded="true"`. One keystroke,
 * one layer, one implementation.
 */
export function ThemeSubmenu({
  id,
  panel,
  onBack,
}: Readonly<{
  /** What the row that opened this points `aria-controls` at. */
  id: string;
  panel: RefObject<HTMLDivElement | null>;
  onBack: () => void;
}>) {
  const t = useT();
  const choice = useThemeChoice();

  // Where the reader starts: the answer that is already standing, so opening the
  // chooser puts them on the appearance they currently have. A radio group makes
  // that one the group's only tab stop, so it is the one to focus.
  useEffect(() => {
    panel.current
      ?.querySelector<HTMLInputElement>('input[type="radio"]:checked')
      ?.focus();
  }, [panel]);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // Left goes back to the row that opened this, as it did when these were
    // menu items. The group's own walk keeps Up and Down, which is all a
    // column of three needs.
    if (event.key === "ArrowLeft") {
      onBack();
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    // The parent menu listens on the same bubble path, so without this its own
    // roving tabstop moves under a reader who is standing in here. Not
    // prevented: the arrow walk that must run is the radio group's.
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.stopPropagation();
    }
  };

  return (
    // The flyout is the positioned box and the group is what it holds. The ref
    // this hands back is what `usePopoverDismiss` tests containment against.
    // The sheet-at-phone-width rules are written for the flyout box, not a
    // fieldset.
    // biome-ignore lint/a11y/noStaticElementInteractions: the handler is a guard on the PARENT menu's bubble path, not an affordance — the controls a reader operates are the radios inside
    <div className="accountsub" id={id} ref={panel} onKeyDown={onKeyDown}>
      <ChoiceList
        legend={t("shell.theme")}
        hideLegend
        value={choice}
        choices={THEME_CHOICES.map((option) => ({
          value: option,
          label: t(THEME_LABEL_KEYS[option]),
        }))}
        onChange={setThemeChoice}
      />
    </div>
  );
}
