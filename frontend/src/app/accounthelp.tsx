// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ExternalLink } from "lucide-react";
import { type KeyboardEvent, type RefObject, useEffect } from "react";
import { OffsiteLink } from "../design-system/offsitelink";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  buildRelease,
  RELEASE_NOTES_HREF,
  REPOSITORY,
  reportedRelease,
} from "../lib/buildrelease";
import { type RowSeat, useRovingSeats } from "./accountseats";

/** The bug form, with its `commit` field filled in when the bundle knows its release. */
export function reportProblemHref(): string {
  const url = new URL(`${REPOSITORY}/issues/new`);
  url.searchParams.set("template", "bug_report.yml");
  const release = reportedRelease(buildRelease());
  if (release) {
    url.searchParams.set("commit", release);
  }
  return url.toString();
}

const REQUEST_FEATURE_HREF = `${REPOSITORY}/issues/new?template=capability_gap.yml`;
const RESOURCES_HREF = "https://margince.com/resources/";

const REPORT_SEAT = 0;
const REQUEST_SEAT = 1;
const RESOURCES_SEAT = 2;
const RELEASE_NOTES_SEAT = 3;
const SHORTCUTS_SEAT = 4;
const ABOUT_SEAT = 5;
const SEAT_COUNT = 6;

// Help documents open in a new tab, named in text and by the external-link glyph.
function HelpLinkRow({
  href,
  labelKey,
  seat,
  onLeave,
}: Readonly<{
  href: string;
  labelKey: MessageKey;
  seat: RowSeat;
  onLeave: () => void;
}>) {
  const t = useT();
  return (
    <OffsiteLink
      className="acctrow"
      href={href}
      role="menuitem"
      {...seat}
      onClick={onLeave}
    >
      <span className="acctrowlabel">
        {t(labelKey)}
        <span className="sr-only"> {t("common.opensNewTab")}</span>
      </span>
      <ExternalLink size={14} className="acctmore" aria-hidden />
    </OffsiteLink>
  );
}

/**
 * A real menu, unlike the theme flyout: its rows go somewhere, so it walks like
 * the menu it opens from. Escape belongs to `usePopoverDismiss`, as in ThemeSubmenu.
 */
export function HelpSubmenu({
  id,
  panel,
  onBack,
  onLeave,
  onShortcuts,
  onAbout,
}: Readonly<{
  id: string;
  panel: RefObject<HTMLDivElement | null>;
  onBack: () => void;
  /** Closes the whole menu, for a row that takes the reader elsewhere. */
  onLeave: () => void;
  onShortcuts: () => void;
  onAbout: () => void;
}>) {
  const t = useT();
  const { seat, walk, move } = useRovingSeats(SEAT_COUNT);

  useEffect(() => {
    move(REPORT_SEAT);
  }, [move]);

  // The parent menu listens on the same bubble path, so a key this walks must
  // not reach it too. Tab is left to bubble: it leaves, and closes, both layers.
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "ArrowLeft") {
      onBack();
    } else if (!walk(event.key)) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
  };

  return (
    <div
      className="accountsub"
      id={id}
      role="menu"
      aria-label={t("shell.helpMenu")}
      tabIndex={-1}
      ref={panel}
      onKeyDown={onKeyDown}
    >
      <HelpLinkRow
        href={reportProblemHref()}
        labelKey="shell.help.report"
        seat={seat(REPORT_SEAT)}
        onLeave={onLeave}
      />
      <HelpLinkRow
        href={REQUEST_FEATURE_HREF}
        labelKey="shell.help.request"
        seat={seat(REQUEST_SEAT)}
        onLeave={onLeave}
      />
      <HelpLinkRow
        href={RESOURCES_HREF}
        labelKey="shell.help.resources"
        seat={seat(RESOURCES_SEAT)}
        onLeave={onLeave}
      />
      <HelpLinkRow
        href={RELEASE_NOTES_HREF}
        labelKey="shell.help.releaseNotes"
        seat={seat(RELEASE_NOTES_SEAT)}
        onLeave={onLeave}
      />
      <button
        type="button"
        className="acctrow"
        role="menuitem"
        aria-haspopup="dialog"
        {...seat(SHORTCUTS_SEAT)}
        onClick={onShortcuts}
      >
        {t("shell.help.shortcuts")}
      </button>
      <button
        type="button"
        className="acctrow"
        role="menuitem"
        aria-haspopup="dialog"
        {...seat(ABOUT_SEAT)}
        onClick={onAbout}
      >
        {t("about.title")}
      </button>
    </div>
  );
}
