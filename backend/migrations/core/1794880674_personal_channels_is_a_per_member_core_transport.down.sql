-- Kept when an activity still names it; the foreign key then refuses the delete.
DELETE FROM channel_provider WHERE provider = 'personal_channels'
   AND NOT EXISTS (SELECT 1 FROM activity WHERE channel_provider = 'personal_channels');
