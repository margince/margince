// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown } from "lucide-react";
import { useCallback, useId, useRef, useState } from "react";
import { AppLink } from "../design-system/applink";
import { Modal } from "../design-system/atoms";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { useLocale, useT } from "../i18n";
import {
  entryLabel,
  type NavLevelEntry,
  type NavLevelGroup,
  type NavSection,
  navEntryHref,
} from "./nav";

// One group of the switcher's list, grouped as the sidebar's level is.
function SectionPickGroup({
  section,
  group,
  activeId,
  onPick,
}: Readonly<{
  section: NavSection;
  group: NavLevelGroup;
  activeId: string;
  onPick: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The sheet's title already names the section, so the group carrying that
  // name goes unheaded here.
  const heading =
    group.headingKey === section.titleKey ? undefined : group.headingKey;
  return (
    <div className="sectionpickgroup">
      {heading && (
        <Heading size="xsmall" as="h3">
          {t(heading)}
        </Heading>
      )}
      {group.items.map((entry) => (
        <AppLink
          key={entry.id}
          className="t-body"
          href={navEntryHref([section.screen], entry)}
          aria-current={entry.id === activeId ? "page" : undefined}
          // The sheet covers the page it just navigated to, so a row that acts
          // takes the sheet with it.
          onClick={onPick}
        >
          <entry.icon size={16} aria-hidden />
          {entryLabel(entry, locale, t)}
        </AppLink>
      ))}
    </div>
  );
}

/**
 * A section's pages at phone width, where the sidebar is a bar of four places.
 * The links claim the current page; the button only opens them.
 */
export function SectionSwitcher({
  section,
  entry,
}: Readonly<{ section: NavSection; entry: NavLevelEntry }>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const titleId = useId();
  const list = useRef<HTMLDivElement>(null);
  const close = useCallback(() => setOpen(false), []);
  // Focus lands on the reader's own page, not the search box. A focused field
  // raises the phone keyboard over the list.
  const currentRow = useCallback(() => {
    const row = list.current?.querySelector('[aria-current="page"]');
    if (!(row instanceof HTMLElement)) {
      return null;
    }
    row.scrollIntoView?.({ block: "center" });
    return row;
  }, []);
  const label = entryLabel(entry, locale, t);
  return (
    <>
      <button
        type="button"
        className="pageswitch"
        aria-haspopup="dialog"
        aria-expanded={open}
        // The accessible name keeps the visible word inside it (WCAG 2.5.3).
        aria-label={t("shell.sectionSwitch", { name: label })}
        onClick={() => setOpen(true)}
      >
        <span>{label}</span>
        <ChevronDown size={16} aria-hidden />
      </button>
      <Modal
        open={open}
        onClose={close}
        labelledBy={titleId}
        intent="drawer"
        initialFocusTo={currentRow}
      >
        {/* The title and search stay put while the list scrolls under them. */}
        <DrawerHead>
          {/* Named by the SECTION: the list is everything Settings holds, and
              the entry the reader came from is marked inside it. */}
          <Heading size="large" id={titleId} className="t-h2 modal-title">
            {t(section.titleKey)}
          </Heading>
          {section.leadFor?.(close)}
        </DrawerHead>
        <DrawerBody>
          <div className="sectionpick" ref={list}>
            {section.groups.map((group, index) => (
              <SectionPickGroup
                key={group.headingKey ?? `group-${index}`}
                section={section}
                group={group}
                activeId={entry.id}
                onPick={close}
              />
            ))}
          </div>
        </DrawerBody>
      </Modal>
    </>
  );
}
