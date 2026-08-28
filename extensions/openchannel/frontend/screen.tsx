import { api, QueryStates, throwProblem } from "@margince/frontend/api";
import { useCan, useT } from "@margince/frontend/app";
import { Card, SectionHeader } from "@margince/frontend/design-system";
import { useQuery } from "@tanstack/react-query";
import {
  ENDPOINT_KEY,
  ENDPOINT_OBJECT,
  type Endpoint,
  INBOUND_OBJECT,
  OUTBOUND_OBJECT,
  POLL_MS,
} from "./contract";
import { EndpointCard } from "./endpointcard";
import { InboundList, OutboundList } from "./traffic";

// #/ext/openchannel — the whole connector, driven from one screen.
//
// It lives in the UNIT's own tree as a pnpm workspace package: unit-authored
// TSX compiled into the SPA bundle, with react, react-dom and react-query
// declared as PEERS so the host's single copy of the hook dispatcher and the
// QueryClient context is the one that runs.
//
// EVERYTHING HERE IS THE CALLER'S OWN. Not one of the unit's seven operations
// takes a member argument, so there is no version of this screen that opens,
// re-points or reads a colleague's endpoint — the owner recorded on an
// endpoint is whose secret verifies what arrives on it and whose authority the
// payload is eventually acted under.
//
// THE SCREEN SHOWS THE SIGNING SECRET EXACTLY ONCE, at the moment it is
// minted, and says so beside it. No operation reads one back, so a lost secret
// is re-minted rather than recovered.

/**
 * The caller's own endpoint, or the honest absence of one.
 *
 * `enabled` is the read grant: without it an ungranted seat fires a request
 * the server answers 403, and then fires it again every {@link POLL_MS},
 * because this query polls. What that seat should see is "you were not granted
 * this", not a failing read on a timer.
 */
function useEndpoint(enabled: boolean) {
  return useQuery({
    enabled,
    refetchInterval: POLL_MS,
    queryKey: ENDPOINT_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/ext/openchannel/endpoint",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      // The declared field or an error. An absent `opened` is undefined, which
      // is falsey — so a body this screen could not read would render "no
      // endpoint", which is a claim about the member's own edge made from a
      // read that established nothing, and it invites them to open one over an
      // endpoint senders are already configured against.
      if (typeof data?.opened !== "boolean") {
        throw new Error("the endpoint read carried no `opened` field");
      }
      // `null` rather than a missing member: react-query refuses an undefined
      // query result, and "the read answered and there is no endpoint" is the
      // ordinary first state of this screen rather than an absence of data.
      const endpoint: Endpoint | null = data.endpoint ?? null;
      return { endpoint };
    },
  });
}

export default function OpenchannelScreen() {
  const t = useT();
  const canReadEndpoint = useCan(ENDPOINT_OBJECT, "read");
  const canReadInbound = useCan(INBOUND_OBJECT, "read");
  const canReadOutbound = useCan(OUTBOUND_OBJECT, "read");
  const endpoint = useEndpoint(canReadEndpoint);
  return (
    <div className="wrap narrow">
      {/* level 1: the app shell yields the page's name to a composed unit, so
          the screen's own top header IS the page's h1. */}
      <SectionHeader
        title={t("extOpenchannel.title")}
        sub={t("extOpenchannel.sub")}
        level={1}
      />
      <Card>
        {canReadEndpoint ? (
          // Through the query gate, not off `endpoint.data` directly: data is
          // undefined both while the read is in flight and when it failed, and
          // rendering either as "no endpoint" states something the read did
          // not establish.
          <QueryStates query={endpoint}>
            <EndpointCard endpoint={endpoint.data?.endpoint} />
          </QueryStates>
        ) : (
          <p>{t("extOpenchannel.endpoint.noGrant")}</p>
        )}
      </Card>
      <Card>
        <InboundList canRead={canReadInbound} />
      </Card>
      <Card>
        <OutboundList canRead={canReadOutbound} />
      </Card>
    </div>
  );
}
