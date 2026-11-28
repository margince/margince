// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { problemCodeOf, problemStatus, unwrap } from "../../lib/problem";
import { channelConnectionsNotConfigured } from "./channelconfiguration";

export type BusinessProvider = "whatsapp" | "zalo-oa";
export type BusinessAccount = Pick<
  components["schemas"]["ZaloOAConnection"],
  "id" | "status" | "readerTeamId" | "mailVisibility"
> & { name: string };
export type BusinessDraft =
  components["schemas"]["CreateWhatsAppConnectionRequest"] &
    components["schemas"]["CreateZaloOAConnectionRequest"];
export const blankBusinessDraft: BusinessDraft = {
  readerTeamId: "",
  wabaId: "",
  phoneNumberId: "",
  displayPhoneNumber: "",
  accessToken: "",
  appSecret: "",
  oaId: "",
  appId: "",
  displayName: "",
  oaSecretKey: "",
  refreshToken: "",
};
export const businessTitles: Record<BusinessProvider, MessageKey> = {
  whatsapp: "connectors.business.whatsapp",
  "zalo-oa": "connectors.business.zalo",
};
type CredentialField = {
  key: keyof BusinessDraft;
  label: MessageKey;
  secret?: boolean;
};
const access: CredentialField = {
  key: "accessToken",
  label: "connectors.business.accessToken",
  secret: true,
};
const appSecret: CredentialField = {
  key: "appSecret",
  label: "connectors.business.appSecret",
  secret: true,
};
export const businessFields: Record<
  BusinessProvider,
  readonly CredentialField[]
> = {
  whatsapp: [
    { key: "wabaId", label: "connectors.business.wabaId" },
    { key: "phoneNumberId", label: "connectors.business.phoneNumberId" },
    { key: "displayPhoneNumber", label: "connectors.business.phone" },
    access,
    appSecret,
  ],
  "zalo-oa": [
    { key: "oaId", label: "connectors.business.oaId" },
    { key: "appId", label: "connectors.business.appId" },
    { key: "displayName", label: "connectors.business.name" },
    access,
    {
      key: "refreshToken",
      label: "connectors.business.refreshToken",
      secret: true,
    },
    appSecret,
    {
      key: "oaSecretKey",
      label: "connectors.business.oaSecretKey",
      secret: true,
    },
  ],
};

export async function listBusinessAccounts(
  provider: BusinessProvider,
): Promise<{ notConfigured: boolean; data: BusinessAccount[] }> {
  try {
    return { notConfigured: false, data: await readBusinessAccounts(provider) };
  } catch (error) {
    if (
      channelConnectionsNotConfigured(
        problemStatus(error),
        problemCodeOf(error),
      )
    )
      return { notConfigured: true, data: [] };
    throw error;
  }
}

async function readBusinessAccounts(
  provider: BusinessProvider,
): Promise<BusinessAccount[]> {
  if (provider === "whatsapp") {
    return unwrap(await api.GET("/channel-connections/whatsapp")).data.map(
      (row) => ({ ...row, name: row.displayPhoneNumber }),
    );
  }
  return unwrap(await api.GET("/channel-connections/zalo-oa")).data.map(
    (row) => ({ ...row, name: row.displayName }),
  );
}

export async function createBusinessAccount(
  provider: BusinessProvider,
  draft: BusinessDraft,
): Promise<string | null> {
  if (provider === "whatsapp") {
    const {
      readerTeamId,
      wabaId,
      phoneNumberId,
      displayPhoneNumber,
      accessToken,
      appSecret,
    } = draft;
    const created = unwrap(
      await api.POST("/channel-connections/whatsapp", {
        body: {
          readerTeamId,
          wabaId,
          phoneNumberId,
          displayPhoneNumber,
          accessToken,
          appSecret,
        },
      }),
    );
    return created.verifyToken;
  }
  const {
    readerTeamId,
    oaId,
    appId,
    displayName,
    accessToken,
    refreshToken,
    appSecret,
    oaSecretKey,
  } = draft;
  unwrap(
    await api.POST("/channel-connections/zalo-oa", {
      body: {
        readerTeamId,
        oaId,
        appId,
        displayName,
        accessToken,
        refreshToken,
        appSecret,
        oaSecretKey,
      },
    }),
  );
  return null;
}

export async function disconnectBusinessAccount(
  provider: BusinessProvider,
  id: string,
) {
  unwrap(
    await api.DELETE(
      provider === "whatsapp"
        ? "/channel-connections/whatsapp/{id}"
        : "/channel-connections/zalo-oa/{id}",
      {
        params: { path: { id } },
      },
    ),
  );
}

export async function changeBusinessVisibility(
  provider: BusinessProvider,
  id: string,
  mail_visibility: "held" | "shared",
) {
  unwrap(
    await api.PATCH(
      provider === "whatsapp"
        ? "/channel-connections/whatsapp/{id}/visibility"
        : "/channel-connections/zalo-oa/{id}/visibility",
      {
        params: { path: { id } },
        body: { mail_visibility, confirm: true },
      },
    ),
  );
}
