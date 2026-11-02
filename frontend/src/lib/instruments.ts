// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The browser half of extension instrumentation. main.tsx starts every
// composed unit's instrument before the first render; React's root error
// options and the router feed them. Every call is guarded: an instrument
// that throws is logged to the console and the app goes on.

/** What a unit's browser instrument is handed when the SPA boots. */
export type BrowserInstrumentContext = Readonly<{
  /** The SPA's release version; empty when unknown. */
  version: string;
  /** The unit's session-less edge, `/v1/public/instrument/<unit>/`. */
  edge: string;
}>;

/** The hooks a browser instrument returns. Every one is optional. */
export type BrowserInstruments = {
  onError?(
    error: unknown,
    info: { componentStack?: string; caught: boolean },
  ): void;
  /** A route pattern with record ids replaced, never the raw address. */
  onNavigate?(route: string): void;
};

/** A unit's browser instrument: the named export its Go side declares. */
// A unit that has nothing to hook may return nothing, hence `void`.
export type BrowserInstrument = (
  ctx: BrowserInstrumentContext,
  // biome-ignore lint/suspicious/noConfusingVoidType: a block body that returns nothing is void, not undefined
) => BrowserInstruments | undefined | void;

type Started = { unit: string; hooks: BrowserInstruments };

let started: Started[] = [];
let lastRoute: string | undefined;

function guarded(unit: string, what: string, call: () => void): void {
  try {
    call();
  } catch (error) {
    console.error(`extension ${unit}: instrument ${what} threw`, error);
  }
}

export function startInstruments(
  registry: Readonly<Record<string, BrowserInstrument>>,
  version: string,
): void {
  started = [];
  for (const [unit, instrument] of Object.entries(registry)) {
    guarded(unit, "start", () => {
      const hooks = instrument({
        version,
        edge: `/v1/public/instrument/${unit}/`,
      });
      if (hooks) started.push({ unit, hooks });
    });
  }
}

export function reportError(
  error: unknown,
  info: { componentStack?: string; caught: boolean },
): void {
  for (const { unit, hooks } of started) {
    if (hooks.onError) {
      guarded(unit, "onError", () => hooks.onError?.(error, info));
    }
  }
}

export function notifyNavigate(route: string): void {
  if (route === lastRoute) return;
  lastRoute = route;
  for (const { unit, hooks } of started) {
    if (hooks.onNavigate) {
      guarded(unit, "onNavigate", () => hooks.onNavigate?.(route));
    }
  }
}

/** Tests only. */
export function resetInstruments(): void {
  started = [];
  lastRoute = undefined;
}
