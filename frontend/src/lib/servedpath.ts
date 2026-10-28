/**
 * Whether the address bar names a PATH this build does not serve.
 *
 * The web tier answers any path with the shell, and the router reads only the
 * fragment, so a mistyped `/settings` opened home and left the address bar
 * reading `/settings`. Later navigation then stacked onto it as
 * `/settings#/settings`.
 *
 * Measured against the build's own base rather than against `"/"`, because the
 * app is served under one in Storybook and in the built preview. Comparing
 * with a literal would call every story an unknown page, which is the burn
 * `takeFromHash` already records above.
 */
export function addressedAPathWeDoNotServe(): boolean {
  const base = import.meta.env.BASE_URL || "/";
  const { pathname } = globalThis.location;
  return (
    pathname !== base &&
    `${pathname}/` !== base &&
    pathname !== `${base}index.html`
  );
}
