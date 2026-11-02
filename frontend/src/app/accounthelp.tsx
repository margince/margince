// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ExternalLink } from "lucide-react";
import { type KeyboardEvent, type RefObject, useEffect } from "react";
import { OffsiteLink } from "../design-system/offsitelink";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { AccountRelease, reportedRelease } from "./accountfoot";
import { type RowSeat, useRovingSeats } from "./accountseats";

const REPOSITORY = "https://github.com/margince/margince";

/** The bug form, with its `commit` field filled in when the bundle knows its release. */
export function reportProblemHref(): string {
  const url = new URL(`${REPOSITORY}/issues/new`);
  url.searchParams.set("template", "bug_report.yml");
  const release = reportedRelease();
  if (release) {
    url.searchParams.set("commit", release);
  }
  return url.toString();
}

const REQUEST_FEATURE_HREF = `${REPOSITORY}/issues/new?template=capability_gap.yml`;
const RESOURCES_HREF = "https://margince.com/resources/";
const RELEASE_NOTES_HREF = `${REPOSITORY}/releases`;

const REPORT_SEAT = 0;
const REQUEST_SEAT = 1;
const RESOURCES_SEAT = 2;
const SHORTCUTS_SEAT = 3;
const RELEASE_NOTES_SEAT = 4;
const SEAT_COUNT = 5;

/**
 * A menu item that leaves the app. It opens a new tab, and says so twice: a
 * glyph for the eye and a clipped sentence for a screen reader.
 */
function OffsiteRow({
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
        <span className="sr-only"> {t("shell.help.newTab")}</span>
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
}: Readonly<{
  id: string;
  panel: RefObject<HTMLDivElement | null>;
  onBack: () => void;
  /** Closes the whole menu, for a row that takes the reader elsewhere. */
  onLeave: () => void;
  onShortcuts: () => void;
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
      <OffsiteRow
        href={reportProblemHref()}
        labelKey="shell.help.report"
        seat={seat(REPORT_SEAT)}
        onLeave={onLeave}
      />
      <OffsiteRow
        href={REQUEST_FEATURE_HREF}
        labelKey="shell.help.request"
        seat={seat(REQUEST_SEAT)}
        onLeave={onLeave}
      />
      <OffsiteRow
        href={RESOURCES_HREF}
        labelKey="shell.help.resources"
        seat={seat(RESOURCES_SEAT)}
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
      <OffsiteRow
        href={RELEASE_NOTES_HREF}
        labelKey="shell.help.releaseNotes"
        seat={seat(RELEASE_NOTES_SEAT)}
        onLeave={onLeave}
      />
      <hr />
      <AccountRelease />
    </div>
  );
}
