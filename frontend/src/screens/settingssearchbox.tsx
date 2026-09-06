// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Search } from "lucide-react";
import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";
import { navigate } from "../app/router";
import { useT } from "../i18n";
import type { SettingsPage } from "./settingscatalog";
import { settingsHref } from "./settingsrouting";
import { settingsSearch } from "./settingssearch";

/**
 * Finding a settings page by typing.
 *
 * NOT `ComboBox` or `useSuggestList`, and the reason is the contract rather
 * than the looks. Those bind a VALUE: the list is help toward what goes in the
 * box, they do their own matching (`matchingSuggestions`), and an empty query
 * offers everything because everything is a candidate value. This one
 * NAVIGATES: the box is never the answer, the ranking is
 * permission-filtered and computed by `settingsSearch`, and an empty query must
 * offer nothing — the sidebar beside it is already the list of everything.
 *
 * Wiring this to that hook would mean overriding its matcher, its empty case
 * and its pick semantics, which is the shape of reuse that leaves both callers
 * worse. The keyboard grammar below is deliberately the SAME grammar, because a
 * reader should not have to learn two.
 */
export function SettingsSearchBox({
  pages,
}: Readonly<{ pages: readonly SettingsPage[] }>) {
  const t = useT();
  const [typed, setTyped] = useState("");
  // -1 is "nothing active", which is where a fresh query starts: the first row
  // is not chosen until the reader reaches for it, so Enter on an untouched
  // query does nothing rather than opening a page they never looked at.
  const [active, setActive] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);
  const activeRef = useRef<HTMLAnchorElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  // Closed by a click outside or by Escape, without clearing what was typed —
  // a reader who looks away has not abandoned their query.
  const [dismissed, setDismissed] = useState(false);
  const listboxId = useId();

  // The active row kept in view. The list is capped at 60vh, so wrapping
  // ArrowUp from the first hit selects the LAST one — and without this, Enter
  // then opens a page the reader never saw selected.
  useEffect(() => {
    // `active` is read here rather than only in the dependency list, so the
    // effect's subject is visible in its own body: it runs BECAUSE the active
    // row moved, and a dependency array carrying a value the body never
    // mentions is the shape a linter is right to question.
    if (active < 0) {
      return;
    }
    // Guarded on the METHOD, not the element: jsdom implements neither
    // scrolling nor this call, and a keyboard test should fail on the keyboard
    // rather than on a browser API the environment does not have.
    activeRef.current?.scrollIntoView?.({ block: "nearest" });
  }, [active]);

  const hits = settingsSearch(typed, pages, t);
  // Whether the popup is ON SCREEN, which is what `aria-expanded` must report.
  // It was `hits.length > 0`, so a query matching nothing rendered a visible
  // popup while the combobox told assistive technology it was collapsed.
  const showList = typed.trim() !== "" && !dismissed;
  const open = showList;

  // A click anywhere else closes the popup, without clearing the query. Without
  // it the list stayed over the page after the reader had plainly moved on.
  useEffect(() => {
    if (!showList) {
      return;
    }
    function onPointerDown(event: MouseEvent) {
      if (!wrapRef.current?.contains(event.target as Node)) {
        setDismissed(true);
      }
    }
    document.addEventListener("mousedown", onPointerDown);
    return () => document.removeEventListener("mousedown", onPointerDown);
  }, [showList]);
  // The id of one option's element, built HERE rather than interpolated at each
  // call site. Two reasons, and the second is why it is a function: the row and
  // the `aria-activedescendant` pointing at it must spell the same id or the
  // reference dangles, and an index interpolated into JSX is what
  // `format/jsx-magnitude.test.ts` reads as an unformatted magnitude reaching a
  // reader. This one never reaches one — it is a DOM id — and the same shape
  // (`optionDomId` in design-system/suggestlist.tsx) is how the rest of the
  // tree says so.
  const optionDomId = (index: number) => `${listboxId}-option-${index}`;

  function go(index: number) {
    const hit = hits[index];
    if (!hit) {
      return;
    }
    // Closed before the move, or the list would flash over the page the reader
    // has just arrived at.
    setTyped("");
    setActive(-1);
    navigate(settingsHref(hit.page.id));
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Escape") {
      // The text goes and the focus STAYS. A reader who clears a search is
      // still searching; throwing them back to the page would make them find
      // the box again to type the next word.
      event.preventDefault();
      setTyped("");
      setActive(-1);
      return;
    }
    if (!open) {
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((current) => (current + 1) % hits.length);
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((current) => (current <= 0 ? hits.length - 1 : current - 1));
      return;
    }
    if (event.key === "Enter" && active >= 0) {
      event.preventDefault();
      go(active);
    }
  }

  return (
    <div className="settingssearch" ref={wrapRef}>
      {/* The combobox is the INPUT, not the wrapper: a wrapper carrying the
          role would put the text field inside its own combobox, and a screen
          reader would announce a group where there is a control. */}
      <Search aria-hidden="true" />
      <input
        ref={inputRef}
        type="text"
        role="combobox"
        aria-expanded={open}
        aria-controls={listboxId}
        // `list` rather than `both`: nothing is written into the box on
        // arrowing, so claiming inline completion would promise a behaviour
        // this control does not have.
        aria-autocomplete="list"
        aria-activedescendant={active >= 0 ? optionDomId(active) : undefined}
        aria-label={t("settings.search.label")}
        placeholder={t("settings.search.placeholder")}
        value={typed}
        onChange={(event) => {
          setTyped(event.target.value);
          setActive(-1);
          // Typing is the reader coming back to it.
          setDismissed(false);
        }}
        onKeyDown={onKeyDown}
      />
      {showList && (
        /* A div rather than ul/li: `role="listbox"` on a list element is a
           second role over one that already means something. */
        <div className="settingssearch-list" id={listboxId} role="listbox">
          {hits.length === 0 ? (
            /* Not an option: there is nothing to choose, and a listbox with one
               unselectable row would let a reader arrow onto a sentence. */
            <p className="settingssearch-empty">{t("settings.search.none")}</p>
          ) : (
            hits.map((hit, index) => (
              /* An anchor, so a hit is a real address — copyable, and openable
                 in a tab by every modifier the browser understands.

                 `tabIndex={-1}`: a combobox's popup is driven by
                 `aria-activedescendant` from the input, and its rows must not
                 join the page's Tab sequence. Tab from the box goes on to the
                 next control, as the pattern requires.

                 `onClick`, not `onMouseDown`: activating on mousedown fires
                 before the click completes, which takes a drag-to-cancel away
                 and beats the browser to its own modifier handling. The blur
                 problem it was solving is solved by `onMouseDown`'s
                 preventDefault below, which keeps focus in the input WITHOUT
                 acting. */
              <a
                key={hit.page.id}
                id={optionDomId(index)}
                role="option"
                tabIndex={-1}
                aria-selected={index === active}
                ref={index === active ? activeRef : undefined}
                className={
                  index === active
                    ? "settingssearch-hit active"
                    : "settingssearch-hit"
                }
                href={`#/settings/${hit.page.id}`}
                onMouseDown={(event) => {
                  // Focus stays in the box; nothing is opened here.
                  event.preventDefault();
                }}
                onClick={(event) => {
                  // Every modified click belongs to the BROWSER — a new tab, a
                  // new window, a download. Only the plain one is ours.
                  if (
                    event.metaKey ||
                    event.ctrlKey ||
                    event.shiftKey ||
                    event.altKey
                  ) {
                    return;
                  }
                  event.preventDefault();
                  go(index);
                }}
              >
                <span className="settingssearch-label">{hit.label}</span>
                {/* `t-caption`, the catalogued utility, rather than a rule of
                    its own saying the same two properties. */}
                <span className="t-caption">{hit.group}</span>
              </a>
            ))
          )}
        </div>
      )}
      {/* The count, for a reader who cannot see the list appear. Polite, so it
          waits for a pause in typing rather than interrupting every keystroke. */}
      <p className="sr-only" role="status" aria-live="polite">
        {typed.trim() === ""
          ? ""
          : t("settings.search.count").replace("{n}", String(hits.length))}
      </p>
    </div>
  );
}
