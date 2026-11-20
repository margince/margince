import { useEffect } from "react";
import { SCREEN_CODE } from "./App.screencode";
import { SCREEN_CHUNKS_ID } from "./lib/screenchunks";
import { COARSE_POINTER } from "./lib/viewport";

type ScreenKey = keyof typeof SCREEN_CODE;

// The screens most sessions open next, in that order.
const FIRST: readonly ScreenKey[] = [
  "deals",
  "contacts",
  "contactPage",
  "companies",
  "leads",
];
// Screens most sessions never open.
const NEVER: readonly ScreenKey[] = [
  "settings",
  "book",
  "oauthConsent",
  "unsubscribe",
  "confirm",
  "onboarding",
  "buyerRoom",
];

const isScreenKey = (key: string): key is ScreenKey =>
  Object.hasOwn(SCREEN_CODE, key);

export const WARM_ORDER: readonly ScreenKey[] = [
  ...FIRST,
  ...Object.keys(SCREEN_CODE)
    .filter(isScreenKey)
    .filter((key) => !FIRST.includes(key) && !NEVER.includes(key)),
];

const isHref = (value: unknown): value is string => typeof value === "string";

const isIndexList = (value: unknown): value is number[] =>
  Array.isArray(value) && value.every((index) => Number.isInteger(index));

// The dev server writes no map, so there the warm-up does nothing.
function emittedScreenFiles(): Map<string, string[]> {
  const text = document.getElementById(SCREEN_CHUNKS_ID)?.textContent;
  const found = new Map<string, string[]>();
  if (!text) return found;
  let map: unknown;
  try {
    map = JSON.parse(text);
  } catch (reason: unknown) {
    console.warn("the screen chunk map could not be read", reason);
    return found;
  }
  if (typeof map !== "object" || map === null) return found;
  const files = "files" in map ? map.files : undefined;
  const screens = "screens" in map ? map.screens : undefined;
  if (!Array.isArray(files) || typeof screens !== "object" || !screens)
    return found;
  const listed: unknown[] = files;
  for (const [key, indexes] of Object.entries(screens)) {
    if (!isIndexList(indexes)) continue;
    const hrefs = indexes.map((index) => listed[index]);
    if (hrefs.every(isHref)) found.set(key, hrefs);
  }
  return found;
}

function linked(href: string): boolean {
  const url = new URL(href, document.baseURI).href;
  return [...document.querySelectorAll<HTMLLinkElement>("link[href]")]
    .map((link) => link.href)
    .concat(
      [...document.querySelectorAll<HTMLScriptElement>("script[src]")].map(
        (script) => script.src,
      ),
    )
    .includes(url);
}

function preload(href: string): Promise<void> {
  return new Promise((settled) => {
    const link = document.createElement("link");
    // A style preload fetches the sheet without applying it. It uses the CORS
    // mode of Vite's own stylesheet link, which is then served from it.
    if (href.endsWith(".css")) {
      link.rel = "preload";
      link.as = "style";
    } else {
      link.rel = "modulepreload";
    }
    link.crossOrigin = "";
    link.href = href;
    link.addEventListener("load", () => settled());
    link.addEventListener("error", () => {
      console.warn("route chunk warm-up failed", href);
      settled();
    });
    document.head.append(link);
  });
}

// A modulepreload downloads and compiles a screen's code; the code runs on the
// screen's first visit.
export function useWarmRouteChunks() {
  useEffect(() => {
    if (window.matchMedia?.(COARSE_POINTER).matches) return;
    const emitted = emittedScreenFiles();
    const queue = WARM_ORDER.flatMap((key) => {
      const files = emitted.get(key);
      return files ? [files] : [];
    });
    let cancelled = false;
    let next = 0;
    let cancelPending: (() => void) | undefined;

    const schedule = () => {
      if (cancelled || next >= queue.length) return;
      // Each screen's files can consume the idle period. Yield before the next one,
      // without a deadline that could force background work into a busy frame.
      if (typeof window.requestIdleCallback === "function") {
        const handle = window.requestIdleCallback(() => void warm());
        cancelPending = () => window.cancelIdleCallback(handle);
      } else {
        const handle = window.setTimeout(() => void warm(), 0);
        cancelPending = () => window.clearTimeout(handle);
      }
    };

    const warm = async () => {
      if (cancelled) return;
      const files = queue[next++] ?? [];
      await Promise.all(files.filter((href) => !linked(href)).map(preload));
      schedule();
    };

    schedule();
    return () => {
      cancelled = true;
      cancelPending?.();
    };
  }, []);
}
