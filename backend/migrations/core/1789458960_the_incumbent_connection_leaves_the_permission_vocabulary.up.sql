-- The incumbent connection is no longer an object any surface admits on, so a
-- role document that still lists it grants authority over nothing. Removed from
-- the seeded roles only: an operator-defined role is theirs to hold whatever it
-- holds, and a key the gate never reads is inert either way.
UPDATE role SET permissions = jsonb_set(permissions, '{objects}',
  (permissions -> 'objects') - 'overlay_connection')
WHERE is_system AND key IN ('admin','ops','management','manager','rep','read_only')
  AND permissions ? 'objects' AND (permissions -> 'objects') ? 'overlay_connection';
