-- A member's own chats arrive on one core transport whose credential is that member's.
INSERT INTO channel_provider (provider, transport, label, credential_model, supplies_transport)
VALUES ('personal_channels', 'core', 'Personal channels', 'per_member', true)
ON CONFLICT (provider) DO UPDATE
   SET transport = EXCLUDED.transport, label = EXCLUDED.label,
       credential_model = EXCLUDED.credential_model, supplies_transport = EXCLUDED.supplies_transport;
