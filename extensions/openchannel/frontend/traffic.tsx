import { api, QueryStates, throwProblem } from "@margince/frontend/api";
import { formatDateTime, useLocale, useT } from "@margince/frontend/app";
import {
  Badge,
  DataTable,
  EmptyState,
  SectionHeader,
} from "@margince/frontend/design-system";
import { useQuery } from "@tanstack/react-query";
import {
  INBOUND_KEY,
  type InboundEntry,
  OUTBOUND_KEY,
  type OutboundAttempt,
  POLL_MS,
} from "./contract";

// What arrived, and what left. Two listings rather than one, because who has
// been messaging a member and who that member has been messaging are two
// questions an operator answers for two different sets of people — which is
// why they are two RBAC objects, and why each list is asked for only by a seat
// that holds its own.
//
// NEITHER LISTING CARRIES A PAYLOAD. An inbound body is bytes an anonymous
// party chose and an outbound body is already on the record it was sent from,
// so both tables show what BECAME of a message and never the message.

/** The reader's own zone: "when did this arrive" is read against their clock. */
function readerZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/**
 * How a queue state is worded.
 *
 * A closed map with a named fallback, rather than interpolating the token
 * straight into a copy key: the contract publishes three states and the table
 * writes a fourth (`withdrawn`, for a request whose timeline entry was since
 * archived), so a screen that trusted the enum would render a raw token to a
 * reader. The fallback says the state has a name this screen does not know,
 * which is true, instead of showing them the column value.
 */
const STATE_COPY: Readonly<Record<string, `ext${string}`>> = {
  pending: "extOpenchannel.state.pending",
  ingested: "extOpenchannel.state.ingested",
  failed: "extOpenchannel.state.failed",
  withdrawn: "extOpenchannel.state.withdrawn",
};

/** Which states read as settled, as caution, and as stopped. */
const STATE_TONE: Readonly<Record<string, "success" | "warn" | "danger">> = {
  pending: "warn",
  ingested: "success",
  failed: "danger",
  withdrawn: "warn",
};

const OUTCOME_COPY: Readonly<Record<string, `ext${string}`>> = {
  sent: "extOpenchannel.outcome.sent",
  refused: "extOpenchannel.outcome.refused",
  unknown: "extOpenchannel.outcome.unknown",
};

const OUTCOME_TONE: Readonly<Record<string, "success" | "warn" | "danger">> = {
  sent: "success",
  refused: "danger",
  unknown: "warn",
};

/**
 * `enabled` is the caller's read grant rather than a convenience: without it
 * an ungranted seat fires a request the server answers 403 — and then fires it
 * again every {@link POLL_MS}, because these listings poll.
 */
function useTraffic<Row>(
  enabled: boolean,
  key: readonly string[],
  read: () => Promise<Row[]>,
) {
  return useQuery({
    enabled,
    refetchInterval: POLL_MS,
    queryKey: key,
    queryFn: read,
  });
}

export function InboundList({ canRead }: Readonly<{ canRead: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = readerZone();
  const inbound = useTraffic<InboundEntry>(canRead, INBOUND_KEY, async () => {
    const { data, error, response } = await api.GET("/ext/openchannel/inbound");
    if (error || !response.ok) {
      throwProblem(error);
    }
    if (!Array.isArray(data?.entries)) {
      throw new Error("the inbound listing carried no `entries` array");
    }
    return data.entries;
  });
  return (
    <>
      <SectionHeader
        title={t("extOpenchannel.inbound.title")}
        sub={t("extOpenchannel.inbound.sub")}
      />
      {canRead ? (
        <QueryStates query={inbound}>
          {inbound.data && inbound.data.length > 0 ? (
            <DataTable<InboundEntry>
              label={t("extOpenchannel.inbound.title")}
              rows={inbound.data}
              rowKey={(row) => row.id}
              columns={[
                {
                  key: "state",
                  header: t("extOpenchannel.inbound.state"),
                  render: (row) => (
                    <Badge quiet tone={STATE_TONE[row.state]}>
                      {t(
                        STATE_COPY[row.state] ?? "extOpenchannel.state.other",
                        { state: row.state },
                      )}
                    </Badge>
                  ),
                },
                {
                  key: "received",
                  header: t("extOpenchannel.inbound.received"),
                  render: (row) =>
                    formatDateTime(row.received_at, locale, zone),
                },
                {
                  key: "attempts",
                  header: t("extOpenchannel.inbound.attempts"),
                  render: (row) => row.attempts,
                },
                {
                  key: "bytes",
                  header: t("extOpenchannel.inbound.bytes"),
                  render: (row) => row.body_bytes,
                },
                {
                  key: "why",
                  header: t("extOpenchannel.inbound.why"),
                  // The class in THIS connector's own vocabulary, never a
                  // remote party's message: a stranger's prose is not this
                  // installation's to display, and a class has a translation
                  // while a message does not.
                  render: (row) =>
                    row.last_error_class
                      ? t(`extOpenchannel.error.${row.last_error_class}`)
                      : "",
                },
              ]}
            />
          ) : (
            <EmptyState>
              <p className="t-small">{t("extOpenchannel.inbound.empty")}</p>
            </EmptyState>
          )}
        </QueryStates>
      ) : (
        <p>{t("extOpenchannel.inbound.noGrant")}</p>
      )}
    </>
  );
}

export function OutboundList({ canRead }: Readonly<{ canRead: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = readerZone();
  const outbound = useTraffic<OutboundAttempt>(
    canRead,
    OUTBOUND_KEY,
    async () => {
      const { data, error, response } = await api.GET(
        "/ext/openchannel/outbound",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      if (!Array.isArray(data?.attempts)) {
        throw new Error("the outbound listing carried no `attempts` array");
      }
      return data.attempts;
    },
  );
  return (
    <>
      <SectionHeader
        title={t("extOpenchannel.outboundList.title")}
        sub={t("extOpenchannel.outboundList.sub")}
      />
      {canRead ? (
        <QueryStates query={outbound}>
          {outbound.data && outbound.data.length > 0 ? (
            <DataTable<OutboundAttempt>
              label={t("extOpenchannel.outboundList.title")}
              rows={outbound.data}
              rowKey={(row) => row.id}
              columns={[
                {
                  key: "outcome",
                  header: t("extOpenchannel.outboundList.outcome"),
                  render: (row) => (
                    <Badge quiet tone={OUTCOME_TONE[row.outcome]}>
                      {t(
                        OUTCOME_COPY[row.outcome] ??
                          "extOpenchannel.outcome.other",
                        { outcome: row.outcome },
                      )}
                    </Badge>
                  ),
                },
                {
                  key: "recipient",
                  header: t("extOpenchannel.outboundList.recipient"),
                  render: (row) => row.recipient,
                },
                {
                  key: "attempt",
                  header: t("extOpenchannel.outboundList.attempt"),
                  render: (row) => row.attempt,
                },
                {
                  key: "when",
                  header: t("extOpenchannel.outboundList.when"),
                  render: (row) => formatDateTime(row.created_at, locale, zone),
                },
                {
                  key: "why",
                  header: t("extOpenchannel.outboundList.why"),
                  render: (row) =>
                    row.error_class
                      ? t(`extOpenchannel.error.${row.error_class}`)
                      : "",
                },
              ]}
            />
          ) : (
            <EmptyState>
              <p className="t-small">
                {t("extOpenchannel.outboundList.empty")}
              </p>
            </EmptyState>
          )}
        </QueryStates>
      ) : (
        <p>{t("extOpenchannel.outboundList.noGrant")}</p>
      )}
    </>
  );
}
