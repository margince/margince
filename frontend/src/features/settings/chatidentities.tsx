// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/button";
import { ErrorLine } from "../../design-system/errorline";
import { SettingRow } from "../../design-system/settingrow";
import type { Toast } from "../../design-system/toast";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemMessageOf, unwrap } from "../../lib/problem";

type Network = components["schemas"]["ChatNetwork"];
type Identity = components["schemas"]["ChatIdentity"];

const NETWORKS: readonly {
  network: Network;
  name: string;
  label: MessageKey;
}[] = [
  { network: "whatsapp", name: "WhatsApp", label: "account.chatWhatsapp" },
  { network: "telegram", name: "Telegram", label: "account.chatTelegram" },
  { network: "zalo", name: "Zalo", label: "account.chatZalo" },
];

const QUERY_KEY = ["me", "chat-identities"];

function useChatIdentities() {
  return useQuery({
    queryKey: QUERY_KEY,
    queryFn: async () =>
      unwrap(await api.GET("/me/chat-identities")).data satisfies Identity[],
  });
}

// Shown once, so it lives in the component, not in the query cache.
function useStart(network: Network, onCode: (code: string) => void) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/me/chat-identities/{network}", {
          params: { path: { network } },
        }),
      ),
    onSuccess: async (started) => {
      onCode(started.code);
      await client.invalidateQueries({ queryKey: QUERY_KEY });
    },
  });
}

function useRemove(network: Network, toast: Toast) {
  const t = useT();
  const client = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      unwrap(
        await api.DELETE("/me/chat-identities/{network}", {
          params: { path: { network } },
        }),
      ),
    onSuccess: async () => {
      toast.show(t("account.chatRemoved"));
      await client.invalidateQueries({ queryKey: QUERY_KEY });
    },
  });
}

function ChatIdentityRow({
  network,
  name,
  label,
  identity,
  toast,
}: Readonly<{
  network: Network;
  name: string;
  label: MessageKey;
  identity: Identity | undefined;
  toast: Toast;
}>) {
  const t = useT();
  const [code, setCode] = useState<string | null>(null);
  const start = useStart(network, setCode);
  const remove = useRemove(network, toast);
  const confirmed = identity?.account_hint !== undefined;
  const waiting = code !== null || identity?.expires_at !== undefined;
  const failure = start.error ?? remove.error;
  let description = t("account.chatHelp", { network: name });
  if (code !== null) {
    description = t("account.chatCode", { network: name, code });
  } else if (waiting) {
    description = t("account.chatWaiting", { network: name });
  }
  const verb = confirmed || waiting ? remove : start;
  return (
    <SettingRow
      label={t(label)}
      description={description}
      value={
        confirmed
          ? t("account.chatConfirmed", {
              account: identity?.account_hint ?? "",
            })
          : undefined
      }
      control={
        <div className="form-stack">
          {confirmed || waiting ? (
            <Button
              variant="ghost"
              pending={verb.isPending}
              onClick={() => {
                setCode(null);
                remove.mutate();
              }}
            >
              {t("account.chatRemove")}
            </Button>
          ) : (
            <Button
              variant="ghost"
              pending={start.isPending}
              onClick={() => start.mutate()}
            >
              {t("account.chatConfirm")}
            </Button>
          )}
          <ErrorLine>
            {failure
              ? problemMessageOf(failure, t, t("account.chatFailed"))
              : undefined}
          </ErrorLine>
        </div>
      }
    />
  );
}

/** One row per chat network: confirm your own account, or remove it. */
export function ChatIdentityRows({ toast }: Readonly<{ toast: Toast }>) {
  const query = useChatIdentities();
  return (
    <>
      {NETWORKS.map((entry) => (
        <ChatIdentityRow
          key={entry.network}
          {...entry}
          identity={query.data?.find((row) => row.network === entry.network)}
          toast={toast}
        />
      ))}
    </>
  );
}
