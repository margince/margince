SET LOCAL lock_timeout = '3s';
UPDATE role SET name = 'Member', updated_at = now()
 WHERE key = 'rep' AND is_system AND name = 'User';
