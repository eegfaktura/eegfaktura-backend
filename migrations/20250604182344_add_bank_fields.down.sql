-- reverse: modify "eeg" table
ALTER TABLE "base"."eeg" DROP COLUMN "creditor_id";
-- reverse: modify "bankaccount" table
ALTER TABLE "base"."bankaccount" DROP COLUMN IF EXISTS "mandate_date",
    DROP COLUMN IF EXISTS "mandate_reference",
    DROP COLUMN IF EXISTS "sepa_direct_debit";
