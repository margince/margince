import { expect, type Page, test } from "@playwright/test";
import { de } from "../src/i18n/de";

/**
 * The two pages a recipient reaches from a message, driven as a person
 * reaches them: through the address in the mail, in a browser.
 *
 * These are the surfaces that were broken. The visible "Unsubscribe" link
 * pointed at the RFC 8058 machine endpoint, which is POST-only, so a
 * click got 405; "Manage preferences" pointed at the JSON read. Both are
 * pages now, and the rules below are the ones that make them safe: a GET
 * must never withdraw, and a replay must not claim to have changed
 * something.
 *
 * The runner is pinned to de-DE, so the copy asserted here comes from the
 * German catalog rather than being typed twice: a spec holding its own
 * copy of a sentence goes green while the page says something else.
 *
 * The API is mocked rather than seeded. A preference token can only be
 * minted by SENDING a message, which needs a mail connector this lane has
 * no business standing up — and the assertions here are about what the
 * page does with an answer, not about how the answer was produced.
 */

const TOKEN = "pref_e2e_token";
const PURPOSE = "business_correspondence";

type Sent = { method: string; url: string };

const CENTER = {
  masked_email: "m•••••@example.com",
  workspace_name: "Demo Workspace",
  refused: [],
  purposes: [
    {
      key: "transactional",
      label: "Deal & service messages",
      state: "unknown",
      locked: true,
      grant_needs_confirmation: false,
      choice: "no_objection",
      can_opt_in: false,
    },
    {
      key: PURPOSE,
      label: "Direct correspondence",
      state: "unknown",
      locked: false,
      grant_needs_confirmation: false,
      choice: "no_objection",
      can_opt_in: true,
    },
  ],
};

/**
 * Stand in for the public edge, and record every request so a test can
 * assert what actually went out — on this surface the wire IS the
 * contract.
 *
 * `stopped` flips after the first successful POST so a replay answers
 * with the empty array the real endpoint returns, which is what the page
 * reads as "already unsubscribed".
 */
async function mockPublicEdge(page: Page, sent: Sent[]) {
  let stopped = false;
  await page.route("**/v1/public/preferences/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    sent.push({ method: request.method(), url: url.pathname + url.search });

    if (url.pathname.endsWith(`/${TOKEN}`)) {
      await route.fulfill({ json: CENTER });
      return;
    }
    if (url.pathname.endsWith("/unsubscribe")) {
      const unsubscribed = stopped ? [] : [PURPOSE];
      stopped = true;
      await route.fulfill({ json: { unsubscribed } });
      return;
    }
    await route.fulfill({
      status: 404,
      json: { title: "Not Found", status: 404, code: "not_found" },
    });
  });
}

test.describe("the pages a message links to", () => {
  // The rule the unsubscribe page exists for. Mail scanners and link
  // prefetchers follow links in a mailbox with nobody present, so opening
  // one must never withdraw anything.
  test("opening the unsubscribe link writes nothing", async ({ page }) => {
    const sent: Sent[] = [];
    await mockPublicEdge(page, sent);

    await page.goto(`/#/unsubscribe/${TOKEN}/${PURPOSE}`);
    await expect(
      page.getByRole("button", { name: de["prefs.unsub.confirm"] }),
    ).toBeVisible();

    expect(sent.filter((r) => r.method !== "GET")).toEqual([]);
  });

  test("one explicit press stops the purpose the link named", async ({
    page,
  }) => {
    const sent: Sent[] = [];
    await mockPublicEdge(page, sent);

    await page.goto(`/#/unsubscribe/${TOKEN}/${PURPOSE}`);
    await page.getByRole("button", { name: de["prefs.unsub.confirm"] }).click();

    await expect(
      page.getByRole("heading", { name: de["prefs.unsub.doneTitle"] }),
    ).toBeVisible();
    const post = sent.find((r) => r.method === "POST");
    expect(post?.url).toContain(`purpose=${PURPOSE}`);
  });

  // The endpoint reports what it CHANGED, so a second press answers with
  // an empty array — and the page must say "already off" rather than show
  // a fresh confirmation for a no-op.
  test("a second press reads as already unsubscribed", async ({ page }) => {
    const sent: Sent[] = [];
    await mockPublicEdge(page, sent);

    await page.goto(`/#/unsubscribe/${TOKEN}/${PURPOSE}`);
    await page.getByRole("button", { name: de["prefs.unsub.confirm"] }).click();
    await expect(
      page.getByRole("heading", { name: de["prefs.unsub.doneTitle"] }),
    ).toBeVisible();

    // RELOAD rather than navigate: the address has not changed, so a goto
    // to the same hash leaves the confirmed view standing — which is what
    // a reader pressing the link in the mail a second time actually does.
    await page.reload();
    await page.getByRole("button", { name: de["prefs.unsub.confirm"] }).click();
    await expect(
      page.getByRole("heading", { name: de["prefs.unsub.alreadyOff"] }),
    ).toBeVisible();
  });

  test("a dead link says so once, and offers no button", async ({ page }) => {
    await page.route("**/v1/public/preferences/**", (route) =>
      route.fulfill({
        status: 404,
        json: { title: "Not Found", status: 404, code: "not_found" },
      }),
    );

    await page.goto(`/#/unsubscribe/pref_gone/${PURPOSE}`);
    await expect(page.getByText(de["prefs.invalidLink"])).toBeVisible();
    await expect(
      page.getByRole("button", { name: de["prefs.unsub.confirm"] }),
    ).toHaveCount(0);
  });

  // A locked purpose carries no control at all: one that always failed
  // would be worse than none.
  test("a purpose that cannot be switched off offers no control", async ({
    page,
  }) => {
    await mockPublicEdge(page, []);
    await page.goto(`/#/unsubscribe/${TOKEN}/transactional`);

    await expect(page.getByText(de["prefs.unsub.lockedTitle"])).toBeVisible();
    await expect(
      page.getByRole("button", { name: de["prefs.unsub.confirm"] }),
    ).toHaveCount(0);
  });

  test("the preference centre opens on the same token and stages its changes", async ({
    page,
  }) => {
    const sent: Sent[] = [];
    await mockPublicEdge(page, sent);

    await page.goto(`/#/preferences/${TOKEN}`);
    const boxes = page.getByRole("checkbox");
    await expect(boxes.first()).toBeVisible();

    // Staged, not written: the whole point of the save bar.
    await boxes.last().click();
    expect(sent.filter((r) => r.method === "PUT")).toEqual([]);
  });

  // 320px is the narrowest screen worth supporting, and this page is read
  // on a phone more often than not.
  test("both pages fit a 320px screen", async ({ page }) => {
    await mockPublicEdge(page, []);
    await page.setViewportSize({ width: 320, height: 640 });

    for (const address of [
      `/#/unsubscribe/${TOKEN}/${PURPOSE}`,
      `/#/preferences/${TOKEN}`,
    ]) {
      await page.goto(address);
      await expect(page.locator("h1").first()).toBeVisible();
      const overflows = await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth + 1,
      );
      expect(overflows, `${address} scrolls sideways at 320px`).toBe(false);
    }
  });

  // Somebody who cannot use a mouse must still be able to stop the email.
  test("the unsubscribe press is reachable by keyboard alone", async ({
    page,
  }) => {
    const sent: Sent[] = [];
    await mockPublicEdge(page, sent);
    await page.goto(`/#/unsubscribe/${TOKEN}/${PURPOSE}`);

    const button = page.getByRole("button", {
      name: de["prefs.unsub.confirm"],
    });
    await expect(button).toBeVisible();
    await button.focus();
    await page.keyboard.press("Enter");

    await expect(
      page.getByRole("heading", { name: de["prefs.unsub.doneTitle"] }),
    ).toBeVisible();
    expect(sent.some((r) => r.method === "POST")).toBe(true);
  });
});
