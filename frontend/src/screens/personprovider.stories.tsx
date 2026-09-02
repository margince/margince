// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { PersonProviderSection } from "./personprovider";
import {
  completedProviderRun,
  providerCompletedProfile,
} from "./personprovider.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type Profile = components["schemas"]["PersonProviderProfile"];

// What a person's page shows about data BOUGHT from a provider: the values, how
// sure the provider was of each, and the run they came from.
//
// The section's states are RUN states, not layout states — the `state` field is
// a lifecycle, and the card's header badge is where it is reported. That is why
// the stories below are named for postures a run is genuinely in rather than
// for arrangements of the same content: an expired credential and a person the
// provider simply does not know are different sentences, and only one of them
// is something an operator can act on.
//
// The completed snapshot is `personprovider.fixtures.ts`, shared with the
// research drawer's stories. A second copy would be a second claim about what
// the provider returned.

function profile(overrides: Partial<Profile> = {}): Profile {
  return { ...providerCompletedProfile, ...overrides };
}

const meta: Meta<typeof PersonProviderSection> = {
  title: "Records/Person record/Bought data",
  component: PersonProviderSection,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof PersonProviderSection>;

// The buy verb is gated on the RUN's state (`canEnrichNow`) rather than on a
// grant, so this card reads no session and the stories route none. The run read
// is routed because the section polls it while a run is moving, and an unrouted
// request would leave the page instead of answering.
function section(value: Profile | undefined) {
  return () => {
    installFetchStub({
      "GET /people/p-1/enrichment-runs/run-1": () =>
        jsonResponse(completedProviderRun),
    });
    return (
      <StoryProviders>
        <PersonProviderSection personId="p-1" profiles={value && [value]} />
      </StoryProviders>
    );
  };
}

/** A completed purchase: an address, a mobile with the provider's confidence
 *  in it, the employment, and the run behind them. */
export const Completed: Story = { render: section(profile()) };

/** The same purchase in German — the confidence sentence is where a figure and
 *  a translated sentence meet, so it is the line worth reading here. */
export const CompletedGerman: Story = {
  render: () => {
    installFetchStub({
      "GET /people/p-1/enrichment-runs/run-1": () =>
        jsonResponse(completedProviderRun),
    });
    return (
      <StoryProviders locale="de">
        <PersonProviderSection personId="p-1" profiles={[profile()]} />
      </StoryProviders>
    );
  },
};

/**
 * Several values, each at a different confidence. A provider that is 41% sure
 * of a number and 97% sure of another has said two different things, and a card
 * that printed both without the figure would be passing its own uncertainty off
 * as our claim.
 */
export const MixedConfidence: Story = {
  render: section(
    profile({
      mobile_phones: [
        { value: "+491701234567", confidence: 0.97 },
        { value: "+498912345678", confidence: 0.41 },
        // No confidence at all is a third answer, and it is not zero: the
        // provider returned the number and said nothing about it.
        { value: "+4915112345678" },
      ],
    }),
  ),
};

/**
 * A run that finished and matched nobody. The card keeps its place and says so
 * — an absent card would read as "this person has nothing to buy", which is a
 * claim about the person rather than about the run.
 */
export const NoMatch: Story = {
  render: section(
    profile({
      state: "no_match",
      emails: [],
      mobile_phones: [],
      linkedin_url: null,
      current_employment: undefined,
      job_history: [],
    }),
  ),
};

/**
 * Nothing has been bought yet, which is the state a contact sits in until
 * somebody asks. The card carries the plate that NAMES the lookup rather than
 * an empty body with a quiet button in the header: a reader who came here to
 * buy data should not have to find the verb.
 *
 * `provider` is absent on purpose. The server fills it from the newest run, so
 * a contact nobody has looked up carries no provider name — a fixture that set
 * one would prove the button works in a case that cannot happen.
 */
export const NeverRun: Story = {
  render: section(
    profile({
      state: "never_run",
      provider: "surfe",
      retrieved_at: null,
      emails: [],
      mobile_phones: [],
      linkedin_url: null,
      current_employment: undefined,
      job_history: [],
      location: null,
      departments: [],
      seniorities: [],
      latest_run: undefined,
      contributing_runs: undefined,
      categories_not_requested: [],
    }),
  ),
};

/**
 * The first lookup on a contact, in German. The plate carries the whole
 * explanation of what a lookup spends and where the answer lands, so it is the
 * copy most worth reading in a second language.
 */
export const NeverRunGerman: Story = {
  render: () => (
    <StoryProviders locale="de">
      <PersonProviderSection
        personId="p-1"
        profiles={[
          profile({
            state: "never_run",
            provider: "surfe",
            retrieved_at: null,
            emails: [],
            mobile_phones: [],
            linkedin_url: null,
            current_employment: undefined,
            job_history: [],
            location: null,
            departments: [],
            seniorities: [],
            latest_run: undefined,
            contributing_runs: undefined,
            categories_not_requested: [],
          }),
        ]}
      />
    </StoryProviders>
  ),
};

/** A run in flight. The badge carries the lifecycle and the section polls the
 *  run rather than the page, so a purchase lands without a reload. */
export const InProgress: Story = {
  render: section(profile({ state: "in_progress" })),
};

/**
 * A lookup running under a connection whose LAST call failed — the state a
 * reader is most likely to be pressing from, because the failure message tells
 * them to press it.
 *
 * The section reports the connection's condition ahead of the run, which is
 * right for a page at rest and wrong here: it named a failure from hours ago
 * over a lookup that was working. The header now says what is happening and the
 * line above the values says who is being asked.
 */
export const RunningOnAnImpairedConnection: Story = {
  render: section(
    profile({
      state: "provider_error",
      latest_run: {
        ...completedProviderRun,
        state: "in_progress",
        applied: false,
      },
    }),
  ),
};

/**
 * Bought, and the values have not reached the record yet. The apply commits
 * AFTER the run completes, so a page that stopped watching at `completed`
 * refreshed one step before the thing being waited for existed.
 */
export const CompletedButNotYetApplied: Story = {
  render: section(
    profile({
      latest_run: { ...completedProviderRun, applied: false },
    }),
  ),
};

/**
 * The credential the provider was bought through has expired. An operator
 * fault rather than a data one, so it says which — a generic failure here would
 * send somebody looking at the person instead of at the connection.
 */
export const InvalidCredentials: Story = {
  render: section(profile({ state: "invalid_credentials" })),
};

/** Out of credits: the run did not run, and nothing about the person changed.
 *  The distinction from a provider error is what an operator needs. */
export const InsufficientCredits: Story = {
  render: section(profile({ state: "insufficient_credits" })),
};

/**
 * Only one category was asked for. `categories_not_requested` is how a card
 * says a mobile is missing because nobody bought one — the alternative reads as
 * a provider that had none.
 */
export const CategoryNotRequested: Story = {
  render: section(
    profile({
      categories_not_requested: ["mobile"],
      mobile_phones: [],
    }),
  ),
};

/**
 * A purchase that came back nearly empty: six categories asked for, one
 * returned, and that one an employment the record already knew.
 *
 * This is what a bad lookup actually looks like, and it used to be
 * indistinguishable from a good one — a green "Found" over a single field, with
 * nothing saying when it happened or how much of it was missing. The receipt
 * and the not-found line are what tell the two apart.
 */
export const MostlyEmptyPurchase: Story = {
  render: section(
    profile({
      state: "completed",
      emails: [],
      mobile_phones: [],
      linkedin_url: null,
      job_history: [],
      departments: [],
      seniorities: [],
      current_employment: { company_name: "e-Kugellager" },
      categories_not_requested: [],
      categories_without_answer: [
        "professional_email",
        "mobile",
        "linkedin_profile",
        "job_history",
        "personal_email",
      ],
    }),
  ),
};

/**
 * The section WITHHELD. `person360` names it in `sections_omitted` and hands
 * down no profile, and the card is then absent rather than empty — the one
 * place that is right, because a card holding no fact cannot be misread as
 * "nothing was found".
 */
export const Withheld: Story = { render: section(undefined) };

/** At 390px the values stack under their eyebrows and the header badge sits
 *  beside a title that has to wrap rather than truncate. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: section(profile()),
};

/**
 * TWO providers connected — the state the whole per-provider split exists for.
 *
 * One has been paid and answered; the other has never been asked. Each section
 * carries its own name, mark, state and button, so the reader can see who sold
 * them the mobile number on screen and choose who to spend with next. Under a
 * single unnamed card this page could not express either fact.
 */
export const TwoProviders: Story = {
  render: () => {
    installFetchStub({
      "GET /people/p-1/enrichment-runs/run-1": () =>
        jsonResponse(completedProviderRun),
    });
    return (
      <StoryProviders>
        <div className="record-stack">
          <PersonProviderSection
            personId="p-1"
            profiles={[
              profile(),
              profile({
                // A provider this frontend has no logo or brand name for: the
                // neutral mark and the contract key stand in, which is the
                // honest answer for a name it has never heard of.
                provider: "acmedata",
                state: "never_run",
                retrieved_at: null,
                emails: [],
                mobile_phones: [],
                linkedin_url: null,
                current_employment: undefined,
                job_history: [],
                location: null,
                departments: [],
                seniorities: [],
                latest_run: undefined,
                contributing_runs: undefined,
                categories_not_requested: [],
              }),
            ]}
          />
        </div>
      </StoryProviders>
    );
  },
};

/**
 * The mobile number offered for a contact whose work email is already on
 * record — the common case once a lookup has run, and the one that spends more
 * than a reader would guess.
 *
 * Surfe will not look for a number without the email flag set, and it charges
 * for whatever it sends back regardless of what we already hold. So this press
 * costs two credits and buys the address a second time. The button names only
 * what is being SOUGHT; the line under it names what is paid for again.
 */
export const BuyMobileWithEmailHeld: Story = {
  render: () => {
    installFetchStub({
      "GET /provider-connections": () =>
        jsonResponse({
          data: [
            {
              provider: "surfe",
              status: "connected",
              credential_present: true,
              catalog: [
                { category: "linkedin_profile", free: true, cost: {} },
                {
                  category: "professional_email",
                  free: false,
                  cost: { email: 1 },
                },
                {
                  category: "mobile",
                  free: false,
                  cost: { email: 1, mobile: 1 },
                  requires: "professional_email",
                },
              ],
              configuration: {
                categories: {
                  linkedin_profile: true,
                  professional_email: true,
                  mobile: true,
                },
              },
              credits: { pools: {} },
              version: 1,
              created_at: "2026-08-20T09:00:00Z",
              updated_at: "2026-08-20T09:00:00Z",
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <PersonProviderSection
          personId="p-1"
          profiles={[profile({ state: "completed", mobile_phones: [] })]}
        />
      </StoryProviders>
    );
  },
};
