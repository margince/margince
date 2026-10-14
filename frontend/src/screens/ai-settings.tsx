import type { components } from "../api/schema";
import { PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { formatElapsed, useNow } from "../format/now";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCan } from "../lib/capability";
import { useProviderHealth } from "./ai-provider-health";
import { boundProviders, type RoutingRead } from "./ai-routing-query";
import { type LastCall, useLastCallAt } from "./aicalls";
import "./ai-settings.css";

type ProviderStatus = components["schemas"]["AiProviderKeyStatus"];

// A boot whose stored binding cannot be served may fall back to the offline
// fake. Every lane then answers with canned text, while the bindings on screen
// still name a vendor. `configured` is what the running process serves.
export function useAnswersFromStandIn(): boolean {
  const canDiagnose = useCan("ai_diagnostics", "read");
  const configured = useProviderHealth(canDiagnose).data?.configured;
  return configured?.includes("fake") === true;
}

// Whether this installation can call the vendors it has bound. A binding to a
// vendor with no key fails closed at call time, and this line shows it before
// the call.
export function ProvidersStatus({
  providers,
  routing,
}: Readonly<{
  providers: readonly ProviderStatus[];
  routing: RoutingRead["routing"] | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const lastCall = useLastCallAt();
  const standIn = useAnswersFromStandIn();
  const now = useNow(60_000);
  const keyed = providers.filter((p) => p.configured).length;
  const bound = routing ? boundProviders(routing) : null;
  // Unknown without the routing read, which is silence rather than a zero.
  const missing =
    bound === null
      ? null
      : providers.filter(
          (p) => bound.has(p.provider) && !p.configured && !p.optional,
        ).length;
  return (
    <PanelBody>
      <p className="ai-providers-status" data-testid="ai-providers-status">
        <span>
          {t("aiSettings.providers.value", {
            count: formatNumber(keyed, locale),
            total: formatNumber(providers.length, locale),
          })}
        </span>
        {missing ? (
          // ds:ignore a count label in the danger ink, not a message
          <span className="ai-settings-missing">
            {t("aiSettings.providers.missing", {
              count: formatNumber(missing, locale),
            })}
          </span>
        ) : null}
        {standIn ? (
          <span>{t("aiSettings.providers.standIn")}</span>
        ) : (
          <LastCallLine lastCall={lastCall} now={now} locale={locale} />
        )}
      </p>
    </PanelBody>
  );
}

// What each silence from the call trace is worth saying. One arm of `LastCall`
// is one row, so a state added to that union and not here fails to compile.
const TRACE_SILENCE: Record<
  Exclude<LastCall["state"], "at">,
  MessageKey | null
> = {
  // Not this reader's to see, which says nothing about the installation.
  withheld: null,
  // Still arriving, and it resolves by waiting.
  unread: null,
  failed: "aiSettings.providers.traceFailed",
  never: "aiSettings.providers.neverCalled",
};

function LastCallLine({
  lastCall,
  now,
  locale,
}: Readonly<{ lastCall: LastCall; now: number; locale: Locale }>) {
  const t = useT();
  if (lastCall.state !== "at") {
    const silence = TRACE_SILENCE[lastCall.state];
    return silence ? <span>{t(silence)}</span> : null;
  }
  return (
    <span>
      {t("aiSettings.providers.lastCall", {
        elapsed: formatElapsed(now - lastCall.epochMs, t, locale),
      })}
    </span>
  );
}
