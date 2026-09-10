SET LOCAL lock_timeout = '3s';

DROP TRIGGER IF EXISTS communication_instruction_refuse_rewrite ON communication_instruction;
DROP FUNCTION IF EXISTS communication_instruction_refuse_rewrite();
DROP TABLE IF EXISTS communication_instruction CASCADE;
