// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { meFixture } from "../src/testing/mefixture";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import { copy } from "./copy";
import { seededViews } from "./mockapi/fixtures";
import { expectShellRendered } from "./screens";
import { mockApi, servePrincipal } from "./seed";
import { pageOverflow, textsOf } from "./waits";

test.beforeAll(async () => {
  await loadLocale("de");
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("AC-inbox: the staged decision is in the Decisions view", async ({
  page,
}) => {
  await page.goto("/#/home?queue=1&queue_view=decisions");
  await expect(
    page.getByText(/Send the follow-up to Anna Weber/).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Entscheiden" }).first(),
  ).toBeVisible();
});

test("AC-book: the reusable booking link is available for sharing and signatures", async ({
  page,
}) => {
  await page.goto("/#/book");
  await expect(page.locator("nav.rail")).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: de["scheduling.myLink"] }),
  ).toHaveValue("https://crm.example.test/#/book/host-1");
  await expect(
    page.getByRole("button", { name: de["scheduling.copyLink"] }),
  ).toBeEnabled();
});

test("AC-automations-1: create from the catalog arrives paused; enable is the deliberate second step", async ({
  page,
}) => {
  // Scoped to the editor's own region rather than to the page. The criterion
  // then still reads what it names if the page gains a second card.
  await page.goto("/#/settings/automations/library");
  const automations = page.locator("[data-automations-admin]");
  await expect(automations.getByText("Stillstands-Erinnerung")).toBeVisible();
  await automations
    .getByRole("button", { name: "Vorlage verwenden" })
    .first()
    .click();
  // Name and parameters are one form behind the library's verb. They are
  // asserted in the dialog it opens, not in the region it opened from.
  const form = page.getByRole("dialog");
  // the schema default arrives in the one parameter field
  await expect(
    form.getByRole("spinbutton", { name: de["auto.param.dueInDays"] }),
  ).toHaveValue("3");
  await form.getByRole("button", { name: "Erstellen" }).click();
  // The outcome lands on the CARD: by the time it is true the dialog that
  // produced it is gone. The new row is on the other tab, so it says where.
  await expect(
    page.getByText(
      "Pausiert erstellt. Schalte sie unter „Deine Automatisierungen“ ein.",
    ),
  ).toBeVisible();
  await page.getByRole("button", { name: "Deine Automatisierungen" }).click();
  const row = page.getByTestId("automation-au-2");
  // The row states its status on the control that changes it, rather than on a
  // badge beside a button whose label named the other state. So the criterion
  // reads the switch: it arrives off, and one flip turns it on.
  const active = row.getByRole("switch", { name: "Aktiv" });
  await expect(active).toHaveAttribute("aria-checked", "false");
  await active.click();
  await expect(active).toHaveAttribute("aria-checked", "true");
});

test("AC-automations-2: anti-DSL — no free-form rule body, no user-defined trigger", async ({
  page,
}) => {
  // The anti-DSL claim is about the automations surface, so it is counted over
  // that surface's region. An input elsewhere on the page has nothing to do
  // with rule authoring.
  await page.goto("/#/settings/automations/library");
  const automations = page.locator("[data-automations-admin]");
  await expect(automations.getByText("Stillstands-Erinnerung")).toBeVisible();
  await automations
    .getByRole("button", { name: "Vorlage verwenden" })
    .first()
    .click();
  // The authoring form is the dialog the verb opens. The claim is about what a
  // reader can write, so it is counted where the inputs are. It is also counted
  // in the region, so a rule body beside the library would still fail this.
  const form = page.getByRole("dialog");
  await expect(form.locator("textarea")).toHaveCount(0);
  await expect(automations.locator("textarea")).toHaveCount(0);
  // the instance name plus the schema-derived parameter, and nothing else
  await expect(form.getByRole("textbox")).toHaveCount(1);
  await expect(form.getByRole("spinbutton")).toHaveCount(1);
});

test("AC-settings-16: the audit log renders attributed entries, filters live, and loads more", async ({
  page,
}) => {
  // The audit log is the trail that proves the consent, retention and DSR
  // surfaces were honoured. It has its own page because it answers to
  // `audit_log`, where those answer to the consent registry and retention policy.
  // It names the contact behind each entry (AuditEntryLine, PD-002). The
  // signed-in human reads "Du". A machine acting under someone's authority
  // reads as that contact, with the tool as a qualifier. An agent's own id is
  // never the label. Attribution exists so a human can be asked about a change.
  await page.goto("/#/settings/audit");
  // The settings sidebar's own group heading also reads "Du", so the actor
  // label is read inside the trail's table. The count-0 below then proves the
  // filter dropped the ROW, not the sidebar.
  const trail = page.getByRole("table", {
    name: de["settings.auditTrailLabel"],
  });
  await expect(trail.getByText("Du", { exact: true })).toBeVisible();
  await expect(page.getByText("Marcus Brandt", { exact: true })).toBeVisible();
  await expect(page.getByText("über einen Agenten")).toBeVisible();
  // The agent's own identifier is not shown at all when a human stands behind it.
  await expect(page.getByText("agent:runner", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Mehr laden" }).click();
  // A bare connector presented no grant, so there is no human to name and no
  // gap to claim. It shows what acted.
  await expect(
    page.getByText("connector:gmail", { exact: true }),
  ).toBeVisible();
  // The dials stand open on a bar above the trail, each applying on its own.
  // The actor dial offers members by the names the column shows.
  await page.getByRole("combobox", { name: de["settings.auditActor"] }).click();
  await page
    .getByRole("listbox")
    .getByRole("option", { name: "Lena Fischer" })
    .click();
  // The reader's own row stays, and both rows of others go. The reader's row
  // was on screen before the pick, so its presence alone proves nothing.
  await expect(trail.getByText("Du", { exact: true })).toBeVisible();
  await expect(page.getByText("Marcus Brandt", { exact: true })).toHaveCount(0);
  await expect(page.getByText("connector:gmail", { exact: true })).toHaveCount(
    0,
  );
  await expect(page).toHaveURL(/actor=human%3Au1/);
});

test("AC-settings: the passport list is metadata-only and strikes revoked rows", async ({
  page,
}) => {
  // Passports are a credential the COLLEAGUE holds, so they live on AI assistants,
  // behind "For developers": a reader connecting an AI tool never meets one.
  await page.goto("/#/settings/agents");
  await expect(page.getByText("Marcus' Claude", { exact: true })).toBeHidden();
  await page.getByText(de["aiAccess.dev.title"], { exact: true }).click();
  await expect(page.getByText("Marcus' Claude", { exact: true })).toBeVisible();
  // A revoked passport can do nothing more, so it waits behind a toggle.
  await expect(page.getByText("Alter Runner")).toHaveCount(0);
  await page
    .getByRole("button", {
      name: de["settings.passportsShowRevoked_one"].replace("{count}", "1"),
    })
    .click();
  const revoked = page.locator('[data-passport="pp-2"]');
  await expect(revoked.getByText("widerrufen")).toBeVisible();
  // Struck on the name rather than across the whole row. The row also carries
  // the dates and the standing, and a line through those makes them hard to read.
  await expect(revoked.getByText("Alter Runner")).toHaveCSS(
    "text-decoration-line",
    "line-through",
  );
  // no token is ever re-disclosed on this surface
  await expect(page.getByText(/mgp_/)).toHaveCount(0);
});

// The public page opens on the current month, so these pin the clock to the
// month the fixture's free times fall in.
test("AC-book-public: consent gates calendar invitation and its wording passes through verbatim", async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date("2026-07-01T06:00:00Z"));
  await page.goto("/#/book/host-1");
  await expect(page.locator("nav.rail")).toHaveCount(0);
  await page.locator(".bookguest-times .meeting-slots button").first().click();
  const submit = page.getByRole("button", {
    name: copy(de["scheduling.confirmAt"]),
  });
  await expect(submit).toBeDisabled();
  await page
    .getByRole("textbox", { name: de["book.name"], exact: true })
    .fill("Jonas Beispiel");
  await page
    .getByRole("textbox", { name: de["book.email"] })
    .fill("jonas@beispiel.example");
  await expect(submit).toBeDisabled();
  const consent = page.getByRole("checkbox", {
    name: de["book.consentWording"],
  });
  await consent.check();
  await expect(submit).toBeEnabled();
  const shownWording = await page
    .getByText(de["book.consentWording"], { exact: true })
    .textContent();
  const requestPromise = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().includes("/public/booking/host-1"),
  );
  await submit.click();
  const request = await requestPromise;
  const body = request.postDataJSON();
  expect(body.consent.wording).toBe(shownWording);
  expect(body.consent.policy_version).toBeTruthy();
  expect(body.consent.purpose_id).toBeUndefined();
  expect(request.headers()["idempotency-key"]).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page).toHaveURL(/#\/book\/manage-guest-booking$/);
  await expect(page.getByRole("status")).toContainText(
    de["scheduling.pending"],
  );
  await expect(page.getByText(de["scheduling.confirmed"])).toHaveCount(0);
});

test("AC-book-public-409: a taken slot degrades honestly — no fabricated confirmation", async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date("2026-07-01T06:00:00Z"));
  await page.goto("/#/book/host-1");
  await page.getByRole("button", { name: /^12:00$/ }).click();
  await page
    .getByRole("textbox", { name: de["book.name"], exact: true })
    .fill("Jonas Beispiel");
  await page
    .getByRole("textbox", { name: de["book.email"] })
    .fill("jonas@beispiel.example");
  await page.getByRole("checkbox").check();
  await page
    .getByRole("button", {
      name: copy(de["scheduling.confirmAt"], { time: "12:00" }),
    })
    .click();
  await expect(page.getByText(de["common.duplicate.slotTaken"])).toBeVisible();
  await expect(page.getByText(de["scheduling.confirmed"])).toHaveCount(0);
});

test("AC-onboarding-1: onboarding is the rail-less conversational shell", async ({
  page,
}) => {
  // Onboarding is the conversational shell: a focused, rail-less flow whose
  // journey is a conversation thread, not a stepper.
  //
  // The flow opens on the gate: one question, centred, before any thread
  // exists. So the AC asserts rail-less and no stepper against that first
  // surface, and that the single question is the whole ask.
  //
  // That first screen belongs to an installation nobody has described yet.
  // Under the global seed the journey is finished, and a finished reader on
  // this route is sent to the Brief. So this AC layers the unstarted journey on
  // top (the most recently registered route answers first).
  await mockApi(page, { journey: "unstarted" });
  await page.goto("/#/onboarding");
  await expect(page.locator("nav.rail")).toHaveCount(0);
  await expect(page.locator(".stepper")).toHaveCount(0);
  await expect(page.getByLabel("Website-Adresse")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Website analysieren" }),
  ).toBeVisible();
  // The thread belongs to the working view, not the gate.
  await expect(
    page.getByRole("log", { name: "Einrichtungsgespräch" }),
  ).toHaveCount(0);
});

test("AC-create-1: a contact is created from the list and lands on its 360", async ({
  page,
}) => {
  await page.goto("/#/contacts");
  await page.getByRole("button", { name: "Neuer Kontakt" }).click();
  await page.getByLabel("Vollständiger Name").fill("Peter Neu");
  // Email is now a repeatable row group (P-15): add a row, then fill it.
  await page.getByRole("button", { name: "E-Mail hinzufügen" }).click();
  await page.getByLabel("E-Mail *").fill("peter@neu.example");
  await page.getByRole("button", { name: "Erstellen" }).click();
  await expect(page).toHaveURL(/#\/contacts\/p-new$/);
});

test("AC-create-2: the palette's New-deal action opens the create form; only open stages offered", async ({
  page,
}) => {
  await page.goto("/#/deals/new");
  // Scope to the create dialog. The deals list also renders a stage filter
  // select, over all stages, whose name contains "Phase". A page-wide
  // getByLabel would match both. The create form's stage select lives inside
  // the modal and offers open stages only.
  const stageSelect = page.getByRole("dialog").getByLabel("Phase");
  await expect(stageSelect).toBeVisible();
  // The choices exist only while the popup is open, and the popup is portalled
  // to the body. So it is located from the page, not from inside the dialog.
  await stageSelect.click();
  const stageNames = await textsOf(
    page.locator('[role="listbox"]').getByRole("option"),
  );
  expect(stageNames.filter(Boolean)).toEqual([
    "Qualify",
    "Proposal",
    "Negotiation",
  ]);
  // The list is put away the way it was opened, with a second press on the
  // trigger. Not Escape: the surrounding Modal listens for it too and would
  // close the whole create form.
  await stageSelect.click();
  await expect(page.locator('[role="listbox"]')).toHaveCount(0);
  await page.getByLabel("Deal-Name").fill("Neuer Deal");
  // By role: getByLabel("Wert") matches any label containing the word.
  await page.getByRole("textbox", { name: "Wert" }).fill("480");
  await page.getByRole("button", { name: "Erstellen" }).click();
  await expect(page).toHaveURL(/#\/deals\/d-new$/);
});

test("AC-create-3: a company created with a domain offers the contacts on it, none ticked", async ({
  page,
}) => {
  // A reader who may link contacts to a company, which the shared admin is not.
  await servePrincipal(
    page,
    meFixture({
      allow: {
        company: ["read", "create", "update"],
        contact: ["read", "update"],
        relationship: ["read", "create"],
      },
    }),
  );
  await page.goto("/#/companies");
  await page.getByRole("button", { name: de["create.company"] }).click();
  await page
    .getByLabel(`${de["create.displayName"]} *`)
    .fill("Nordlicht Logistik GmbH");
  await page.getByRole("button", { name: de["field.addDomain"] }).click();
  await page
    .getByLabel(`${de["field.domain"]} *`)
    .fill("nordlicht-logistik.example");
  await page.getByRole("button", { name: "Erstellen", exact: true }).click();
  await expect(page).toHaveURL(/#\/companies\/o-new$/);

  const dialog = page.getByRole("dialog", {
    name: de["co.domainContacts.dialogTitle"],
  });
  const ticks = dialog.getByRole("checkbox");
  await expect(ticks).toHaveCount(2);
  await expect(dialog.getByRole("checkbox", { checked: true })).toHaveCount(0);
  await expect(
    dialog.getByRole("button", { name: de["co.domainContacts.linkNone"] }),
  ).toBeDisabled();

  await dialog.getByRole("checkbox", { name: /Anja Petersen/ }).check();
  const sent = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().endsWith("/companies/o-new/link-contacts"),
  );
  await dialog
    .getByRole("button", { name: copy(de["co.domainContacts.link_one"]) })
    .click();
  expect((await sent).postDataJSON()).toEqual({ contact_ids: ["p-anja"] });
  await expect(dialog).toBeHidden();
});

// AC-filters-and-views: authoring a dynamic list's filter in the product.
//
// `filters` in CORE_SCREENS (screens.ts) puts the screen in the 390px and axe
// passes. These tests say what the screen is for. A criterion whose text is not
// in this tree gets no test here, because the test would assert a guess.
test.describe("filters and views", () => {
  // Every clause below is authored through the picker, the way a reader
  // authors one, rather than by seeding a tree in code. The claim is that a
  // human can build this filter.
  async function authorIndustryIs(page: Page) {
    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await page.getByRole("combobox", { name: "Feld" }).click();
    await page.getByRole("option", { name: "Branche" }).click();
    await page.getByRole("combobox", { name: "Operator" }).click();
    await page.getByRole("option", { name: "ist", exact: true }).click();
    await page.getByRole("textbox", { name: "Wert" }).fill("automotive");
  }

  test("AC-filters-and-views-3: a clause is a field, an operator its type admits, and a value", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);

    // Before anything is authored the page says what would bring a count,
    // rather than showing a zero that would read as "no companies match".
    await expect(
      page.getByText(
        de["filters.hint.start"].replace("{records}", "Unternehmen"),
      ),
    ).toBeVisible();

    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();

    // The field picker is the server's vocabulary, not a list this screen keeps.
    // `industry` and `lifecycle` are company fields. `tag` is the leaf that is
    // an `EXISTS` over a join rather than a column. The screen names each field
    // the server sends in the reader's language, sorted by those words.
    await page.getByRole("combobox", { name: "Feld" }).click();
    expect(await textsOf(page.getByRole("option"))).toEqual([
      "Branche",
      "Lebenszyklus",
      de["filters.field.tag"],
      "Verantwortlich",
      // The custom field sorts after the core ones. A reader scanning for a
      // column they added finds it in one place rather than interleaved.
      "fleet size",
    ]);
    await page.getByRole("option", { name: "Branche" }).click();

    // And the operator set is the FIELD's. `industry` is text, so `enthält` is
    // offered; the tag clause in AC-4 proves the narrowing by its absence.
    await page.getByRole("combobox", { name: "Operator" }).click();
    expect(await textsOf(page.getByRole("option"))).toEqual([
      "ist",
      "ist nicht",
      "ist eines von",
      "enthält",
      "hat einen Wert",
    ]);
    await page.getByRole("option", { name: "ist", exact: true }).click();

    await page.getByRole("textbox", { name: "Wert" }).fill("automotive");

    // The count is the server's: 812 is the fixture's match_count, a number no
    // arithmetic on the two returned rows could produce.
    await expect(page.getByText("Passende Unternehmen: 812")).toBeVisible();
  });

  // Custom fields and tags are selectable beside core fields. A picker that
  // offered only core fields would satisfy every other case here. The badge
  // matters because `cf_fleet_size` and a core column look identical without it.
  test("AC-filters-and-views-2: a custom field is offered beside the core ones, and says it is one", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);
    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await page.getByRole("combobox", { name: "Feld" }).click();
    await page.getByRole("option", { name: "fleet size" }).click();

    // The badge sits beside the picker, not inside the option. A chosen field's
    // option is no longer on screen. The reader needs to know it is a custom
    // column while reading the clause, not while opening the menu.
    await expect(page.getByText("eigenes Feld")).toBeVisible();

    // Its operators are its type's. It is a number, so the comparisons appear
    // and `enthält` does not. A fixed operator set here would produce a 422 the
    // reader could not interpret.
    await page.getByRole("combobox", { name: "Operator" }).click();
    expect(await textsOf(page.getByRole("option"))).toEqual([
      "ist",
      "ist nicht",
      "ist größer als",
      "ist mindestens",
      "ist kleiner als",
      "ist höchstens",
      "ist eines von",
      "hat einen Wert",
    ]);
  });

  test("AC-filters-and-views-4: conditions combine through the word between them, a group nests the other join, and a linked field is narrowed", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);

    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await page.getByRole("button", { name: "Bedingung hinzufügen" }).click();
    await expect(page.getByRole("combobox", { name: "Feld" })).toHaveCount(2);

    // The join is the word between two conditions, and it is the switch for
    // that word alone.
    const matchAll = page.getByRole("button", {
      name: de["filters.connector.and"]
        .replace("{word}", de["filters.join.and"])
        .replace("{other}", de["filters.join.or"]),
      exact: true,
    });
    await expect(matchAll).toHaveCount(1);
    await matchAll.click();
    await expect(
      page.getByRole("button", {
        name: de["filters.connector.or"]
          .replace("{word}", de["filters.join.or"])
          .replace("{other}", de["filters.join.and"]),
        exact: true,
      }),
    ).toHaveCount(1);

    // A tag leaf is reached through a join, so the engine narrows it to the link
    // operators. `enthält` is gone: a picker that offered it would produce a 422
    // the reader could not interpret. A record holds a set of tags, so the
    // operators read as what the set includes.
    await page.getByRole("combobox", { name: "Feld" }).first().click();
    await page
      .getByRole("option", { name: de["filters.field.tag"], exact: true })
      .click();
    await page.getByRole("combobox", { name: "Operator" }).first().click();
    expect(await textsOf(page.getByRole("option"))).toEqual([
      de["filters.op.include"],
      de["filters.op.notInclude"],
      de["filters.op.includeAny"],
      de["filters.op.includeSomeOrNone"],
    ]);
    await page.keyboard.press("Escape");

    // Nesting: "Add a group" adds a group of the other join with one condition
    // in it. That is how a filter mixes "and" with "or".
    await page
      .getByRole("button", { name: de["filters.addGroup"], exact: true })
      .click();
    await expect(
      page.getByText(de["filters.group.all"], { exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("combobox", { name: "Feld" })).toHaveCount(3);
  });

  test("AC-filters-and-views-5: the preview shows matching records, and says it is a page of them", async ({
    page,
  }) => {
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);
    await authorIndustryIs(page);

    await expect(
      page.getByRole("heading", { level: 2, name: "Passende Datensätze" }),
    ).toBeVisible();
    // By cell: the shell's own workspace strip carries the same company name.
    // A bare text match would pass on the chrome while the table was empty.
    await expect(
      page.getByRole("cell", { name: "Brandt Automotive", exact: true }),
    ).toBeVisible();
    // Every row satisfies the filter, and that is asserted rather than assumed.
    // A reader checks a filter against the preview before saving it as a list.
    // So the preview must never show a row the predicate excludes, and a test
    // that names only the expected row would miss one.
    await expect(
      page.getByRole("cell", { name: "Kessler Fahrzeugbau", exact: true }),
    ).toBeVisible();
    // The LABEL the screen renders, not the code the filter was authored with.
    // The industry cell reads through `industryWords`. Naming the code here
    // passes only while that column is untranslated.
    await expect(
      page.getByRole("cell", { name: "Automobilindustrie", exact: true }),
    ).toHaveCount(2);
    // Counting the rows is what holds "every row satisfies the predicate".
    // Naming an absent industry does not hold it on its own. `logistics` is not
    // one of INDUSTRY_CODES, so it renders as the raw code and the name
    // matches. A row carrying a real code renders as its wording, and the name
    // would miss it.
    await expect(page.getByRole("row")).toHaveCount(3);
    await expect(page.getByRole("cell", { name: "logistics" })).toHaveCount(0);
    // The caption says 812 match and two are shown, so the reader knows this
    // is a sample rather than the whole selection.
    await expect(page.getByText(de["filters.resultsCaption"])).toBeVisible();
  });

  // The 390px sweep visits the library. A written condition adds a clause row
  // of three controls, a different width problem, so it is measured here.
  test("at 390px a written condition fits without scrolling sideways, under one heading", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/#/filters/companies");
    await expectShellRendered(page);
    await authorIndustryIs(page);

    await expect(page.getByText("Passende Unternehmen: 812")).toBeVisible();
    expect(await pageOverflow(page)).toEqual([]);
    await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
  });

  test("AC-filters-and-views-1: the object tab is part of the address", async ({
    page,
  }) => {
    await page.goto("/#/filters/deals");
    await expectShellRendered(page);
    const objects = page.getByRole("combobox", { name: "Datensatztyp" });
    await expect(objects).toHaveText("Deals");

    // "Kontakte": `filters.tab.contacts` is a different key from the nav's, and
    // the catalog gives both the same word. The tab says what every other
    // surface says.
    await objects.click();
    await page.getByRole("option", { name: "Kontakte" }).click();
    await expect(page).toHaveURL(/#\/filters\/contacts$/);

    // Reloaded, not just navigated: the tab is where you are, so a shared link
    // and a refresh have to land on the same object.
    await page.reload();
    await expect(objects).toHaveText("Kontakte");
  });

  test("AC-filters-and-views-9: a list's view opens on its list from the library, and a built view's tab opens it in the library", async ({
    page,
  }) => {
    const byName = {
      id: "v-by-name",
      resource: "contacts",
      name: "Nach Name",
      owner_id: "u1",
      shared_scope: "private",
      query: { list: { q: "", sort: "full_name", filters: {} } },
      version: 1,
    };
    await page.route(/\/v1\/views(\?.*)?$/, (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      const resource = new URL(route.request().url()).searchParams.get(
        "resource",
      );
      const views = [...seededViews, byName].filter(
        (view) => resource === null || view.resource === resource,
      );
      return route.fulfill({
        json: { data: views, page: { next_cursor: null, has_more: false } },
      });
    });
    await settleAnimations(page);
    await page.goto("/#/filters");
    const row = page.getByRole("row").filter({ hasText: "Nach Name" });
    await expect(
      row.getByText(de["filters.library.savedOn.contacts"]),
    ).toBeVisible();
    await row.getByRole("link", { name: "Nach Name" }).click();
    await expect(page).toHaveURL(/#\/contacts\?per=25&sort=full_name$/);
    await expect(
      page.getByRole("button", { name: "Nach Name", exact: true }),
    ).toHaveAttribute("aria-pressed", "true");

    const built = page.getByRole("link", {
      name: de["views.opensInFilters"].replace("{name}", "Contacts I own"),
    });
    await expect(built).not.toHaveAttribute("aria-pressed");
    await expectNoAaViolations(page, "contacts — a link tab on the rail");
    await built.focus();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/#\/filters\/contacts\/v-owned$/);
    await expect(
      page.getByRole("heading", { level: 1, name: "Contacts I own" }),
    ).toBeVisible();
  });
});
