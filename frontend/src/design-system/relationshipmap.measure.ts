// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type RefObject,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
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

/** A kind is null until drawn, or with no canvas (happy-dom), so the caller
 * falls back to counting characters. */
export function useMapTextMeasures(
  scope: RefObject<Element | null>,
): MapTextMeasures {
  const [key, setKey] = useState("");
  const landed = useRef(0);
  // After every commit, because the drawn labels decide which kinds exist. An
  // unchanged key sets the same string, which renders nothing.
  useLayoutEffect(() => {
    const sample = () => setKey(keyOf(scope.current, landed.current));
    sample();
  });
  // A face that lands changes the metrics but not the font names, so it bumps
  // the key. Subscribed in the commit, so no face lands unheard before it.
  useLayoutEffect(() => {
    const fonts = globalThis.document?.fonts;
    if (!fonts?.addEventListener) {
      return;
    }
    const resample = () => {
      landed.current += 1;
      setKey(keyOf(scope.current, landed.current));
    };
    fonts.addEventListener("loadingdone", resample);
    fonts.addEventListener("loadingerror", resample);
    return () => {
      fonts.removeEventListener("loadingdone", resample);
      fonts.removeEventListener("loadingerror", resample);
    };
  }, [scope]);
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

function keyOf(scope: Element | null, landed: number): string {
  return [
    String(landed),
    ...SELECTORS.map((selector) => fontOf(scope?.querySelector(selector))),
  ].join("\n");
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
  // A context that cannot measure would cut every label to "…".
  const probe = context.measureText("M").width;
  if (!Number.isFinite(probe) || probe <= 0) {
    return null;
  }
  return (text) => context.measureText(text).width;
}
