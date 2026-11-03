import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { AppLink } from "../design-system/applink";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { providerName } from "../format/connectorprovider";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import { CAPTURE_CONCERNS } from "../lib/captureconcern";
import { useConnectors } from "../lib/connectors";
import { problemMessageOf, unwrap } from "../lib/problem";
import { briefKey, magicKey, worklistKey } from "../lib/workkeys";

type Connection = components["schemas"]["CaptureConnection"];

export function CaptureHealthBanner() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const client = useQueryClient();
  const query = useConnectors({ refreshHealth: true });
  const dismiss = useMutation({
    mutationFn: async (connection: Connection) => {
      if (!connection.concern_since)
        throw new Error(t("captureConcern.changed"));
      return unwrap(
        await api.POST("/connectors/{provider}/silence/dismiss", {
          params: { path: { provider: connection.provider } },
          body: { since: connection.concern_since },
        }),
      );
    },
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["connectors"] }),
        client.invalidateQueries({ queryKey: magicKey }),
        client.invalidateQueries({ queryKey: worklistKey }),
        client.invalidateQueries({ queryKey: briefKey }),
        client.invalidateQueries({ queryKey: ["capture-health"] }),
        client.invalidateQueries({ queryKey: ["system-health"] }),
      ]);
    },
  });
  const concerns = (query.data?.data ?? []).filter(
    (row) => row.concern && Object.hasOwn(CAPTURE_CONCERNS, row.concern),
  );
  const order = Object.keys(CAPTURE_CONCERNS);
  concerns.sort(
    (a, b) =>
      order.indexOf(a.concern ?? "") - order.indexOf(b.concern ?? "") ||
      (a.id === b.id ? 0 : a.id < b.id ? -1 : 1),
  );
  const row = concerns[0];
  if (query.isError || !row?.concern) return null;
  const label = row.account_label || providerName(row.provider, t);
  const since = row.concern_since
    ? formatDateTime(row.concern_since, locale, viewerZone())
    : "";
  return (
    <div className="appbanner">
      <Callout
        tone="warning"
        kind="standing"
        title={t("captureConcern.title", {
          mailbox: label,
          concern: t(CAPTURE_CONCERNS[row.concern]),
        })}
        actions={
          <AppLink href="#/settings/connections">
            {t(
              row.concern === "reauth_required"
                ? "captureConcern.reconnect"
                : "captureConcern.open",
            )}
          </AppLink>
        }
        dismiss={
          row.concern === "capture_silent" &&
          row.concern_since &&
          !dismiss.isPending
            ? {
                label: t("captureConcern.dismiss"),
                onDismiss: () => dismiss.mutate(row),
              }
            : undefined
        }
      >
        {since && <p>{t("captureConcern.since", { since })}</p>}
        {concerns.length > 1 && (
          <p>
            {plural("captureConcern.more", concerns.length - 1, {
              count: formatNumber(concerns.length - 1, locale),
            })}
          </p>
        )}
        {dismiss.isError && (
          <ErrorLine>{problemMessageOf(dismiss.error, t)}</ErrorLine>
        )}
      </Callout>
    </div>
  );
}
