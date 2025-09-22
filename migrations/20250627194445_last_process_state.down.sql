-- reverse: modify "bankaccount" table
ALTER TABLE "base"."bankaccount" ALTER COLUMN "sepa_direct_debit" SET DEFAULT 'B2B', ALTER COLUMN "mandate_date" TYPE date, ALTER COLUMN "mandate_reference" TYPE character varying;
