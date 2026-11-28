// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Share } from "lucide-react";
import {
  type RefObject,
  useEffect,
  useLayoutEffect,
  useRef,
  useSyncExternalStore,
} from "react";
import { InstallButton } from "../components/installbutton";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { calendarDay } from "../format/calendarday";
import { useNow } from "../format/now";
import { useT } from "../i18n";
import { connectivityNow, subscribeConnectivity } from "../lib/connectivity";
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
import { useLicenseNoticeAt } from "./licensebanner";
import "./installhint.css";

export type HintedInstall = Extract<
  InstallState,
  { kind: "available" | "manual-ios" }
>;

// A banner whose read is still out counts as ahead, so the hint never flashes.
function useBannerAhead(now: number): boolean {
  const offline =
    useSyncExternalStore(subscribeConnectivity, connectivityNow) !== "online";
  const capture = useCaptureConcerns();
  const economy = useEconomyBand();
  const reindex = useReindexNeeded();
  const license = useLicenseNoticeAt(now);
  return (
    offline ||
    capture === undefined ||
    capture.length > 0 ||
    !economy.known ||
    economy.warns ||
    reindex !== false ||
    license !== null
  );
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
  const ahead = useBannerAhead(now);
  useEffect(() => noteShellDay(today), [today]);
  useEffect(() => {
    if (install.kind === "installed") noteInstalled();
  }, [install.kind]);
  if (
    route.screen !== "home" ||
    !coarse ||
    (install.kind === "available" && install.browserOffered === false) ||
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
  // Focus inside the hint goes on to the page however the hint leaves. A layout
  // cleanup still sees the hint's nodes in place.
  useLayoutEffect(() => {
    const node = box.current;
    return () => {
      if (node?.contains(document.activeElement)) {
        scroller.current?.focus({ preventScroll: true });
      }
    };
  }, [scroller]);
  return (
    <div className="appbanner" ref={box}>
      <Callout
        tone="discovery"
        kind="standing"
        title={t("installHint.title")}
        actions={
          <>
            <Button variant="ghost" onClick={onDismiss}>
              {t("installHint.notNow")}
            </Button>
            {install.kind === "available" && (
              <InstallButton
                prompt={install.prompt}
                onAnswered={(outcome) => {
                  if (outcome === "dismissed") onDismiss();
                }}
              />
            )}
          </>
        }
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
