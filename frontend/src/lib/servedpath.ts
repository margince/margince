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
  const base = import.meta.env.BASE_URL || "/";
  const { pathname } = globalThis.location;
  // A record, booking or consent address is served: the web tier answers it
  // with the shell, and recordaddress writes or restores it.
  return (
    !namesAnAppAddress(pathname) &&
    pathname !== base &&
    `${pathname}/` !== base &&
    pathname !== `${base}index.html`
  );
}
