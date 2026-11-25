// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { useCoreEngine } from "./margince-core-engine";
import "./margince-core.css";

/**
 * The Margince Core: the product's one AI identity, a glass ball the shader
 * draws and the engine moves. A caller passes `state` and never restyles it.
 *
 * The state list is closed: idle, ingest and working are the agent's work.
 * `warning` is a stop a human must look at, and `error` a broken run.
 * An open list is a status channel nobody can test. It is `aria-hidden`: the
 * surface around it states every state in text and owns the click.
 *
 * A host without WebGL2 gets a static dress in the state's colour, with the
 * same `data-core-state`, so nothing reading the DOM can tell the difference.
 */
export type MarginceCoreState =
  | "idle"
  | "ingest"
  | "working"
  | "warning"
  | "error";

/**
 * Which way round the ball is dressed.
 *
 * On a dark surface the Core is emissive: it adds its own light and the ribbons
 * are the object. On paper it goes opaque and dark, with the ribbons glowing
 * inside it, because an emissive ball on white is a smudge.
 *
 * `auto` follows the app theme, right for anything on the page. The ink field
 * (a toast, the capture chip) is dark in both themes, so a Core there is `dark`.
 */
export type CoreSurface = "auto" | "dark";

/**
 * Reads the theme the shell has already resolved onto the root element.
 *
 * DARK unless the root says `light`, which is the same reading the Core's own
 * stylesheet makes (`:root:not([data-theme="light"]) .core`). A host that mounts
 * the Core before stamping the attribute would otherwise get the paper dress
 * from the shader and the dark dress from the CSS, on the same ball.
 */
function isDarkRoot(): boolean {
  return document.documentElement.dataset.theme !== "light";
}

function useDarkPage(): boolean {
  const [dark, setDark] = useState(isDarkRoot);
  useEffect(() => {
    const root = document.documentElement;
    // The attribute is the shell's own output (lib/theme.ts), and the Core's
    // stylesheet keys its own dark values off the same one. Observing it rather
    // than subscribing to the store keeps the design system from importing the
    // app it dresses.
    const observer = new MutationObserver(() => {
      setDark(isDarkRoot());
    });
    observer.observe(root, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });
    return () => observer.disconnect();
  }, []);
  return dark;
}

export function MarginceCoreScene({
  state = "idle",
  progress,
  size = "hero",
  feed = false,
  surface = "auto",
  className = "",
}: Readonly<{
  state?: MarginceCoreState;
  /** 0..1. Draws the ring; omit it and no ring renders (WDS-CORE-2). */
  progress?: number;
  size?: "hero" | "md";
  /**
   * Context arriving at the Core, which the shader draws as the intake
   * whirlpool. OFF by default, because a caller that has not said material is
   * arriving is a caller with nothing arriving: an orb that runs intake while it
   * waits for somebody to sign in is claiming work no installation is doing.
   */
  feed?: boolean;
  /** What the Core is sitting on. See `CoreSurface`. */
  surface?: CoreSurface;
  className?: string;
}>) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const darkPage = useDarkPage();
  const paper = surface === "auto" && !darkPage;
  const live = useCoreEngine(canvas, state, {
    paper,
    feed,
    // The hero is the Core as the subject of a page rather than as a light in the
    // chrome, and it wears the same state with more life in it.
    hero: size === "hero",
    // `paper` alone misses a theme flip on a `surface="dark"` Core, since that
    // one holds paper constant: this is what lets the engine re-read the
    // palette when the tokens it draws from could have changed.
    dark: darkPage,
  });

  const classes = [
    "core",
    size === "md" ? "core-md" : "",
    live ? "core-live" : "core-still",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={classes} data-core-state={state} aria-hidden="true">
      <span className="core-glow" />
      <div className="core-shell">
        <canvas className="core-canvas" ref={canvas} />
        {/* The static dress, under the canvas and covered by it while the
            engine is live: shell, rim and glass, in the state's own colour. It
            is always in the markup rather than swapped in on failure, because a
            context can be lost between two frames and a hole where the Core was
            is worse than a still one. */}
        <div className="core-rim" />
        <div className="core-glass" />
      </div>
      {progress === undefined ? null : (
        <svg className="core-progress" viewBox="0 0 100 100" aria-hidden="true">
          <circle cx="50" cy="50" r="48.5" pathLength="100" />
          <circle
            className="core-progress-value"
            cx="50"
            cy="50"
            r="48.5"
            pathLength="100"
            strokeDasharray={`${Math.max(0, Math.min(1, progress)) * 100} 100`}
          />
        </svg>
      )}
    </div>
  );
}
