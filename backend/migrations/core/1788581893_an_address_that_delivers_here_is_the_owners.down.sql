SET LOCAL lock_timeout = '3s';

DROP TABLE capture_alias_sighting;

DELETE FROM capture_owner_identity WHERE source = 'delivered_to';

ALTER TABLE capture_owner_identity
    DROP CONSTRAINT capture_owner_identity_source_check;

ALTER TABLE capture_owner_identity
    ADD CONSTRAINT capture_owner_identity_source_check
        CHECK (source IN ('user', 'provider'));
