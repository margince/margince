// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type RefObject, useMemo, useSyncExternalStore } from "react";
import type { MeasureText } from "./relationshipmap.layout";

/** Each kind of word on the map, measured in the type its own element draws. */
export type MapTextMeasures = Readonly<{
  name: MeasureText | null;
  sub: MeasureText | null;
  pill: MeasureText | null;
  lane: MeasureText | null;
  more: MeasureText | null;
}>;

const SELECTORS = [
  ".rmap-name",
  ".rmap-sub",
  "[class^='rmap-pill-']",
  ".rmap-lane",
  ".rmap-more text",
] as const;

/**
 * useMapTextMeasures measures the map's words in the type `scope` draws them
 * in. A kind is null until one of its elements is drawn, and wherever no canvas
 * exists (jsdom), so the caller falls back to counting characters.
 */
export function useMapTextMeasures(
  scope: RefObject<Element | null>,
): MapTextMeasures {
  // The fonts' load state rides in the key, so a face that lands late
  // re-measures although the computed font names did not change.
  const key = useSyncExternalStore(
    watchFonts,
    () =>
      [
        globalThis.document?.fonts?.status ?? "",
        ...SELECTORS.map((selector) =>
          fontOf(scope.current?.querySelector(selector)),
        ),
      ].join("\n"),
    () => "",
  );
  return useMemo(() => {
    const [, name, sub, pill, lane, more] = key.split("\n");
    return {
      name: canvasMeasure(name),
      sub: canvasMeasure(sub),
      pill: canvasMeasure(pill),
      lane: canvasMeasure(lane),
      more: canvasMeasure(more),
    };
  }, [key]);
}

function watchFonts(changed: () => void) {
  const fonts = globalThis.document?.fonts;
  if (!fonts?.addEventListener) {
    return () => undefined;
  }
  fonts.addEventListener("loadingdone", changed);
  fonts.addEventListener("loadingerror", changed);
  return () => {
    fonts.removeEventListener("loadingdone", changed);
    fonts.removeEventListener("loadingerror", changed);
  };
}

function fontOf(element: Element | null | undefined): string {
  if (!element) {
    return "";
  }
  const style = getComputedStyle(element);
  if (!style.fontSize || !style.fontFamily) {
    return "";
  }
  return `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
}

function canvasMeasure(font: string | undefined): MeasureText | null {
  if (!font) {
    return null;
  }
  const context = document.createElement("canvas").getContext("2d");
  if (!context) {
    return null;
  }
  context.font = font;
  return (text) => context.measureText(text).width;
}
