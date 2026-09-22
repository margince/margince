import { expect, type Locator, type Page } from "@playwright/test";

// Playwright's LIST readers do not auto-wait. `allTextContents()`,
// `allInnerTexts()`, `all()` and `count()` resolve against whatever matches at
// that instant and answer an empty result for zero matches rather than
// retrying — so a read placed right after a container's visibility assert is
// really asking "has the frame painted?", and reports the body's absence as
// the body's contents being wrong.
//
// That is expensive to read from a CI log. The failure that prompted this
// module named a specific German heading as missing, so it looked like a
// content or locale regression; the drawer had simply not filled in yet.
//
// These helpers wait for the CONTENT the caller is about to read. Where a spec
// knows the exact number it expects, `expect(locator).toHaveCount(n)` is
// better still — it waits and asserts in one step, and states the number.

/** The texts of every match, read once at least one of them has rendered. */
export async function textsOf(locator: Locator): Promise<string[]> {
  await expect(locator.first()).toBeVisible();
  return locator.allTextContents();
}

/** A handle per match, taken once at least one of them has rendered. */
export async function itemsOf(locator: Locator): Promise<Locator[]> {
  await expect(locator.first()).toBeVisible();
  return locator.all();
}

/**
 * Sign in as the dev bootstrap admin, unless the session is already up.
 *
 * A dev stack that has been logged into already redirects /#/login straight to
 * the app, so the form is ABSENT on the happy path — waiting for a navigation
 * that never happens is a 30s timeout rather than a login failure. The form is
 * filled only when it is actually rendered.
 *
 * Which of the two the page is showing cannot be read from a bare count,
 * though: an unrendered form matches zero email inputs, exactly as a signed-in
 * session does. Counting reads a slow paint as "already signed in", skips the
 * sign-in, and the failure surfaces much later as whichever assertion first
 * needed a session. Waiting for the page to resolve into ONE of its two states
 * removes the race without deciding in advance which state that is.
 */
export async function signIn(page: Page): Promise<void> {
  await page.goto("/#/login", { waitUntil: "networkidle" });
  const email = page
    .locator('input[type="email"], input[name="email"]')
    .first();
  // The nav is what "signed in" looks like, and it is what every assertion in
  // the specs depends on — anchoring here rather than on a URL change means
  // the wait describes the state the tests need.
  const rail = page.locator("nav.rail").first();
  await expect(email.or(rail)).toBeVisible();
  if (await rail.isVisible()) {
    return;
  }
  await email.fill(process.env.E2E_EMAIL ?? "admin@demo.test");
  await page
    .locator('input[type="password"], input[name="password"]')
    .first()
    .fill(process.env.E2E_PASSWORD ?? "demo-password-123");
  await page.locator('button[type="submit"]').first().click();
  await expect(rail).toBeVisible();
}

// Below the waits, and not one: the MEASURE two specs share. It lives here
// rather than in either of them because a second copy of it would be a second
// answer to whether the page pans sideways.

/**
 * How far the PAGE scrolls sideways, and which scroller does it.
 *
 * NOT `document.body.scrollWidth`, which is what this sweep used to read. The
 * shell pins `.main` to `overflow: hidden` (shell.css) and gives `.scroll` an
 * `overflow-y: auto` that computes `overflow-x` to `auto` with it, so wide
 * content scrolls INSIDE the page's own scroller and the body never grows a
 * pixel. Measured across all twelve settings tabs the body reported 0 while
 * `.scroll` itself overflowed by up to 273px — the assertion was structurally
 * incapable of failing, whatever the layout did.
 *
 * So this reads the two elements that actually scroll the page: the document,
 * and the shell's content column. Anything a screen spills — a header row, a
 * card, a table — spills into one of them, and it is the reader panning THOSE
 * that §3.8 forbids.
 *
 * A component that declares a horizontal scroll region of its own is not this
 * and is deliberately not measured: `.table-scroll` around a table too wide for
 * a phone (atoms.css) is the sanctioned answer to wide content, and it is
 * bounded by its card, so the page around it never moves.
 */
export async function pageOverflow(page: Page): Promise<string[]> {
  return page.evaluate(() => {
    const scrollers: { name: string; element: Element }[] = [
      { name: "the document", element: document.documentElement },
      ...Array.from(document.querySelectorAll(".scroll")).map((element) => ({
        name: "the shell content scroller (.scroll)",
        element,
      })),
    ];
    return scrollers
      .map(({ name, element }) => ({
        name,
        overflow: element.scrollWidth - element.clientWidth,
      }))
      .filter(({ overflow }) => overflow > 0)
      .map(({ name, overflow }) => `${name}: ${overflow}px past the viewport`);
  });
}
