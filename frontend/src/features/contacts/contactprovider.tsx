// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge, EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  ProviderMark,
  providerBrandName,
} from "../../design-system/provider-mark";
import { WriteRefused } from "../../design-system/querystates";
import { useT } from "../../i18n";
import {
  profileLabel,
  profileTone,
  useProviderConnections,
} from "../../lib/provider-status";
// What a licensed data provider was PAID to tell us about this contact
// (ADR-0101), shown BESIDE the canonical record and never folded into it.
//
// Every value carries a provenance mark, because a bought value and one a
// colleague typed are different kinds of fact and a page that renders them
// alike invites a rep to treat a purchase as a confirmation.
import { BuyPriced } from "./contactprovider.buy";
import { LookupRunning } from "./contactprovider.notices";
import {
  EnrichNow,
  RunWatch,
  stillMoving,
  useEnrichRun,
} from "./contactprovider.run";
import { ProviderValues } from "./contactprovider.values";

export { keepPolling, POLL_LIMIT } from "./contactprovider.run";

export type Profile = components["schemas"]["ContactProviderProfile"];

export type Contact = components["schemas"]["Contact"];

export type Provider = components["schemas"]["Provider"];

type ProviderConnection = components["schemas"]["ProviderConnection"];

/**
 * One panel per connected provider, each named and marked with its own logo.
 *
 * Named rather than "the connected data provider": every value here was PAID
 * for, and a reader deciding whether to spend again has to know who was already
 * asked, who answered, and who is about to be. With two providers connected the
 * unnamed card could not even be acted on — there was no way to say which one
 * the button would spend with.
 */
export function ContactProviderSection({
  contactId,
  profiles,
}: Readonly<{ contactId: string; profiles: Profile[] | undefined }>) {
  if (!profiles) {
    // Absent means the caller lacks the grant — `sections_omitted` names it —
    // which is not the same as empty, so the section stays away entirely
    // rather than claiming this contact has nothing.
    return null;
  }
  return (
    <>
      {profiles.map((profile) => (
        <ProviderPanel
          key={profile.provider}
          contactId={contactId}
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
  contactId,
  profile,
}: Readonly<{ contactId: string; profile: Profile }>) {
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
  // The panel has no values to show, so the small header button is the only way
  // to change that. An empty plate that names the action instead: the reader
  // came here to buy data, and a blank card with a quiet button in its corner
  // is how they miss that they can.
  //
  // Asked of the VALUES rather than of the state alone, because never_run is
  // also what a cancelled run reads as, and a merge cancels the losing side's
  // queued run while relinking the claims both sides paid for. Such a profile
  // says never_run and carries purchases, and a plate reading "nothing bought"
  // over somebody's bought mobile number invites paying for it twice.
  const nothingToShow = profile.state === "never_run" && !hasValues(profile);
  // Whether the provider was ever asked about THIS contact, which is what the
  // button's two wordings claim and a different question from the one above.
  // The folded section state cannot answer it, in either direction: it reports
  // the CONNECTION's condition ahead of the run, so invalid_credentials reads
  // as a lookup over a contact nobody ever looked up, and a cancelled latest
  // run folds back to never_run over one that was. Run history answers it
  // directly, and the record already carries it — latest_run for the ordinary
  // case, contributing_runs for a merged record whose runs came from the other
  // side. Values with neither mean a retained purchase whose run rows aged
  // out, and a purchase is a lookup that happened.
  const lookedUp =
    profile.latest_run != null ||
    (profile.contributing_runs?.length ?? 0) > 0 ||
    hasValues(profile);
  // Whether a lookup is happening RIGHT NOW, which the section's own state
  // cannot answer: it reports the connection's condition ahead of the run, so
  // a live run under a connection that last failed reads as provider_error.
  const running = stillMoving(profile.latest_run);
  // Two phases, and the reader cares which. `asking` is the provider working;
  // `landing` is the provider done and the values being folded onto the record.
  // Both keep the watch alive, and saying "asking Surfe" over the second one
  // reports work nobody is doing — the vendor has already answered.
  const asking = running && profile.latest_run?.state !== "completed";
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
        <Badge tone={profileTone(asking ? "in_progress" : profile.state)}>
          {t(
            asking
              ? "provider.profile.inProgress"
              : profileLabel(profile.state),
          )}
        </Badge>
      }
      actions={
        nothingToShow ? undefined : (
          <EnrichNow
            contactId={contactId}
            profile={profile}
            enrich={enrich}
            free={free}
            catalogPending={catalogPending}
            running={running}
            // Same free details either way; only the wording differs, and it
            // reports whether this contact has a lookup behind it.
            recheck={lookedUp}
          />
        )
      }
    >
      <PanelBody>
        <WriteRefused
          titleKey="provider.profile.lookupRefused"
          error={enrich.error}
        />
        {/* Above the values, because it is a caveat about what is under it. */}
        {running && <LookupRunning asking={asking} provider={name} />}
        {nothingToShow ? (
          <EmptyState
            title={t("provider.profile.emptyTitle")}
            action={
              <EnrichNow
                contactId={contactId}
                profile={profile}
                enrich={enrich}
                free={free}
                catalogPending={catalogPending}
                running={running}
                recheck={lookedUp}
              />
            }
          >
            {t("provider.profile.emptyBody", { provider: name })}
          </EmptyState>
        ) : (
          <ProviderValues profile={profile} />
        )}
        <BuyPriced
          contactId={contactId}
          profile={profile}
          enrich={enrich}
          running={running}
        />
        <RunWatch contactId={contactId} profile={profile} />
      </PanelBody>
    </Panel>
  );
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
