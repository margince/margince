// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package telegram

// The two webhook calls, together because they answer one question between
// them: whether this bot has a registration, and clearing it if it does.
//
// They are a pair rather than two utilities. deleteWebhook is idempotent and
// answers ok whether or not anything was there, so it cannot report what it
// repaired — and Telegram refuses getUpdates for two different reasons, a
// registered webhook or another consumer holding the same bot, with the SAME
// error. A caller that clears and then polls successfully has learned nothing
// about which it met unless it asked first.

import "context"

// DeleteWebhook clears any webhook registered against the bot.
//
// drop_pending_updates is deliberately NOT sent: its default is false, and those
// pending updates are the customer's messages — the first poll after a connect is
// meant to collect them.
func (a *httpAPI) DeleteWebhook(ctx context.Context, token string) error {
	return a.call(ctx, token, "deleteWebhook", nil, nil)
}

// WebhookRegistered asks getWebhookInfo whether this bot has one.
//
// The URL is the registration: Telegram answers getWebhookInfo for every bot,
// and reports an EMPTY url for one that has no webhook rather than refusing.
// So an empty url is a definite "none", not a missing answer.
func (a *httpAPI) WebhookRegistered(ctx context.Context, token string) (bool, error) {
	var out struct {
		URL string `json:"url"`
	}
	if err := a.call(ctx, token, "getWebhookInfo", nil, &out); err != nil {
		return false, err
	}
	return out.URL != "", nil
}
