-- Only what this migration wrote, identified by the provenance it stamped.
-- A basis or suppression the running product recorded carries a different note
-- and source, and survives.
DELETE FROM communication_suppression WHERE source = 'carried_from_person_consent';
DELETE FROM communication_basis WHERE note LIKE 'carried from the qualifying event%';
