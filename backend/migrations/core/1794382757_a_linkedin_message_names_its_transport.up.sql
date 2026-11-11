SET LOCAL lock_timeout = '3s';
-- LinkedIn joins the registry as a core transport with no sender, as WhatsApp
-- did: a member can log a conversation held there, and nothing replies on it.
-- An installation whose unit already registered `linkedin` keeps that row.
INSERT INTO channel_provider (provider, transport, label, credential_model, supplies_transport)
VALUES ('linkedin', 'core', 'LinkedIn', 'workspace_bot', false)
ON CONFLICT (provider) DO NOTHING;
