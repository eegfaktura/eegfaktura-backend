-- modify "meteringpoint" table
ALTER TABLE "base"."meteringpoint" ADD COLUMN IF NOT EXISTS "last_process_state" text NULL;
