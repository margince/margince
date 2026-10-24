SET LOCAL lock_timeout = '5s';

-- Every role that sends mail may take a recorded exception for one message.
--
-- Directing a send was admin alone (1789048321). The product decision is that
-- a sender is never at a dead end: where the engine cannot establish
-- permission, the sender may send one message with an explanation, a shown
-- warning and a recorded acknowledgement. The exception covers one message and
-- never becomes standing permission. DirectSend still refuses a rep who tries
-- it past a contact's own stop: only an admin may do that.
--
-- Only where the stored value is still exactly the all-false document the
-- earlier migration wrote, so an operator's own hand-set value is kept.
-- ops and read_only stay without the grant: neither sends mail.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,communication_exception}',
        '{"create":true,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('rep', 'manager', 'management')
      AND permissions ? 'objects'
      AND (permissions -> 'objects' -> 'communication_exception')
          = '{"create":false,"read":false,"update":false,"delete":false}'::jsonb;
