-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- A Zalo Official Account's messages and customer identities name this channel.
INSERT INTO channel_provider (provider, transport, label, credential_model, supplies_transport)
VALUES ('zalo_oa', 'core', 'Zalo', 'workspace_bot', false)
ON CONFLICT (provider) DO NOTHING;
