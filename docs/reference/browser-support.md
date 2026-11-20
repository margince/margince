# Browser support

Margince supports the browsers that [`frontend/.browserslistrc`](../../frontend/.browserslistrc)
resolves to. The file is the one list: the production build, its test and this page all read it.
Print the versions it means today from `frontend/`:

```sh
pnpm exec browserslist
```

## Who the list is for

| Rank | Browsers | Query in the file |
|---|---|---|
| Primary | Chrome, Chrome on Android, Edge, Samsung Internet | `last 2 … versions` |
| Secondary | Safari on Mac, Safari on iPhone and iPad | `last 2 … major versions` |
| Secondary | Firefox on computers | `last 2 Firefox versions`, `Firefox ESR` |
| Any | A version above 1% of use in Germany, Austria, Switzerland or Vietnam | `> 1% in DE`, `AT`, `CH`, `VN` |

Safari counts major releases, because an iPhone or Mac keeps the system release its hardware can run.
Firefox keeps its extended support release, which firms and public offices in the German-speaking
market run and which updates once a year.

The usage line uses the caniuse figures for each country, so a browser that is common in Vietnam but
rare in Germany still counts. Browsers built on Chromium that caniuse does not list, such as Cốc Cốc,
run a recent Chrome engine and follow the Chrome floor.

The file shuts out Internet Explorer, the old Edge, Opera Mini in its proxy mode, KaiOS phones and
Firefox on Android, and every browser that has had no update for two years (`not dead`). It also
shuts out UC Browser and QQ Browser. Both pass the usage line in Vietnam, but their engine is years
behind Chrome and lacks `:has()`, `@container` and `dvh`, which the app already uses.

## How the list reaches the build

[`frontend/scripts/browser-targets.ts`](../../frontend/scripts/browser-targets.ts) turns the list into
the oldest version of each engine, and `vite.config.ts` passes that as `build.target`. Vite lowers
JavaScript syntax to it and hands the same floor to Lightning CSS for the CSS. Samsung Internet,
Android WebView and Opera Mobile have no Vite target name. Each counts toward the Chrome floor with
the Chromium its version runs, which `baseline-browser-mapping` knows. Samsung Internet runs an older
Chromium than Chrome does, so it often sets that floor. Any other browser the list admits without a
Vite target name fails the build.

The TypeScript `lib` in `tsconfig.app.json` names the JavaScript built-ins every browser on the list
has (`ES2024`). A newer built-in needs the check below before code uses it.

[`frontend/scripts/browser-targets.test.ts`](../../frontend/scripts/browser-targets.test.ts) fails when
the list loses a primary or secondary family, admits a browser it shuts out or a dead one, drops the
Firefox extended support release, or when `build.target` stops being the list's floor. The build
fails when a Chromium build on the list is older than the Chromium data. A version newer than that
data counts with the newest Chromium the data knows.

## Using a web feature

- A feature that every browser on the list has needs no fallback. Check it against the printed
  versions on caniuse or in the MDN Baseline table.
- A feature that only some of them have goes behind `@supports` in CSS or a feature test in script.
  A browser without it still gets a working page. For example,
  [`recordtabs.css`](../../frontend/src/design-system/recordtabs.css) anchors the tab underline in CSS
  where the browser can, and keeps a fallback where it cannot.
- No test checks which CSS or DOM features the code uses. The end-to-end lane runs Chromium only, so
  Safari and Firefox are checked by hand.

## Refreshing the browser data

The versions come from the `caniuse-lite` and `baseline-browser-mapping` packages in
`pnpm-lock.yaml`. Browserslist warns once the data is six months old. Refresh it from the repository
root, then run the test above:

```sh
pnpm update -r caniuse-lite baseline-browser-mapping
```
