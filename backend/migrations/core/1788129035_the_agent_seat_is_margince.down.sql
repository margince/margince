SET LOCAL lock_timeout = '3s';
UPDATE app_user SET display_name = 'Gradion Agent', updated_at = now()
 WHERE is_agent AND display_name = 'Margince Agent';
