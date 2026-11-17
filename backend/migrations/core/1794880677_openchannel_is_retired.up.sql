-- The openchannel unit is retired: a member's own chats now arrive through the
-- core's personal channels. What it left in an installation goes with it, so no
-- personal data stays behind with no retention or erasure to reach it. Messages
-- still waiting to land and replies not yet sent are dropped.
--
-- IF EXISTS everywhere: a fresh installation never created these.
SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS ext.ext_openchannel_archived_before_landing;
DROP TABLE IF EXISTS ext.ext_openchannel_outbound;
DROP TABLE IF EXISTS ext.ext_openchannel_inbound;
DROP TABLE IF EXISTS ext.ext_openchannel_endpoint;
DROP TABLE IF EXISTS schema_migrations_ext_openchannel;

DELETE FROM extension_secret WHERE extension_name = 'openchannel';
DELETE FROM extension_ingest_position WHERE unit = 'openchannel';
DELETE FROM extension_ingest_refusal WHERE unit = 'openchannel';

UPDATE role SET permissions = permissions
    #- '{objects,ext_openchannel_endpoint}'
    #- '{objects,ext_openchannel_inbound}'
    #- '{objects,ext_openchannel_outbound}'
 WHERE permissions -> 'objects' ?| ARRAY['ext_openchannel_endpoint', 'ext_openchannel_inbound', 'ext_openchannel_outbound'];

-- Kept as a label so its old activities still name their transport; it sends nothing.
UPDATE channel_provider SET supplies_transport = false WHERE provider = 'openchannel';
