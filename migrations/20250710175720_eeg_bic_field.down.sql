-- reverse: modify "eeg" table
ALTER TABLE "base"."eeg" DROP COLUMN "bic";
-- reverse: modify "bankaccount" table
ALTER TABLE "base"."bankaccount" ALTER COLUMN "mandate_date" DROP DEFAULT;
