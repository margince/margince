// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { EvidenceMark } from "../design-system/evidencemark";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody } from "../design-system/panel";
import {
  ProviderMark,
  providerBrandName,
} from "../design-system/provider-mark";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import {
  canEnrichNow,
  isRunning,
  profileLabel,
  profileTone,
} from "./provider-status";

// What a licensed data provider was PAID to tell us about this person
// (ADR-0101), shown BESIDE the canonical record and never folded into it.
//
// Every value carries a provenance mark, because a bought value and one a
// colleague typed are different kinds of fact and a page that renders them
// alike invites a rep to treat a purchase as a confirmation.

type Profile = components["schemas"]["PersonProviderProfile"];
type Provider = components["schemas"]["Provider"];

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
  const enrich = useEnrichRun(personId);
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
          {providerBrandName(profile.provider) ?? profile.provider}
        </span>
      }
      titleAction={
        <Badge tone={profileTone(profile.state)}>
          {t(profileLabel(profile.state))}
        </Badge>
      }
      actions={
        firstRun ? undefined : (
          <EnrichNow profile={profile} enrich={enrich} small />
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
        {firstRun ? (
          <EmptyState
            title={t("provider.profile.emptyTitle")}
            action={<EnrichNow profile={profile} enrich={enrich} />}
          >
            {t("provider.profile.emptyBody")}
          </EmptyState>
        ) : (
          <ProviderValues profile={profile} />
        )}
        <RunWatch personId={personId} profile={profile} />
      </PanelBody>
    </Panel>
  );
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

function ProviderValues({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const { locale } = useLocale();
  const source = boughtFrom(profile);
  return (
    <>
      {profile.emails.length > 0 && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.emails")}</Eyebrow>
          {profile.emails.map((email) => (
            <div key={email.value}>
              <EvidenceMark value={email.value} source={source} />{" "}
              {email.email_type && (
                <span className="t-caption">
                  {/* Which label this is, and WHOSE it is. An address the
                      provider did not classify is labelled from what we
                      asked for, and saying so is the whole point of the
                      distinction. */}
                  {email.email_type_source === "provider"
                    ? t("provider.profile.emailType.provider", {
                        type: email.email_type,
                      })
                    : t("provider.profile.emailType.requested", {
                        type: email.email_type,
                      })}
                </span>
              )}
            </div>
          ))}
        </div>
      )}
      {profile.mobile_phones.length > 0 && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.mobiles")}</Eyebrow>
          {profile.mobile_phones.map((phone) => (
            <div key={phone.value}>
              <EvidenceMark value={phone.value} source={source} />{" "}
              {phone.confidence != null && (
                <span className="t-caption">
                  {t("provider.profile.confidence", {
                    percent: formatNumber(
                      Math.round(phone.confidence * 100),
                      locale,
                    ),
                  })}
                </span>
              )}
            </div>
          ))}
        </div>
      )}
      {profile.linkedin_url && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.linkedin")}</Eyebrow>
          <EvidenceMark value={profile.linkedin_url} source={source} />
        </div>
      )}
      {profile.current_employment && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.employment")}</Eyebrow>
          <EvidenceMark
            value={[
              profile.current_employment.job_title,
              profile.current_employment.company_name,
            ]
              .filter(Boolean)
              .join(" · ")}
            source={source}
          />
        </div>
      )}
      {profile.job_history.length > 0 && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.jobHistory")}</Eyebrow>
          {profile.job_history.map((job) => (
            <div key={`${job.company_name}-${job.job_title ?? ""}`}>
              <EvidenceMark
                value={[job.job_title, job.company_name]
                  .filter(Boolean)
                  .join(" · ")}
                source={source}
              />
            </div>
          ))}
        </div>
      )}
      {profile.location && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.location")}</Eyebrow>
          <EvidenceMark value={profile.location} source={source} />
        </div>
      )}
      {profile.departments.length > 0 && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.departments")}</Eyebrow>
          <EvidenceMark
            value={profile.departments.join(", ")}
            source={source}
          />
        </div>
      )}
      {profile.seniorities.length > 0 && (
        <div>
          <Eyebrow as="h4">{t("provider.profile.seniorities")}</Eyebrow>
          <EvidenceMark
            value={profile.seniorities.join(", ")}
            source={source}
          />
        </div>
      )}
      {profile.categories_not_requested.length > 0 && (
        // The difference between "we asked and they had nothing" and "we
        // never asked". Without this line a blank field reads as the first
        // when it is often the second.
        <p className="t-caption">
          {t("provider.profile.notRequested", {
            categories: profile.categories_not_requested.join(", "),
          })}
        </p>
      )}
    </>
  );
}

/** The lookup itself, hoisted so the header button and the first-run plate
 *  drive ONE mutation: two `useMutation` calls would each carry their own
 *  pending and error state, and the reader would see a failure on whichever
 *  control they did not press.
 *
 *  The provider arrives as a mutation VARIABLE rather than a captured value.
 *  React Query re-arms a mutation's options in a passive effect, so a function
 *  closing over render state can run against the previous render's copy. */
function useEnrichRun(personId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["enrich", personId],
    mutationFn: async ({ provider }: { provider: Provider }) => {
      const { data, error } = await api.POST("/people/{id}/enrichment-runs", {
        params: { path: { id: personId } },
        body: { provider },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      // The run is durable and the provider has not been called yet, so the
      // page re-reads and RunWatch picks the run up from there.
      void queryClient.invalidateQueries({ queryKey: ["person360", personId] });
    },
  });
}

function EnrichNow({
  profile,
  enrich,
  small = false,
}: Readonly<{
  profile: Profile;
  enrich: ReturnType<typeof useEnrichRun>;
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
      // A profile carries no provider name until a run exists, so the first
      // lookup on a contact names the one the contract has.
      onClick={() => enrich.mutate({ provider: profile.provider ?? "surfe" })}
    >
      <Search size={15} aria-hidden="true" /> {t("provider.profile.enrichNow")}
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
      // A run that has stopped moving is the page's cue to re-read: the
      // claims land in the same transaction as the terminal state, so by
      // now the section has something new to show.
      if (data && !RUNNING_RUN_STATES.has(data.state)) {
        void queryClient.invalidateQueries({
          queryKey: ["person360", personId],
        });
      }
      return data;
    },
    enabled: runId != null && isRunning(profile.state),
    refetchInterval: (query) =>
      query.state.data && RUNNING_RUN_STATES.has(query.state.data.state)
        ? 2500
        : false,
  });
}

/** The run states that are still moving. `submitting` counts: the run is
 *  mid-flight to the provider, and treating it as finished would offer a
 *  second "look them up" while the first is still out. */
const RUNNING_RUN_STATES = new Set<
  components["schemas"]["ProviderRun"]["state"]
>(["queued", "submitting", "in_progress"]);
