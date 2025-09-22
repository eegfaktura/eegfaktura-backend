-- modify "bankaccount" table
ALTER TABLE "base"."bankaccount" ADD COLUMN IF NOT EXISTS "mandate_reference" VARCHAR NULL,
    ADD COLUMN IF NOT EXISTS "mandate_date" DATE NULL DEFAULT now()::DATE,
    ADD COLUMN IF NOT EXISTS "sepa_direct_debit" VARCHAR NULL;
-- modify "eeg" table
ALTER TABLE "base"."eeg" ADD COLUMN IF NOT EXISTS "creditor_id" text NULL;
