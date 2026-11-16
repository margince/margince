-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- A registered unit keeps its transport ownership and sending authority.
INSERT INTO channel_provider (provider, transport, label, credential_model, supplies_transport)
VALUES ('linkedin', 'core', 'LinkedIn', 'workspace_bot', false)
ON CONFLICT (provider) DO NOTHING;
