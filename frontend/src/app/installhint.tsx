// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Share } from "lucide-react";
import { type RefObject, useEffect, useRef } from "react";
import { InstallButton } from "../components/installbutton";
import { Callout } from "../design-system/callout";
import { calendarDay } from "../format/calendarday";
import { useNow } from "../format/now";
import { useT } from "../i18n";
import { useConnectivity } from "../lib/connectivity";
import {
  installHintDue,
  noteHintDismissed,
  noteInstalled,
  noteShellDay,
  useInstallHintRecord,
} from "../lib/installhint";
import { type InstallState, useInstallState } from "../lib/pwa";
import { useRoute } from "../lib/router";
import { useViewerZone } from "../lib/viewerzone";
import { useCoarsePointer } from "../lib/viewport";
import { useCaptureConcerns } from "./capturehealthbanner";
import { useEconomyBand } from "./economybanner";
import { useReindexNeeded } from "./embedreindexbanner";
import { useLicenseNotice } from "./licensebanner";
import "./installhint.css";

export type HintedInstall = Extract<
  InstallState,
  { kind: "available" | "manual-ios" }
>;

// Each banner's own condition, before any dismissal it keeps for the session:
// a notice the reader put away for now still goes first.
function useBannerAhead(): boolean {
  const offline = useConnectivity() !== "online";
  const capture = useCaptureConcerns().length > 0;
  const economy = useEconomyBand().warns;
  const reindex = useReindexNeeded();
  const license = useLicenseNotice() !== null;
  return offline || capture || economy || reindex || license;
}

/** The offer to install the app, on Home on a phone or tablet, once the
 *  schedule in lib/installhint.ts says it is due and no other banner stands. */
export function InstallHint({
  scroller,
}: Readonly<{ scroller: RefObject<HTMLElement | null> }>) {
  const route = useRoute();
  const install = useInstallState();
  const coarse = useCoarsePointer();
  const now = useNow(60_000);
  const today = calendarDay(new Date(now), useViewerZone());
  const record = useInstallHintRecord();
  const ahead = useBannerAhead();
  useEffect(() => noteShellDay(today), [today]);
  useEffect(() => {
    if (install.kind === "installed") noteInstalled();
  }, [install.kind]);
  if (
    route.screen !== "home" ||
    !coarse ||
    ahead ||
    !installHintDue(record, today, now) ||
    (install.kind !== "available" && install.kind !== "manual-ios")
  ) {
    return null;
  }
  return (
    <InstallHintNotice
      install={install}
      onDismiss={() => noteHintDismissed(Date.now())}
      scroller={scroller}
    />
  );
}

export function InstallHintNotice({
  install,
  onDismiss,
  scroller,
}: Readonly<{
  install: HintedInstall;
  onDismiss: () => void;
  scroller: RefObject<HTMLElement | null>;
}>) {
  const t = useT();
  const box = useRef<HTMLDivElement>(null);
  // The hint leaves the page on every answer, so focus inside it goes on to
  // the page it stood over, never to <body>.
  const handOnFocus = () => {
    const active = document.activeElement;
    if (
      active === null ||
      active === document.body ||
      box.current?.contains(active)
    ) {
      scroller.current?.focus({ preventScroll: true });
    }
  };
  const dismiss = () => {
    handOnFocus();
    onDismiss();
  };
  return (
    <div className="appbanner" ref={box}>
      <Callout
        tone="discovery"
        kind="standing"
        title={t("installHint.title")}
        actions={
          install.kind === "available" ? (
            <InstallButton
              prompt={install.prompt}
              onAnswered={(outcome) => {
                if (outcome === "dismissed") dismiss();
                else handOnFocus();
              }}
            />
          ) : undefined
        }
        dismiss={{ label: t("installHint.notNow"), onDismiss: dismiss }}
      >
        {install.kind === "available" ? t("installHint.value") : <ShareStep />}
      </Callout>
    </div>
  );
}

function ShareStep() {
  const t = useT();
  const [before, after = ""] = t("installHint.manual").split("{share}");
  return (
    <>
      {before}
      <Share className="installhint-share" size={16} aria-hidden="true" />
      {after}
    </>
  );
}
