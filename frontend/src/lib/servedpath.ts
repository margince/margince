import { namesAnAppAddress } from "./recordaddress";

/**
 * Whether the address bar names a PATH this build does not serve.
 *
 * The web tier answers any path with the shell, and the router reads only the
 * fragment. So a mistyped `/settings` opened home and stayed in the bar.
 *
 * Measured against the build's own base, since Storybook and the preview serve
 * the app under one. A literal would call every story unknown.
 */
export function addressedAPathWeDoNotServe(): boolean {
  const { href, pathname } = globalThis.location;
  // Resolved, since a static Storybook build is relative (`./`).
  const base = new URL(import.meta.env.BASE_URL || "/", href).pathname;
  // A record, booking or consent address is served: the web tier answers it
  // with the shell, and recordaddress writes or restores it.
  return (
    !namesAnAppAddress(pathname) &&
    pathname !== base &&
    `${pathname}/` !== base &&
    pathname !== `${base}index.html` &&
    !(isStorybook() && pathname === `${base}iframe.html`)
  );
}

// Storybook draws every story in its own frame document. Outside it no such
// document exists, and the web tier answers that path like any other.
const isStorybook = (): boolean => import.meta.env.STORYBOOK === "true";
