// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ConfirmModal } from "../../design-system/confirmmodal";
import type {
  AccountVendor,
  ConnectionAccount,
} from "../../format/connectorprovider";
import { type Translator, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ConnectAccountInput } from "../../lib/connect-account";
import type { Service } from "../../lib/connect-flow-draft";
import { problemMessageOf, throwProblem } from "../../lib/problem";

// Disconnecting an account, or some of its services, behind one confirm.

type Provider = components["schemas"]["CaptureConnection"]["provider"];

// Disconnecting deletes OUR stored credential and does not revoke the grant at
// the vendor, so the confirm names where a careful reader finishes that.
const REVOKE_NOTE: Record<AccountVendor, MessageKey> = {
  google: "connectors.disconnectBodyGoogleNote",
  microsoft: "connectors.disconnectBodyMicrosoftNote",
};

export function vendorOfAccount(
  account: ConnectionAccount,
): AccountVendor | null {
  return account.key === "google" || account.key === "microsoft"
    ? account.key
    : null;
}

function servicesLabel(services: readonly Service[], t: Translator): string {
  if (services.length === 2) return t("connectors.servicesBoth");
  return services[0] === "calendar"
    ? t("connectors.bringsCalendar")
    : t("connectors.bringsMail");
}

// What one disconnect stops: the services, and what may follow it once they
// are gone (a service the same change ticked).
export type PendingStop = Readonly<{
  services: readonly Service[];
  afterwards: ConnectAccountInput | null;
}>;

export function servicesOf(account: ConnectionAccount): Service[] {
  const services: Service[] = [];
  if (account.mail) services.push("mail");
  if (account.calendar) services.push("calendar");
  return services;
}

/** One confirm for every disconnect: the whole account from the overflow, or
 *  the services "Change what it brings in" unticked. */
export function StopConfirm({
  account,
  title,
  stop,
  onClose,
  onStopped,
}: Readonly<{
  account: ConnectionAccount;
  title: string;
  stop: PendingStop | null;
  onClose: () => void;
  onStopped: (afterwards: ConnectAccountInput | null) => void;
}>) {
  const t = useT();
  const disconnect = useDisconnectServices();
  const vendor = vendorOfAccount(account);
  const services = stop?.services ?? [];
  const whole = services.length === servicesOf(account).length;
  const providers = services
    .map((service) =>
      service === "mail" ? account.mail?.provider : account.calendar?.provider,
    )
    .filter((provider): provider is Provider => provider !== undefined);
  return (
    <ConfirmModal
      open={stop !== null}
      onClose={() => {
        disconnect.reset();
        onClose();
      }}
      title={
        whole
          ? t("connectors.disconnectAccountTitle", { account: title })
          : t("connectors.stopServicesTitle", {
              services: servicesLabel(services, t),
            })
      }
      confirmLabel={t("connectors.disconnect")}
      confirmVariant="danger"
      pending={disconnect.isPending}
      error={disconnect.isError ? problemMessageOf(disconnect.error, t) : null}
      onConfirm={() =>
        disconnect.mutate(providers, {
          onSuccess: () => onStopped(stop?.afterwards ?? null),
        })
      }
    >
      <p>
        {t("connectors.disconnectServicesBody", {
          services: servicesLabel(services, t),
        })}
      </p>
      {vendor && <p className="t-caption">{t(REVOKE_NOTE[vendor])}</p>}
    </ConfirmModal>
  );
}

/** Stops each named service in turn. One failure stops the rest, and the
 *  roster read afterwards says which are still connected. */
function useDisconnectServices() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (providers: readonly Provider[]) => {
      for (const provider of providers) {
        const { error } = await api.POST("/connectors/{provider}/disconnect", {
          params: { path: { provider } },
        });
        if (error) {
          throwProblem(error);
        }
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["connectors"] }),
  });
}
