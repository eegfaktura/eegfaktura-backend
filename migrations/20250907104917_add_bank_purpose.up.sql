-- modify "eeg" table
ALTER TABLE "base"."eeg" ADD COLUMN IF NOT EXISTS "bankPurpose" text NULL;
