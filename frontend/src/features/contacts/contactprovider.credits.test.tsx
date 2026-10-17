/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactProviderSection } from "./contactprovider";
import { providerCompletedProfile } from "./contactprovider.fixtures";
import {
  completedKeepingNothing,
  neverRun,
  type Profile,
  queuedRun,
} from "./contactprovider.harness";

afterEach(() => {
  cleanup();
});

// Buying contact data is the one thing on this page that spends money. The
// contact nobody has bought for is the case that has to work. A rep who turned
// automatic lookups off reaches the provider only through this panel. A verb
// they cannot find is a capability they do not have.

type Contact = components["schemas"]["Contact"];

describe("the details that cost credits", () => {
  // Surfe's shape: the mobile lookup is issued only when a work email comes
  // back, so its price is the pair's and `requires` names the other half.
  // A fixture pricing it alone would model a provider that does not exist.
  // It would let the button send the request the server refuses.
  const catalog = [
    { category: "linkedin_profile", free: true, cost: {} },
    { category: "professional_email", free: false, cost: { email: 1 } },
    {
      category: "mobile",
      free: false,
      cost: { mobile: 1, email: 1 },
      requires: "professional_email",
    },
    // The OTHER pairing, and a different relation: the personal address is a
    // fallback the vendor issues only when the professional pass comes back
    // empty. Priced at the trigger's credit plus the fallback's two, which is
    // what the server reserves for the pair.
    {
      category: "personal_email",
      free: false,
      cost: { email: 3 },
      follows: "professional_email",
    },
  ];

  /** The record the panel reads its own held addresses from. Stubbed in every
   *  case rather than left to the fetch fallback: the fallback answers an
   *  empty PAGE, which has no `contact` at all, so a case that leaned on it
   *  would be asserting against a payload this endpoint cannot return. */
  function recordWith(contact: Partial<Contact> = {}): Response {
    return jsonResponse({
      as_of: "2026-08-20T09:00:00Z",
      contact: {
        id: "p-1",
        full_name: "Dana Ruiz",
        emails: [],
        phones: [],
        ...contact,
      },
      sections_omitted: [],
    });
  }

  function mountWithCatalog(
    profile: Profile,
    run: () => Response,
    // Which categories the admin has switched on. The default is all three,
    // which almost every case wants. A case about the selection says so here,
    // instead of building a second stub beside this one.
    categories: Record<string, boolean> = {
      linkedin_profile: true,
      professional_email: true,
      mobile: true,
    },
    // What the CRM record holds, which the provider's own answer does not say.
    // An address somebody typed in or a mailbox captured is on the record and
    // was never bought here. Empty is the default because most cases are about
    // the provider's half.
    record: Partial<Contact> = {},
  ) {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /provider-connections": () =>
        jsonResponse({
          data: [
            {
              provider: "surfe",
              status: "connected",
              credential_present: true,
              catalog,
              configuration: { categories },
              credits: { pools: {} },
              version: 1,
              created_at: "2026-08-20T09:00:00Z",
              updated_at: "2026-08-20T09:00:00Z",
            },
          ],
        }),
      "GET /contacts/p-1/360": () => recordWith(record),
      "POST /contacts/p-1/enrichment-runs": (body) => {
        posted.push(body);
        return run();
      },
    });
    render(
      <StoryProviders>
        <ContactProviderSection contactId="p-1" profiles={[profile]} />
      </StoryProviders>,
    );
    return posted;
  }

  // The free button must never send an omitted category list.
  //
  // An omitted list asks the server for the connection's whole permitted
  // selection, priced categories included (runcategories.go). A press before
  // the catalog arrives would spend credits under a label that says free.
  // The wider an admin sets the selection, the more it costs. The catalog says
  // which categories are free, so until it lands the button has nothing safe to send.
  it("withholds the free lookup until the catalog says what free means", async () => {
    const posted: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      // A request that never settles is the loading window. Omitting the route
      // would resolve to the stub's fallback and settle the query. The button
      // would then be enabled because the catalog answered, and this case would
      // pass while testing nothing.
      "GET /provider-connections": () =>
        new Response(new ReadableStream({ start() {} }), {
          headers: { "Content-Type": "application/json" },
        }),
      "POST /contacts/p-1/enrichment-runs": (body) => {
        posted.push(body);
        return queuedRun();
      },
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
          profiles={[completedKeepingNothing()]}
        />
      </StoryProviders>,
    );

    const button = await screen.findByRole("button", { name: /Check again/ });
    expect(button).toHaveProperty("disabled", true);
    await userEvent.setup().click(button);
    expect(posted).toEqual([]);
  });

  it("offers each priced detail with its price on the button", async () => {
    mountWithCatalog(completedKeepingNothing(), queuedRun);

    // The price is on the button: the decision is what to spend. A button that
    // hid its cost would ask somebody to agree to a number they cannot see.
    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    // The mobile button names both halves and the price of both. The provider
    // will not look for a number without an email to anchor it. A button
    // offering "mobile, 1 credit" would promise a purchase that cannot happen
    // and understate what it spends.
    //
    // Joined by "and", not a comma. A comma reads as a list of things the press
    // might pick from. A rep who took it for the work-email button bought
    // a mobile number he had not asked for.
    expect(
      await screen.findByRole("button", {
        name: /Buy work email and mobile number · 2 credits/,
      }),
    ).toBeDefined();

    // No button for a free category: it arrives without anybody pressing
    // anything, which is the reason for the split.
    expect(screen.queryByRole("button", { name: /Buy LinkedIn/ })).toBeNull();
  });

  it("buys only the detail whose button was pressed", async () => {
    const user = userEvent.setup();
    const posted = mountWithCatalog(completedKeepingNothing(), queuedRun);

    // Anchored on the price, because the mobile button names the work email
    // too (it cannot be bought without one). A loose /Buy work email/
    // matches both.
    await user.click(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    );

    // One category, named. A press that sent the connection's whole selection
    // would spend on the mobile too, which nobody asked for.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["professional_email"],
    });
  });

  it("buys the email with the mobile, because the provider will not look for one without it", async () => {
    const user = userEvent.setup();
    const posted = mountWithCatalog(completedKeepingNothing(), queuedRun);

    await user.click(
      await screen.findByRole("button", {
        name: /Buy work email and mobile number · 2 credits/,
      }),
    );

    // Both categories, and the email first. Surfe always sends
    // skipMobileEnrichmentIfNoEmailFound. A request naming the mobile alone
    // makes it hunt for an email nobody bought, fail, and skip the number.
    // The run then completes with nothing in it. The server refuses that
    // request; this is what stops the button making it.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["professional_email", "mobile"],
    });
  });

  it("offers the mobile with the email already held, and says the email is re-bought", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        // The 819-contact case in a real installation: a work email bought
        // earlier, no number yet.
        emails: [
          {
            value: "dana.buyer@surfe.example",
            email_type: "professional",
            email_type_source: "provider",
            validation_status: "valid",
          },
        ],
      },
      queuedRun,
    );

    // Still offered. The filter reads the button's own category alone. A
    // filter on the whole press would remove the only way to buy a number for
    // a contact whose email is already on record.
    const button = await screen.findByRole("button", {
      name: /Buy mobile number · 2 credits/,
    });
    expect(button).toBeDefined();

    // And the second credit is named. Surfe charges per pool that returns
    // anything and cannot be told to skip an address we already hold. So this
    // press pays for that email twice, and the reader has to see that before
    // pressing, not discover it on the spend history.
    expect(
      await screen.findByText(/includes the work email again/),
    ).toBeDefined();

    // The button does not offer to buy the email: the reader can see they
    // have it. Naming it there would read as a second purchase of a detail
    // already on screen.
    expect(
      screen.queryByRole("button", { name: /Buy work email and mobile/ }),
    ).toBeNull();
  });

  it("still offers the work email when only a personal address came back", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        // A personal address and nothing else. The two are separate purchases
        // from separate pools, so this contact still has no work email.
        emails: [
          {
            value: "dana@privatemail.example",
            email_type: "personal",
            email_type_source: "provider",
            validation_status: "valid",
          },
        ],
      },
      queuedRun,
    );

    // Reading "any address at all" hides each email offer behind the other.
    // A contact whose personal address came back would get no work email
    // offer, and the reverse.
    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();

    // And the mobile press does not claim to re-buy a work email nobody
    // holds. The note names what is paid for twice.
    expect(
      await screen.findByRole("button", {
        name: /Buy work email and mobile number · 2 credits/,
      }),
    ).toBeDefined();
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  it("offers both email purchases when the provider classified neither", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        // Surfe can omit emailType, and the platform only labels it from the
        // frozen cascade where it can. An address of unknown type is one of
        // the two purchases. Guessing which would either hide an offer the
        // reader can still use, or claim a re-buy that never happens.
        emails: [
          {
            value: "dana@unknown.example",
            email_type: null,
            email_type_source: null,
            validation_status: null,
          },
        ],
      },
      queuedRun,
    );

    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    // No re-buy claim either: nothing here is known to be the work email.
    // Saying the press pays for one twice would be a guess presented as a fact.
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  it("says nothing about re-buying when neither half is held", async () => {
    mountWithCatalog(completedKeepingNothing(), queuedRun);

    // The note is the exception, not the furniture. A line about re-buying on
    // every button would train the reader to skip it when it matters.
    await screen.findByRole("button", {
      name: /Buy work email and mobile number · 2 credits/,
    });
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  // The loading window, which on this row is a money question, not a cosmetic
  // one. A button pressed before the record answers cannot say which half of
  // its price buys something already on file. So it waits.
  //
  // A route that never settles is that window. Omitting it would fall through
  // to the stub's empty page, the query would settle, and this case would pass
  // while testing nothing.
  it("offers no purchase until the record says what it already holds", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "GET /provider-connections": () =>
        jsonResponse({
          data: [
            {
              provider: "surfe",
              status: "connected",
              credential_present: true,
              catalog,
              configuration: {
                categories: { professional_email: true, mobile: true },
              },
              credits: { pools: {} },
              version: 1,
              created_at: "2026-08-20T09:00:00Z",
              updated_at: "2026-08-20T09:00:00Z",
            },
          ],
        }),
      "GET /contacts/p-1/360": () =>
        new Response(new ReadableStream({ start() {} }), {
          headers: { "Content-Type": "application/json" },
        }),
    });
    render(
      <StoryProviders>
        <ContactProviderSection
          contactId="p-1"
          profiles={[completedKeepingNothing()]}
        />
      </StoryProviders>,
    );

    // The free lookup is unaffected: it spends nothing, so it has nothing to
    // disclose and no reason to wait. Its enabled state also proves the catalog
    // has landed, since it stays disabled until the catalog says what free means.
    // So every priced button that will be drawn is drawn by the time it goes
    // live. Asserting the absence any earlier would pass on a row that had not
    // been rendered yet.
    const free = await screen.findByRole("button", { name: /Check again/ });
    await expect.poll(() => free).toHaveProperty("disabled", false);
    expect(screen.queryByRole("button", { name: /^Buy / })).toBeNull();
  });

  // The address the provider never sold us. A rep typed the work email in from
  // a business card, or a mailbox connector captured it off a signature. It is
  // on the record and the provider's answer says nothing about it. Surfe still
  // bills for the email pool when the mobile press makes it look again.
  it("names the re-buy for a work address that came from the record", async () => {
    mountWithCatalog(completedKeepingNothing(), queuedRun, undefined, {
      emails: [
        {
          id: "e-1",
          email: "dana@acme.example",
          email_type: "work",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "u-1",
        },
      ],
    });

    // The press still costs two credits and still returns a number, so it is
    // still offered, with the second charge named.
    expect(
      await screen.findByRole("button", {
        name: /Buy mobile number · 2 credits/,
      }),
    ).toBeDefined();
    expect(
      await screen.findByText(/includes the work email again/),
    ).toBeDefined();

    // And nothing offers to buy an address the reader can already see on the
    // record, whoever put it there.
    expect(screen.queryByRole("button", { name: /Buy work email/ })).toBeNull();
  });

  it("still offers the work email when the record holds only a personal one", async () => {
    mountWithCatalog(completedKeepingNothing(), queuedRun, undefined, {
      emails: [
        {
          id: "e-2",
          email: "dana@privatemail.example",
          email_type: "personal",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "u-1",
        },
      ],
    });

    // The record keeps the same type distinction the provider's answer does,
    // for the same reason. The two are separate purchases from separate pools,
    // so a private address is no reason to withhold a work one.
    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  // `other` is the record's third filing. It is one of the two purchases and
  // nobody said which, so it buys no offer and claims no re-buy. The
  // provider's unclassified address gets the same answer.
  it("treats an address the record files as other as neither purchase", async () => {
    mountWithCatalog(completedKeepingNothing(), queuedRun, undefined, {
      emails: [
        {
          id: "e-3",
          email: "dana@unknown.example",
          email_type: "other",
          is_primary: false,
          position: 0,
          source: "import",
          captured_by: "u-1",
        },
      ],
    });

    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    expect(screen.queryByText(/includes the work email again/)).toBeNull();
  });

  // The other half of the same press, and the same defect: a number already on
  // the record is one the mobile press pays for again.
  it("offers no mobile purchase for a number the record already holds", async () => {
    mountWithCatalog(completedKeepingNothing(), queuedRun, undefined, {
      phones: [
        {
          id: "ph-1",
          phone: "+491701234567",
          phone_type: "mobile",
          is_primary: true,
          position: 0,
          source: "manual",
          captured_by: "u-1",
        },
      ],
    });

    expect(
      await screen.findByRole("button", { name: /Buy work email · 1 credit/ }),
    ).toBeDefined();
    expect(screen.queryByRole("button", { name: /mobile number/ })).toBeNull();
  });

  it("offers nothing once both halves of a press are held", async () => {
    mountWithCatalog(
      {
        ...neverRun(),
        state: "completed",
        emails: [
          {
            value: "dana.buyer@surfe.example",
            email_type: "professional",
            email_type_source: "provider",
            validation_status: "valid",
          },
        ],
        mobile_phones: [{ value: "+491701234567", confidence: 0.82 }],
      },
      queuedRun,
    );

    await screen.findByText(/dana\.buyer@surfe\.example/);
    // Nothing left to look for, so no offer. In particular, no two-credit
    // button that would buy both again.
    expect(screen.queryByRole("button", { name: /^Buy / })).toBeNull();
  });

  // A fallback is not a purchase of its own. Surfe issues the personal address
  // only when the professional pass returns nothing. A press naming it alone is
  // refused every time: "personal_email is a fallback for professional_email
  // and cannot be bought without it". Yet the button quotes the price of both.
  // The catalog says which category triggers it, and the press names the pair
  // the price is for.
  it("sends the trigger alongside a fallback the provider only issues after it", async () => {
    const user = userEvent.setup();
    const posted = mountWithCatalog(
      completedKeepingNothing(),
      queuedRun,
      // The fallback switched on and the mobile off, so the row under test is
      // the cascade rather than the prerequisite beside it.
      {
        linkedin_profile: true,
        professional_email: true,
        personal_email: true,
      },
    );

    await user.click(
      await screen.findByRole("button", {
        name: /Buy work email and personal email · 3 credits/,
      }),
    );

    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["professional_email", "personal_email"],
    });
  });

  it("names the free categories when the plain lookup button is pressed", async () => {
    const user = userEvent.setup();
    const posted = mountWithCatalog(completedKeepingNothing(), queuedRun);

    // A run that completed, so the button offers a re-check rather than a
    // first lookup. Either wording drives the same press; this case pins
    // what it sends.
    await user.click(
      await screen.findByRole("button", { name: /Check again/ }),
    );

    // The free set, named. Sending no categories asks for the connection's
    // whole selection, priced ones included. That button would spend without
    // saying so, which the split exists to prevent.
    await expect.poll(() => posted.length).toBe(1);
    expect(posted[0]).toEqual({
      provider: "surfe",
      categories: ["linkedin_profile"],
    });
  });

  it("does not offer to buy what the section already shows", async () => {
    mountWithCatalog(providerCompletedProfile, queuedRun);

    // The completed fixture already carries an address and a mobile. Offering
    // to buy them again would spend credits on what is on screen.
    await screen.findByText(providerCompletedProfile.emails[0].value);
    expect(screen.queryByRole("button", { name: /Buy work email/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Buy mobile/ })).toBeNull();
  });
});
