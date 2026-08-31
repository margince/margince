// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Search } from "lucide-react";
import type { ReactNode } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { EvidenceMark } from "../design-system/evidencemark";
import { type Fact, FactList } from "../design-system/factlist";
import { Panel, PanelBody } from "../design-system/panel";
import {
  ProviderMark,
  providerBrandName,
} from "../design-system/provider-mark";
import { formatDateAbbrev, formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { categoryNames } from "./provider-categories";
import {
  canEnrichNow,
  profileLabel,
  profileTone,
  useProviderConnections,
} from "./provider-status";

// What a licensed data provider was PAID to tell us about this person
// (ADR-0101), shown BESIDE the canonical record and never folded into it.
//
// Every value carries a provenance mark, because a bought value and one a
// colleague typed are different kinds of fact and a page that renders them
// alike invites a rep to treat a purchase as a confirmation.

type Profile = components["schemas"]["PersonProviderProfile"];
type Provider = components["schemas"]["Provider"];
type ProviderConnection = components["schemas"]["ProviderConnection"];

type EnrichRun = {
  personId: string;
  provider: Provider;
  // What this press buys. Absent asks for the connection's whole selection;
  // named, it purchases one priced detail without changing what every future
  // run spends.
  categories?: string[];
};

/** The mark every value in this section carries: bought from a named third
 *  party, on a date. `connector` rather than `agent` — nothing inferred this,
 *  somebody sold it to us. */
function boughtFrom(profile: Profile) {
  return {
    provenance: {
      kind: "connector" as const,
      connector: profile.provider ?? "provider",
    },
    at: profile.retrieved_at ?? null,
  };
}

/**
 * One panel per connected provider, each named and marked with its own logo.
 *
 * Named rather than "the connected data provider": every value here was PAID
 * for, and a reader deciding whether to spend again has to know who was already
 * asked, who answered, and who is about to be. With two providers connected the
 * unnamed card could not even be acted on — there was no way to say which one
 * the button would spend with.
 */
export function PersonProviderSection({
  personId,
  profiles,
}: Readonly<{ personId: string; profiles: Profile[] | undefined }>) {
  if (!profiles) {
    // Absent means the caller lacks the grant — `sections_omitted` names it —
    // which is not the same as empty, so the section stays away entirely
    // rather than claiming this person has nothing.
    return null;
  }
  return (
    <>
      {profiles.map((profile) => (
        <ProviderPanel
          key={profile.provider}
          personId={personId}
          profile={profile}
        />
      ))}
    </>
  );
}

/** Whether this profile holds anything a provider sold us. Every field the
 *  panel can draw is asked about, including the "nobody asked for this"
 *  caption: each one is a purchase or a receipt, and a plate claiming the
 *  panel is empty must not cover any of them. */
function hasValues(profile: Profile): boolean {
  return (
    profile.emails.length > 0 ||
    profile.mobile_phones.length > 0 ||
    profile.job_history.length > 0 ||
    profile.departments.length > 0 ||
    profile.seniorities.length > 0 ||
    profile.categories_not_requested.length > 0 ||
    profile.linkedin_url != null ||
    profile.current_employment != null ||
    profile.location != null
  );
}

// Split from the section so the mutation hook sits above no early return: the
// section leaves before it knows there is a profile, and a hook cannot live
// behind that.
function ProviderPanel({
  personId,
  profile,
}: Readonly<{ personId: string; profile: Profile }>) {
  const t = useT();
  const enrich = useEnrichRun();
  const connections = useProviderConnections();
  // What costs nothing on this connection, as the server itself derives it: a
  // button that guessed the free set could ask for a priced category and
  // spend without saying so.
  const free = freeCategories(connections.data?.connections, profile.provider);
  // `isPending` is the FIRST load only — it stays false through a background
  // refetch, so a button the reader is looking at never goes dead under them.
  // Testing `data === undefined` instead would hold the button forever when the
  // query settles as an error, which is a different fact and has its own
  // refusal to show.
  const catalogPending = connections.isPending;
  // Nobody has looked this contact up yet, so the panel has no values to show
  // and the small header button is the only way to change that. An empty plate
  // that names the action instead: the reader came here to buy data, and a
  // blank card with a quiet button in its corner is how they miss that they
  // can.
  //
  // Asked of the VALUES rather than of the state alone, because never_run is
  // also what a cancelled run reads as, and a merge cancels the losing side's
  // queued run while relinking the claims both sides paid for. Such a profile
  // says never_run and carries purchases, and a plate reading "nothing bought"
  // over somebody's bought mobile number invites paying for it twice.
  const firstRun = profile.state === "never_run" && !hasValues(profile);
  // Whether a lookup is happening RIGHT NOW, which the section's own state
  // cannot answer: it reports the connection's condition ahead of the run, so
  // a live run under a connection that last failed reads as provider_error.
  const running = stillMoving(profile.latest_run);
  // The vendor as the reader should see it named. Spelled once and used by
  // both the heading and the plate: two spellings would let a section headed
  // "Surfe" explain a purchase from "surfe".
  const name = providerBrandName(profile.provider) ?? profile.provider;
  return (
    // The record page's own card, with the provider's state in the header's
    // action slot. It used to be a bare `<section className="pe-card">` — a
    // class no stylesheet defines — which read as unframed text the moment it
    // was shown anywhere but inside a drawer.
    <Panel
      // The vendor's own name and mark, so the reader knows whose data this is
      // and who the button would spend with. `providerBrandName` falls back to
      // the contract key for a provider this build has no name for — an
      // installation can register one this frontend has never heard of, and a
      // section with no heading would be worse than one headed `acmedata`.
      title={
        <span className="pe-provider-title">
          <ProviderMark providerKey={profile.provider} />
          {name}
        </span>
      }
      titleAction={
        // A live run outranks the section's own state in the HEADER, which is
        // the one place the two disagree honestly. The section state answers
        // "what should this reader know about enrichment here" and puts the
        // connection's condition first — right for a page at rest, wrong over
        // a lookup the reader is watching, where it reports a failure from
        // hours ago as though it were the thing happening now.
        //
        // Through the same two maps every other state uses, asked with
        // `in_progress` rather than with a tone picked here: a local answer
        // would be a second opinion about what running looks like, and the two
        // would drift the first time the shared one changed.
        <Badge tone={profileTone(running ? "in_progress" : profile.state)}>
          {t(profileLabel(running ? "in_progress" : profile.state))}
        </Badge>
      }
      actions={
        firstRun ? undefined : (
          <EnrichNow
            personId={personId}
            profile={profile}
            enrich={enrich}
            free={free}
            catalogPending={catalogPending}
            // This contact has already been looked up, so the press is a
            // RE-CHECK: same free details, asked again because a job may have
            // changed. The empty-state twin below is the first lookup and says
            // so instead.
            recheck
            small
          />
        )
      }
    >
      <PanelBody>
        {enrich.error != null && (
          // The refusal a POST answered with. A run the PLATFORM declined —
          // no credits, daily cap, a standing objection — never arrives here:
          // it is a skipped run, and the state badge above says which.
          <Callout tone="danger" live="alert">
            {problemMessageOf(enrich.error, t)}
          </Callout>
        )}
        {/* A lookup takes about half a minute, and for that half-minute the
            panel used to be identical to one where nothing had happened: no
            line, no change, the same buttons. A reader who pressed and saw
            nothing move concluded the button was broken, which was the only
            conclusion available to them. Above the values, because it is a
            caveat about what is under it. */}
        {running && (
          <Callout tone="info" live="status">
            {t("provider.profile.working", { provider: name })}
          </Callout>
        )}
        {firstRun ? (
          <EmptyState
            title={t("provider.profile.emptyTitle")}
            action={
              <EnrichNow
                personId={personId}
                profile={profile}
                enrich={enrich}
                free={free}
                catalogPending={catalogPending}
              />
            }
          >
            {t("provider.profile.emptyBody", { provider: name })}
          </EmptyState>
        ) : (
          <ProviderValues profile={profile} />
        )}
        <BuyPriced personId={personId} profile={profile} enrich={enrich} />
        <RunWatch personId={personId} profile={profile} />
      </PanelBody>
    </Panel>
  );
}

/** The priced details, each behind its own button with its own price.
 *
 *  Automatic enrichment takes only what the provider gives away, so these are
 *  the ones nobody has bought yet and nobody will until a reader decides this
 *  particular contact is worth it. The price is on the button because the
 *  decision is what to spend, and a button that hid its cost would be asking
 *  somebody to agree to a number they cannot see.
 */
function BuyPriced({
  personId,
  profile,
  enrich,
}: Readonly<{
  personId: string;
  profile: Profile;
  enrich: ReturnType<typeof useEnrichRun>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const connections = useProviderConnections();
  const connection = connections.data?.connections.find(
    (c) => c.provider === profile.provider,
  );
  // Only what this connection actually carries: a category an admin switched
  // off is not one to offer, and the server would refuse it anyway.
  const buyable = (connection?.catalog ?? []).filter(
    (entry) =>
      !entry.free &&
      (connection?.configuration.categories?.[entry.category] ?? false) &&
      !alreadyHeld(profile, entry.category),
  );
  if (buyable.length === 0 || !canEnrichNow(profile.state)) {
    return null;
  }
  return (
    <div className="pe-buy-row">
      {buyable.map((entry) => (
        <Button
          key={entry.category}
          small
          type="button"
          pending={enrich.isPending}
          busyLabel={t("provider.profile.lookingUp")}
          onClick={() =>
            enrich.mutate({
              personId,
              provider: profile.provider,
              categories: [entry.category],
            })
          }
        >
          {t("provider.profile.buy", {
            category: categoryNames([entry.category], t),
            credits: formatNumber(creditsOf(entry), locale),
          })}
        </Button>
      ))}
    </div>
  );
}

/** The credits one category costs, summed across pools. A category priced in
 *  two pools is one purchase, and two figures on one button would read as a
 *  choice between them. */
function creditsOf(
  entry: components["schemas"]["ProviderCategoryCost"],
): number {
  return Object.values(entry.cost).reduce((total, n) => total + n, 0);
}

/** Whether the section already shows what this category buys, so a button does
 *  not offer to buy again what is on screen. */
function alreadyHeld(profile: Profile, category: string): boolean {
  switch (category) {
    case "professional_email":
    case "personal_email":
      return profile.emails.length > 0;
    case "mobile":
      return profile.mobile_phones.length > 0;
    default:
      return false;
  }
}

// A component rather than a hook call in the section, so the watch mounts
// only where a profile exists — the section returns early without one, and a
// hook cannot sit behind that return.
function RunWatch({
  personId,
  profile,
}: Readonly<{ personId: string; profile: Profile }>) {
  useRunWatch(personId, profile);
  return null;
}

/** The receipt: when this was bought, what was asked for, how much of it came
 *  back.
 *
 *  The section showed values and nothing about the transaction, so a reader
 *  could not answer "did my lookup do anything" — a run that returned one
 *  category out of six looked exactly like one that returned all six, and a
 *  value bought months ago looked exactly like one bought a minute ago. */
function RunReceipt({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const run = profile.latest_run;
  if (!run || !profile.retrieved_at) {
    return null;
  }
  const at = formatDateAbbrev(profile.retrieved_at, locale, recordZone);
  const silent = profile.categories_without_answer;
  if (!silent) {
    // The server did not say what went unanswered — an older backend, or an
    // adapter that never declared which claim answers which category. The date
    // is still a fact; a count derived from a missing list would be an
    // invention, and inventing "everything came back" is the exact reading
    // this line exists to prevent.
    return (
      <p className="t-caption">{t("provider.profile.receiptAt", { at })}</p>
    );
  }
  // What was actually PUT TO the provider, not everything requested: a
  // fallback that never fired and a category skipped for want of its
  // prerequisite were never sent, and counting them as answered would inflate
  // the very figure this line exists to be honest about.
  const asked =
    profile.categories_asked?.length ?? run.requested_categories.length;
  return (
    <p className="t-caption">
      {t("provider.profile.receipt", {
        at,
        asked: formatNumber(asked, locale),
        answered: formatNumber(asked - silent.length, locale),
      })}
    </p>
  );
}

function ProviderValues({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const facts = useProviderFacts(profile);
  return (
    <>
      <RunReceipt profile={profile} />
      <FactList facts={facts} />
      {(profile.categories_without_answer?.length ?? 0) > 0 && (
        // What was PAID for and came back empty. The other half of the same
        // question as the line below, and the half that was missing: a run
        // answering one category out of six rendered as a plain success with
        // five silent blanks, and the reader could not tell an empty purchase
        // from a full one.
        <p className="t-caption">
          {t("provider.profile.noAnswer", {
            categories: categoryNames(
              profile.categories_without_answer ?? [],
              t,
            ),
          })}
        </p>
      )}
      {profile.categories_not_requested.length > 0 && (
        // The difference between "we asked and they had nothing" and "we
        // never asked". Without this line a blank field reads as the first
        // when it is often the second.
        <p className="t-caption">
          {t("provider.profile.notRequested", {
            categories: categoryNames(profile.categories_not_requested, t),
          })}
        </p>
      )}
    </>
  );
}

/** Every bought value as one label→value table.
 *
 *  Nine stacked `<div><Eyebrow/>…</div>` blocks before this, each styling
 *  itself: a reader scanning for the mobile number read nine headings down the
 *  card instead of one column of labels. `FactList` is the catalog's primitive
 *  for exactly this shape and it is what every other record surface uses.
 *
 *  Rows are BUILT rather than rendered conditionally, because FactList takes an
 *  array: a fact with no value is left out entirely, and an empty `<dd>` would
 *  claim we know the value and it is blank.
 */
function useProviderFacts(profile: Profile): Fact[] {
  const t = useT();
  const { locale } = useLocale();
  const source = boughtFrom(profile);
  const mark = (value: string) => (
    <EvidenceMark value={value} source={source} />
  );
  return [
    ...reachFacts(profile, t, locale, mark),
    ...roleFacts(profile, t, mark),
  ];
}

type Translate = ReturnType<typeof useT>;
type Mark = (value: string) => ReactNode;

/** How to reach them: the addresses and numbers somebody paid for, each with
 *  the qualifier that decides whether to trust it. */
function reachFacts(
  profile: Profile,
  t: Translate,
  locale: Locale,
  mark: Mark,
): Fact[] {
  const facts: Fact[] = profile.emails.map((email) => ({
    key: `email-${email.value}`,
    term: t("provider.profile.emails"),
    value: mark(email.value),
    // Which label this is, and WHOSE it is. An address the provider did not
    // classify is labelled from what we asked for, and saying so is the whole
    // point of the distinction.
    note: email.email_type
      ? t(
          email.email_type_source === "provider"
            ? "provider.profile.emailType.provider"
            : "provider.profile.emailType.requested",
          { type: email.email_type },
        )
      : undefined,
  }));
  for (const phone of profile.mobile_phones) {
    facts.push({
      key: `phone-${phone.value}`,
      term: t("provider.profile.mobiles"),
      value: mark(phone.value),
      note:
        phone.confidence == null
          ? undefined
          : t("provider.profile.confidence", {
              percent: formatNumber(Math.round(phone.confidence * 100), locale),
            }),
    });
  }
  if (profile.linkedin_url) {
    facts.push({
      key: "linkedin",
      term: t("provider.profile.linkedin"),
      value: mark(profile.linkedin_url),
    });
  }
  return facts;
}

/** Who they are: where they work now, where they worked before, and the three
 *  attributes the provider files them under. */
function roleFacts(profile: Profile, t: Translate, mark: Mark): Fact[] {
  const facts: Fact[] = [];
  if (profile.current_employment) {
    facts.push({
      key: "employment",
      term: t("provider.profile.employment"),
      value: mark(roleLine(profile.current_employment)),
    });
  }
  for (const job of profile.job_history) {
    facts.push({
      key: `job-${job.company_name}-${job.job_title ?? ""}`,
      term: t("provider.profile.jobHistory"),
      value: mark(roleLine(job)),
    });
  }
  for (const [key, term, value] of [
    ["location", "provider.profile.location", profile.location],
    [
      "departments",
      "provider.profile.departments",
      profile.departments.join(", "),
    ],
    [
      "seniorities",
      "provider.profile.seniorities",
      profile.seniorities.join(", "),
    ],
  ] as const) {
    if (value) {
      facts.push({ key, term: t(term), value: mark(value) });
    }
  }
  return facts;
}

/** A role as one line: what they do, and where. Either half may be absent —
 *  the provider returns a company with no title often enough that joining
 *  unconditionally printed a leading separator. */
function roleLine(
  role: Readonly<{ job_title?: string | null; company_name?: string | null }>,
): string {
  return [role.job_title, role.company_name].filter(Boolean).join(" · ");
}

/** The lookup itself, hoisted so the header button and the first-run plate
 *  drive ONE mutation: two `useMutation` calls would each carry their own
 *  pending and error state, and the reader would see a failure on whichever
 *  control they did not press.
 *
 *  WHO is looked up and WHERE the answer lands both arrive as mutation
 *  VARIABLES rather than as captured values. React Query re-arms a mutation's
 *  options in a passive effect, so between the commit that renders a control
 *  for one contact and that effect, the observer still holds the previous
 *  render's closure: a captured id would spend a credit on the contact the
 *  reader just left and refresh that page instead of this one. A variable
 *  cannot be older than the control that carried it. */
function useEnrichRun() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ personId, provider, categories }: EnrichRun) => {
      const { data, error } = await api.POST("/people/{id}/enrichment-runs", {
        params: { path: { id: personId } },
        body: categories ? { provider, categories } : { provider },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_data, { personId }) => {
      // The run is durable and the provider has not been called yet, so the
      // page re-reads and RunWatch picks the run up from there. Keyed off the
      // variables the run was started with, so the page that refreshes is the
      // one the request was sent for.
      void queryClient.invalidateQueries({ queryKey: ["person360", personId] });
    },
  });
}

function EnrichNow({
  personId,
  profile,
  enrich,
  free,
  catalogPending,
  recheck = false,
  small = false,
}: Readonly<{
  free: string[];
  // Whether the connection catalog has answered yet. Separate from `free`
  // being empty, which a connection selling nothing free also produces.
  catalogPending: boolean;
  personId: string;
  profile: Profile;
  enrich: ReturnType<typeof useEnrichRun>;
  // Whether this contact has been looked up before. The press buys the same
  // free set either way; what differs is what the reader is being offered — a
  // first lookup, or asking again in case a job changed. A button that says
  // "look this contact up" on a record already showing a lookup reads as the
  // thing that fetches the email, which is the one thing it never does.
  recheck?: boolean;
  small?: boolean;
}>) {
  const t = useT();
  if (!canEnrichNow(profile.state)) {
    return null;
  }
  return (
    <Button
      small={small}
      type="button"
      pending={enrich.isPending}
      busyLabel={t("provider.profile.lookingUp")}
      // Held while the catalog is still in flight, because that is the window
      // where `free` is empty for a reason that is not "nothing is free".
      // Pressing then used to omit the category list, and an omitted list asks
      // the server for the connection's whole PERMITTED selection with the
      // priced ones in it (runcategories.go) — a work email bought under a
      // button labelled free, and the wider an admin sets the selection the
      // more it costs.
      disabled={catalogPending}
      // A profile carries no provider name until a run exists, so the first
      // lookup on a contact names the one the contract has.
      onClick={() =>
        enrich.mutate({
          personId,
          provider: profile.provider ?? "surfe",
          // The free categories, NAMED, and never omitted. A connection that
          // really sells nothing free sends an empty list, which the contract
          // refuses with 422 (minItems) — a refusal the reader can see, rather
          // than a purchase they cannot.
          categories: free,
        })
      }
    >
      <Search size={15} aria-hidden="true" />{" "}
      {t(recheck ? "provider.profile.recheck" : "provider.profile.enrichNow")}
    </Button>
  );
}

/** Watches a run that is still moving and refreshes the page when it lands.
 *
 *  It polls the RUN, not the page, and reads whether to continue from the
 *  response it just received — which is the only version that can stop
 *  itself. Deciding from the `profile` prop instead would freeze the answer
 *  at whatever state mounted the poll: the prop only changes when the page
 *  refetches, which is the thing the poll exists to cause.
 */
function useRunWatch(personId: string, profile: Profile) {
  const queryClient = useQueryClient();
  const runId = profile.latest_run?.id;
  useQuery({
    queryKey: ["provider-run", personId, runId],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/people/{id}/enrichment-runs/{run_id}",
        { params: { path: { id: personId, run_id: runId ?? "" } } },
      );
      if (error) {
        throwProblem(error);
      }
      // Re-read on every answer that is not still moving, INCLUDING the
      // completed-but-unapplied tick. That tick is what turns the section from
      // "asking the provider" into the values themselves, and skipping it left
      // the page one refresh short of the thing being waited for.
      if (!stillMoving(data)) {
        void queryClient.invalidateQueries({
          queryKey: ["person360", personId],
        });
      }
      return data;
    },
    // The RUN's own state, never the section's. The section answers "what
    // should this reader know about enrichment here", and the CONNECTION's
    // condition outranks the run history in that answer — so on an impaired
    // connection the section reads provider_error while a run the reader just
    // started is genuinely in flight. Arming off the section left the watch
    // asleep through exactly that lookup: it completed, the page never
    // re-read, and the button looked dead for the half-minute it took.
    //
    // `applied` and `claims_unwritten` extend it past the terminal state: a run
    // is not done from this page's point of view until its values are ON the
    // record, and the apply commits after the run completes.
    enabled: runId != null && stillMoving(profile.latest_run),
    // The same rule the arming uses, read from the answer just received rather
    // than from the prop: the prop only changes when the page refetches, which
    // is the thing this poll exists to cause.
    refetchInterval: (query) => (stillMoving(query.state.data) ? 2500 : false),
  });
}

/** The run states that are still moving. `submitting` counts: the run is
 *  mid-flight to the provider, and treating it as finished would offer a
 *  second "look them up" while the first is still out. */
const RUNNING_RUN_STATES = new Set<
  components["schemas"]["ProviderRun"]["state"]
>(["queued", "submitting", "in_progress"]);

/** Whether this page should keep watching a run.
 *
 *  Longer than the run machine's own idea of moving. A completed run's values
 *  reach the record in a SECOND commit, so a watch that stopped at `completed`
 *  refreshed the page one step before the thing the reader is waiting for — the
 *  values — existed. `applied` is the server saying that step happened.
 *
 *  `claims_unwritten` ends it too, and is not the same fact: the purchase
 *  succeeded and the hand-off did not, so nothing further is coming and the
 *  section says so rather than spinning forever.
 *
 *  A page opened AFTER the run completed but before the apply committed still
 *  starts the watch, which is why this is asked of the run rather than of the
 *  click that started it. */
function stillMoving(
  run: components["schemas"]["ProviderRun"] | undefined,
): boolean {
  if (!run) {
    return false;
  }
  if (RUNNING_RUN_STATES.has(run.state)) {
    return true;
  }
  return run.state === "completed" && !run.applied && !run.claims_unwritten;
}

/** The categories this connection buys for nothing, from the server's own
 *  catalog. Empty while the connections are still loading, which keeps a
 *  button from asking for a set nobody has confirmed. */
function freeCategories(
  connections: ProviderConnection[] | undefined,
  name: string,
): string[] {
  const connection = connections?.find((c) => c.provider === name);
  return (connection?.catalog ?? [])
    .filter((entry) => entry.free)
    .map((entry) => entry.category);
}
