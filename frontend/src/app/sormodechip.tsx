// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { useSorMode } from "../screens/common";
import { useVisibleSettingsPages } from "../screens/settingsnav";

// The system-of-record mode chip: in overlay mode the whole installation
// reads from an incumbent mirror, which changes what every screen can do —
// list sort/filter dials narrow, some reads answer "not available", and
// mirrored-record writes refuse. Any seat looking at the topbar should be
// able to tell this is happening, not just the admin who is on the Settings
// card that manages it. It reads the already-cached ["me"] query (the same
// probe AuthGate already resolved before any screen mounted), so mounting it
// costs no extra request and it never renders ahead of a real answer. Native
// mode renders nothing — the chip is a state marker, not a permanent fixture.
//
// The MODE is for every seat; the DESTINATION is not. A link offered to somebody
// the page refuses would land them on the access boundary — a chip that lied
// about where it went. So the fact keeps its place for everyone, and the link
// is offered exactly to whoever the page opens for.
//
// Usually that means whoever may CHANGE the installation's wiring, but not
// always: a composed workspace unit puts its settings on that page and nowhere
// else, which opens it without either wiring write. Asking the catalog covers
// both without this file having to know either rule.
//
// It asks the catalog whether this reader may open the page, rather than
// re-deriving the grants: `admin || ops` matched the seeded roles by
// coincidence and would have left a custom role holding
// `overlay_connection:update` looking at a chip that went nowhere. Two writers
// of one rule drift; the page's own answer cannot.
export function SorModeChip() {
  const t = useT();
  const mode = useSorMode();
  const pages = useVisibleSettingsPages();
  const reaches = pages.some((page) => page.id === "integrations");
  if (mode !== "overlay") {
    return null;
  }
  // Same element, same tokens, same words either way: what changes is whether
  // it is a link. A `<span>` with a title and an accessible name still reports
  // the mode to a screen reader; what it no longer does is promise a page.
  const label = t("overlay.chipLabel");
  const explanation = t("overlay.chipAria");
  return reaches ? (
    <a
      // Integrations, not Connections: the mirror that is answering every read is
      // installation-wide wiring, and the personal Connections entry now holds
      // only a reader's own mailbox and network.
      href="#/settings/integrations"
      className="badge badge-accent"
      title={explanation}
      aria-label={explanation}
    >
      {label}
    </a>
  ) : (
    <span className="badge badge-accent" title={explanation}>
      {label}
      {/* The explanation as text rather than as `aria-label`: a plain span has
          no role for that attribute to belong to, so a screen reader is not
          obliged to read it. Visually hidden text is announced with the chip
          and the `title` above still serves a hover. */}
      <span className="sr-only"> {explanation}</span>
    </span>
  );
}
