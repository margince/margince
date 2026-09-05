// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { MarginceCoreScene } from "../design-system/margince-core";
import { formatNumber, formatPercent } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useConnectors } from "../screens/connectors";
import { liveCapture } from "./capture-progress";
import type { Route } from "./router";
import "./capture-chip.css";

/** Where the chip leads: the connections card, which holds the run in full. */
const CONNECTIONS_HREF = "#/settings/connections";

/**
 * The import, in the reader's eyeline.
 *
 * A connect-time import runs for minutes to hours and the person who started
 * it goes back to work while it does. The orb in the rail carries its ring, but
 * the rail is the corner of the window and at phone width a cell in the bar;
 * this puts the same reading top-centre of the content, small, for as long as
 * mail is arriving, and takes itself away the moment it stops. A chip rather
 * than a banner: a banner is a sentence about the workspace, and this is a
 * gauge.
 *
 * It draws a SECOND Core, at chip size, and that is a deliberate exception to
 * the shell's one-Core rule (app/shell.tsx). The rule exists because two Cores
 * drawn for one agent disagree about how many agents there are; this one is
 * drawn for one RUN, wears that run's ring, and exists only while the run
 * does — it is the rail's orb come closer for the duration, and it leaves with
 * the work. Both read the same connections body, so they cannot disagree
 * about the share.
 *
 * It is a link and not a button: pressing it goes to the connections card,
 * where the import can be watched in full or stopped. On that card it is
 * absent, because a link to the page under it is a link to nowhere and the
 * card already draws the run.
 */
export function CaptureChip({ route }: Readonly<{ route: Route }>) {
  const t = useT();
  const { locale } = useLocale();
  const connectors = useConnectors();
  const capture = liveCapture(connectors.data?.data ?? []);
  const onConnections =
    route.screen === "settings" && route.id === "connections";
  if (capture === null || onConnections) {
    return null;
  }
  const detail =
    capture.fraction === null || capture.estimated === null
      ? t("shell.capture.count", {
          scanned: formatNumber(capture.scanned, locale),
        })
      : t("shell.capture.share", {
          percent: formatPercent(capture.fraction, locale),
          scanned: formatNumber(capture.scanned, locale),
          total: formatNumber(capture.estimated, locale),
        });
  return (
    // The live region is the wrapper and the link is inside it: a link that was
    // itself a status would announce every poll's new share as a navigation.
    <div className="capturechip" role="status">
      <a
        className="capturechip-link"
        href={CONNECTIONS_HREF}
        aria-label={`${t("shell.capture.importing")}. ${detail}. ${t("shell.capture.open")}`}
      >
        {/* Always `ingest`: this chip exists only while mail is arriving, so
            the one state it can honestly wear is the one named for that. The
            rail's orb may be showing a fault or a named run at the same moment;
            that is the agent's standing, and this is the import's. Dark
            surface, because the chip's own ground is the rail's. */}
        <MarginceCoreScene
          state="ingest"
          size="md"
          surface="dark"
          progress={capture.fraction ?? undefined}
          className="capturechip-core"
        />
        <span className="capturechip-words">
          <span className="capturechip-line">
            {t("shell.capture.importing")}
          </span>
          <span className="capturechip-detail">{detail}</span>
        </span>
      </a>
    </div>
  );
}
