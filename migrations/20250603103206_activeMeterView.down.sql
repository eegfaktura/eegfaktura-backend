-- reverse: create "metering_partition_factor" table
DROP TABLE "base"."metering_partition_factor";
-- reverse: create index "idx_unique_meteringpoint_active" to table: "meteringpoint"
DROP INDEX "base"."idx_unique_meteringpoint_active";
-- reverse: create "meteringpoint" table
DROP TABLE "base"."meteringpoint";
-- reverse: create "contactdetail" table
DROP TABLE "base"."contactdetail";
-- reverse: create "bankaccount" table
DROP TABLE "base"."bankaccount";
-- reverse: create "address" table
DROP TABLE "base"."address";
-- reverse: create index "idx_tariff" to table: "tariff"
DROP INDEX "base"."idx_tariff";
-- reverse: create "tariff" table
DROP TABLE "base"."tariff";
-- reverse: create "processhistory" table
DROP TABLE "base"."processhistory";
-- reverse: create index "idx_unique_participant_tenant" to table: "participant"
DROP INDEX "base"."idx_unique_participant_tenant";
-- reverse: create "participant" table
DROP TABLE "base"."participant";
-- reverse: create "notification" table
DROP TABLE "base"."notification";
-- reverse: create "gridoperators" table
DROP TABLE "base"."gridoperators";
-- reverse: create index "idx_unique_eeg" to table: "eeg"
DROP INDEX "base"."idx_unique_eeg";
-- reverse: create "eeg" table
DROP TABLE "base"."eeg";
-- reverse: Add new schema named "base"
DROP SCHEMA "base" CASCADE;
