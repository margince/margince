SET LOCAL lock_timeout = '3s';

DROP TRIGGER address_is_stored_lowercase ON activity_participant;
DROP FUNCTION store_participant_address_lowercase();
ALTER TABLE activity_participant DROP CONSTRAINT activity_participant_address_is_canonical;
