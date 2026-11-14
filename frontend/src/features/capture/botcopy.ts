// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";

// The bots this build offers to connect. A connection of a provider the server
// adds later has no panel here until this list names it.
export type BotProvider = "telegram" | "dispact";

// The words that differ between the workspace bots. The rest of the panel and
// its form say the same thing for every provider.
export const BOT_COPY: Readonly<
  Record<
    BotProvider,
    Readonly<{
      name: MessageKey;
      title: MessageKey;
      sub: MessageKey;
      connectCta: MessageKey;
      emptyNote: MessageKey;
      disconnectBody: MessageKey;
      modalTitle: MessageKey;
      tokenHint: MessageKey;
    }>
  >
> = {
  telegram: {
    name: "connectors.provTelegram",
    title: "connectors.telegramTitle",
    sub: "connectors.telegramSub",
    connectCta: "connectors.telegramConnectCta",
    emptyNote: "connectors.telegramEmptyNote",
    disconnectBody: "connectors.telegramDisconnectBody",
    modalTitle: "connectors.telegramModalTitle",
    tokenHint: "connectors.telegramBotTokenHint",
  },
  dispact: {
    name: "connectors.bot.provDispact",
    title: "connectors.bot.dispactTitle",
    sub: "connectors.bot.dispactSub",
    connectCta: "connectors.bot.dispactConnectCta",
    emptyNote: "connectors.bot.dispactEmptyNote",
    disconnectBody: "connectors.bot.dispactDisconnectBody",
    modalTitle: "connectors.bot.dispactModalTitle",
    tokenHint: "connectors.bot.dispactTokenHint",
  },
};
