import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Callout } from "../design-system/callout";
import { CellStack } from "../design-system/cellstack";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { Meter } from "../design-system/readings";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { useCan } from "../lib/capability";
import { useLicenseEntitlement } from "../lib/license-posture";
import { throwProblem } from "../lib/problem";
import { useMe } from "../lib/session";
import "./license.css";
import { LicenseHolderCard } from "./licenseholder";

// The entitlement surface: what the license grants, and how many seats are using
// it. Read-only, because there is nothing here to write — the token is resolved
// from the deployment file at boot, so an operator changes their entitlement by
// changing the deployment, not by typing into a form.
//
// Four states the server distinguishes, and the reading has to as well:
//
//   valid, with a seat count   used against granted, with the bar under it
//   valid, with no seat count  a license that caps nothing: a count, no bar
//   absent                     no license configured; nothing to measure against
//   rejected                   asked and told no — a fault with a repair behind
//                              it, which `absent` is not
//
// The middle case is why `seats_granted` is nullable rather than zero, and why
// the reading says "no limit" instead of a number: a meter filled against a
// limit nobody set would invent the limit.
//
// One reading, because used against granted is one fact. As two figures, a
// reader must work out which one is the other's denominator.
//
// Over the limit is REPORTED, never enforced. The workspace keeps working — P7's
// warning-then-grace, not a silent mid-month lockout — so the notice says what is
// true and what to do, and nothing on this screen blocks anybody.

type LicenseEntitlement = components["schemas"]["LicenseEntitlement"];

/**
 * How many seats are in use, without what the installation is entitled to.
 *
 * The capacity half on its own grant. `GET /installation/license` answers both
 * questions together and takes `license:read`, so a reader who may plan
 * headcount but may not see what the installation pays could not read either —
 * which is what `seat_usage` and this endpoint were split out for.
 *
 * It is the SAME meter: both surfaces run one statement server-side, and that
 * statement is also the ceiling that refuses the next full seat.
 */
function useSeatUsage(enabled: boolean) {
  return useQuery({
    queryKey: ["installation-seat-usage"],
    enabled,
    staleTime: 5 * 60_000,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/seat-usage",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function LicenseCard() {
  const t = useT();
  // Which half this reader may have. Asking the entitlement without the grant
  // would 403 on every load and hand them a red failure with a futile Retry;
  // asking capacity as well would spend a second request on a question the
  // entitlement already answers.
  const canReadLicense = useCan("license", "read");
  // The probe itself, not only its answer. Every capability predicate reads
  // false while /me is in flight, so `!canReadLicense` is true for EVERYBODY
  // during that window — and a fallback keyed on it alone fires the capacity
  // request for a licence holder on every load, before the grant resolves.
  //
  // Neither query runs until the snapshot lands, so exactly one of them runs.
  //
  // ANSWERED, not merely settled: a failed /me leaves `isPending` false with no
  // grants to read, and `!canReadLicense` is then true for a licence holder
  // exactly as it is for someone who may not read one. Keying the fallback on
  // that alone picked the capacity branch permanently, and its retry retried
  // the wrong request — seats, never the snapshot that failed. An unanswered
  // /me is not a narrower reader; it is no reader yet.
  const me = useMe();
  const answered = !me.isPending && !me.isError;
  const query = useLicenseEntitlement(answered && canReadLicense);
  const seats = useSeatUsage(answered && !canReadLicense);

  // Nothing is known about this reader, so neither branch may claim to be
  // their answer. The gate below renders the /me failure and its retry.
  if (me.isError) {
    return (
      <QueryGate query={me} pendingLabel={t("license.seats.title")}>
        {() => null}
      </QueryGate>
    );
  }

  if (answered && !canReadLicense) {
    // Capacity alone. The commercial standing is deliberately absent rather
    // than shown empty: an unreadable entitlement is a fact about who is
    // reading, and rendering it as "no license" would say something false
    // about the installation.
    return (
      <QueryGate query={seats} pendingLabel={t("license.seats.title")}>
        {(usage) => <SeatUsageReading seatsUsed={usage.seats_used} />}
      </QueryGate>
    );
  }

  return (
    <QueryGate query={query} pendingLabel={t("license.seats.title")}>
      {(entitlement) => (
        <>
          {/* Only a verified license has a holder. An unlicensed installation
              has nobody to name, and a refused one proved nothing about who
              holds it — so the card is absent rather than empty. */}
          {entitlement.license && (
            <LicenseHolderCard holder={entitlement.license} />
          )}
          <LicenseReading entitlement={entitlement} />
        </>
      )}
    </QueryGate>
  );
}

// What is still free, or what the count is past — the caption the seat reading
// owes, and the one an uncapped installation owes instead.
//
// Derived from the two figures the value itself shows rather than from
// `over_limit`: this line describes that value, so it has to agree with it. The
// server's verdict still owns the ALERT, which is a claim about the
// installation's standing and not about the arithmetic on screen.
function seatsDetail(
  seatsUsed: number,
  granted: number | null | undefined,
  locale: Locale,
  t: ReturnType<typeof useT>,
): string {
  if (granted === undefined || granted === null) {
    return t("license.seats.uncapped");
  }
  const spare = granted - seatsUsed;
  return spare < 0
    ? t("license.seats.over", { count: formatNumber(-spare, locale) })
    : t("license.seats.left", { count: formatNumber(spare, locale) });
}

// Exported for its story: the states worth looking at are states of the READING,
// and a story that had to stub a query to reach them would be testing the fetch.
/**
 * The seat count alone, for a reader holding capacity and not entitlement.
 *
 * Deliberately says NOTHING about a licence: not "no license configured", not
 * an empty grant, not an uncapped meter. Those are claims about the
 * installation, and what is actually true here is a fact about the reader —
 * they may not see it. A card that filled the gap with "unlicensed" would tell
 * a manager something false about the company they work for.
 */
export function SeatUsageReading({
  seatsUsed,
}: Readonly<{ seatsUsed: number }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Panel title={t("license.seats.title")}>
      <PanelBody>
        <PanelIntro>{t("license.seats.capacityOnly")}</PanelIntro>
      </PanelBody>
      <SeatsReading figure={formatNumber(seatsUsed, locale)} />
    </Panel>
  );
}

export function LicenseReading({
  entitlement,
}: Readonly<{ entitlement: LicenseEntitlement }>) {
  const t = useT();
  const { locale } = useLocale();
  const granted = entitlement.seats_granted;
  const capped = granted !== undefined && granted !== null;
  // The callout and the seat row both name the grant. One spelling keeps them
  // from disagreeing.
  const grantedText = capped
    ? formatNumber(granted, locale)
    : t("license.seats.uncapped");

  const notices =
    entitlement.state !== "valid" || entitlement.over_limit === true;
  return (
    <Panel title={t("license.seats.title")}>
      {notices && (
        <PanelBody>
          {/* This card is where the installation's licence gap is REPAIRED, so
              it states the gap in every posture; the shell banner (refused) and
              the agent panel's pill (absent or refused) name it and link here. The
              orb never does: an unlicensed demo or dev stack would wear permanent
              amber, and amber would stop meaning "a fault that can wait".

              Not `live="alert"`: `over_limit` below owns the only interruption on
              this screen, and neither of these is news that arrived while the
              reader was here. The TONES differ because the two facts do — a
              licence that was refused is a live fault with a repair behind it, and
              one that was never configured is a standing condition. */}
          {entitlement.state === "rejected" && (
            <Callout
              kind="standing"
              tone="warning"
              title={t("license.refused.title")}
            >
              {t("license.refused.body")}
            </Callout>
          )}
          {entitlement.state === "absent" && (
            <Callout kind="standing" title={t("license.absent.title")}>
              {t("license.absent.body")}
            </Callout>
          )}
          {entitlement.over_limit && (
            // `live` overrides the silence `standing` derives. A breach is true on
            // render and still interrupts, because the admin has to act on it.
            <Callout
              kind="standing"
              tone="danger"
              live="alert"
              title={t("license.over.title")}
            >
              {/* Gated on the server's verdict ALONE. `over_limit` is false
                  whenever nothing caps seats, so the contract already answers
                  this; re-deriving it here from `seats_granted` would be the
                  client reaching its own answer about the installation's
                  standing, and on a payload where the two disagreed it would
                  withdraw the one alert an admin has to act on. */}
              {t("license.over.body", {
                used: formatNumber(entitlement.seats_used, locale),
                granted: grantedText,
              })}
            </Callout>
          )}
        </PanelBody>
      )}
      <SeatsReading
        // Used against granted in one value, because that is one fact.
        figure={
          capped
            ? t("license.seats.ofGranted", {
                used: formatNumber(entitlement.seats_used, locale),
                granted: grantedText,
              })
            : formatNumber(entitlement.seats_used, locale)
        }
        // An absent or refused license says "uncapped" in its notice above.
        detail={
          entitlement.state === "valid"
            ? seatsDetail(entitlement.seats_used, granted, locale, t)
            : undefined
        }
        // Only where the reading has a denominator: a bar drawn against an
        // invented limit invents the limit.
        meter={
          capped ? { value: entitlement.seats_used, max: granted } : undefined
        }
        over={entitlement.over_limit === true}
      />
    </Panel>
  );
}

// The seat reading and what it counts, the one row both postures draw. No
// control: the count changes only with the members and the deployment.
function SeatsReading({
  figure,
  detail,
  meter,
  over = false,
}: Readonly<{
  figure: string;
  detail?: string;
  meter?: Readonly<{ value: number; max: number }>;
  over?: boolean;
}>) {
  const t = useT();
  return (
    <SettingList bleed="settings">
      <SettingRow
        label={t("license.seats.inUse")}
        description={t("license.counting")}
        value={
          <CellStack>
            <span className="t-num">{figure}</span>
            {meter && (
              <span className="license-meter">
                <Meter
                  dense
                  value={meter.value}
                  max={meter.max}
                  label={t("license.seats.inUse")}
                  tone={over ? "danger" : undefined}
                />
              </span>
            )}
            {detail && <span className="t-caption">{detail}</span>}
          </CellStack>
        }
        control={null}
      />
    </SettingList>
  );
}
