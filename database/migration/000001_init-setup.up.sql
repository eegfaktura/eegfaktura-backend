CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE SCHEMA IF NOT EXISTS base;

CREATE TABLE IF NOT EXISTS base.EEG
(
    tenant               VARCHAR PRIMARY KEY,
    name                 TEXT    NOT NULL,
    description          VARCHAR(40),
    periods              JSON             DEFAULT ('[]'),
    "rcNumber"           TEXT    NOT NULL,
    area                 TEXT    NOT NULL, /* Ortsgebiet (LOCAL | REGIONAL) | BEG | GEA */
    legal                TEXT    NOT NULL DEFAULT 'verein', /* Unternehmensform ("verein" | "genossenschaft" | "geselschaft") */
    gridoperator_code    TEXT    NOT NULL,
    gridoperator_name    TEXT    NOT NULL,
    "communityId"        TEXT    NOT NULL,
    "businessNr"         TEXT,
    "allocationMode"     TEXT    NOT NULL DEFAULT 'DYNAMIC', /* "DYNAMIC" | "STATIC" */
    "settlementInterval" TEXT    NOT NULL DEFAULT 'MONTHLY', /* "MONTHLY" | "QUARTER" | "BIANNUAL" | "ANNUAL" */
    "providerBusinessNr" INTEGER,
    "taxNumber"          TEXT,
    "vatNumber"          TEXT,
    subjecttovat         BOOLEAN,
    "contactPerson"      TEXT,
    createdat            DATE NOT NULL DEFAULT now()::DATE,
    -- Address Info
    street               TEXT    NOT NULL,
    "streetNumber"       TEXT    NOT NULL,
    city                 TEXT    NOT NULL,
    zip                  TEXT    NOT NULL,
    -- Account Info
    iban                 TEXT,
    owner                TEXT,
    sepa                 BOOLEAN NOT NULL DEFAULT false,
    "bankName"           TEXT,
    -- Contact Info
    phone                TEXT,
    email                TEXT    NOT NULL,
    website              TEXT,

    online               BOOLEAN NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_eeg ON base.EEG (tenant, name, "rcNumber");

CREATE TABLE IF NOT EXISTS base.tariff
(
    id                     UUID    NOT NULL DEFAULT uuid_generate_v4(),
    tenant                 VARCHAR NOT NULL,
    type                   VARCHAR NOT NULL, /* 'tariff type like EEG, VZP, EZP, AKONTO' */
    name                   TEXT    NOT NULL,
    "billingPeriod"        TEXT             DEFAULT 'monthly',
    "useVat"               BOOLEAN          DEFAULT FALSE,
    "vatSupplementaryText" TEXT    NOT NULL DEFAULT '',
    "vatInPercent"         NUMERIC NOT NULL DEFAULT 0,
    "accountNetAmount"     NUMERIC,
    "accountGrossAmount"   NUMERIC,
    "participantFee"       FLOAT   NOT NULL DEFAULT 0,
    "baseFee"              FLOAT   NOT NULL DEFAULT 0,
    "freeKWh"              INTEGER,
    "businessNr"           INTEGER,
    "createdBy"            TEXT,
    "createdDate"          DATE             DEFAULT now(),
    "lastModifiedDate"     DATE             DEFAULT now(),
    version                INTEGER,
    "centPerKWh"           FLOAT            DEFAULT 0,
    discount               INTEGER,
    status                 TEXT    NOT NULL DEFAULT 'ACTIVE', /* ACTIVE | INACTIVE */
    "inactiveSince"        DATE,
    "meteringPointFee"     FLOAT,
    "meteringPointVat"     INTEGER,
    "useMeteringPointFee"  BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT TariffPK PRIMARY KEY (id, version)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tariff ON base.tariff (id, tenant, name, type, version);

CREATE TABLE IF NOT EXISTS base.participant
(
    id                      UUID    NOT NULL DEFAULT uuid_generate_v4(),
    "participantNumber"     VARCHAR,
    tenant                  VARCHAR NOT NULL,
    firstname               VARCHAR NOT NULL,
    lastname                VARCHAR NOT NULL,
    role                    VARCHAR NOT NULL DEFAULT 'EEG_USER', /* 'EEG_USER' | 'EEG_ADMIN' */
    "businessRole"          VARCHAR NOT NULL DEFAULT 'EEG_PRIVATE', /* 'EEG_PRIVATE' | 'EEG_BUSINESS' */
    "titleBefore"           VARCHAR,
    "titleAfter"            VARCHAR,
    "participantSince"      DATE    NOT NULL DEFAULT now(),
    "vatNumber"             VARCHAR,
    "taxNumber"             VARCHAR,
    "companyRegisterNumber" VARCHAR,
    status                  VARCHAR NOT NULL DEFAULT 'NEW', /* 'NEW' | 'PENDING' | 'ACCEPTED' | 'ACTIVE' | 'INACTIVE' | 'ARCHIVED' | 'REJECTED' */
    "createdBy"             VARCHAR NOT NULL,
    "createdDate"           DATE             DEFAULT now(),
    "lastModifiedBy"        VARCHAR NOT NULL,
    "lastModifiedDate"      DATE             DEFAULT now(),
    version                 INTEGER          DEFAULT 1,
    "tariffId"              uuid,
    CONSTRAINT ParticipantPK PRIMARY KEY (id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_participant_tenant ON base.participant (id, tenant, version);

CREATE TABLE IF NOT EXISTS base.contactdetail
(
    id             UUID NOT NULL DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL,
    email          TEXT,
    phone          TEXT,
    CONSTRAINT contactdetailsPK PRIMARY KEY (id),
    CONSTRAINT FK_ParticipantDetail FOREIGN KEY (participant_id) REFERENCES base.participant (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS base.address
(
    id             UUID NOT NULL DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL,
    type           TEXT NOT NULL DEFAULT 'RESIDENCE', /*Address-Types: 'RESIDENCE' | 'BILLING' */
    street         TEXT,
    "streetNumber" TEXT,
    city           TEXT,
    zip            TEXT,
    CONSTRAINT addressPK PRIMARY KEY (id),
    CONSTRAINT FK_ParticipantAddress FOREIGN KEY (participant_id) REFERENCES base.participant (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS base.bankaccount
(
    id             UUID NOT NULL DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL,
    iban           TEXT,
    owner          TEXT,
    "bankName"     TEXT,
    CONSTRAINT bankaccountPK PRIMARY KEY (id),
    CONSTRAINT FK_ParticipantBankaccount FOREIGN KEY (participant_id) REFERENCES base.participant (id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS base.meteringpoint
(
    metering_point_id  TEXT      NOT NULL,
    consent_id         TEXT,
    participant_id     UUID      NOT NULL,
    tenant             TEXT      NOT NULL,
    grid_operator_name VARCHAR,
    grid_operator_id   VARCHAR,
    allocation_factor  FLOAT,
    transformer        TEXT,
    direction          TEXT      NOT NULL DEFAULT 'CONSUMPTION', /* 'GENERATION' | 'CONSUMPTION' */
    status             TEXT      NOT NULL DEFAULT 'INIT', /* "INIT" | "ACTIVE" | "INACTIVE" */
    process_state      TEXT      NOT NULL DEFAULT 'NEW', /* "NEW" | "PENDING" | "ACCEPTED" | "ACTIVE" | "INACTIVE" | "REJECTED" */
    "statusCode"       INTEGER,
    tariff_id          UUID,
    inverterid         TEXT,
    "equipmentNumber"  TEXT,
    "equipmentName"    TEXT,
    street             TEXT,
    "streetNumber"     TEXT,
    city               TEXT,
    zip                TEXT,
    "registeredSince"  DATE      NOT NULL DEFAULT now()::date,
    "modifiedAt"       TIMESTAMP NOT NULL DEFAULT now(),
    "modifiedBy"       TEXT,
    activeSince        DATE,
    inactiveSince      DATE,
    active             INT       NOT NULL DEFAULT 1,
    flag               INT       NOT NULL DEFAULT 1,
    CONSTRAINT meteringpointPK PRIMARY KEY (metering_point_id, tenant, participant_id),
    CONSTRAINT FK_ParticipantMeteringpoint FOREIGN KEY (participant_id) REFERENCES base.participant (id) ON DELETE CASCADE
--    UNIQUE (metering_point_id, active)
--     CONSTRAINT FK_TariffMeteringpoint FOREIGN KEY (tariff_id) REFERENCES base.tariff (id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_meteringpoint_active ON base.meteringpoint (metering_point_id, tenant, flag) where flag = 1;

CREATE TABLE IF NOT EXISTS base.metering_partition_factor
(
    metering_point_id TEXT    NOT NULL,
    version           SERIAL,
    participant_id    UUID    NOT NULL,
    tenant            TEXT    NOT NULL,
    "partFact"        INTEGER NOT NULL,
    "createdAt"       DATE    NOT NULL DEFAULT now(),
    "createdBy"       VARCHAR NOT NULL,
    CONSTRAINT meteringpointPartitionPK PRIMARY KEY (metering_point_id, version),
    CONSTRAINT FK_MeteringpointPartition FOREIGN KEY (metering_point_id, tenant, participant_id)
        REFERENCES base.meteringpoint (metering_point_id, tenant, participant_id) ON DELETE CASCADE ON UPDATE CASCADE
);
-- CREATE TABLE IF NOT EXISTS base.participant_meter_state
-- (
--     participant_id UUID      NOT NULL,
--     tenant         TEXT      NOT NULL,
--     metering_point TEXT      NOT NULL,
--     activeSince    TIMESTAMP NOT NULL DEFAULT now(),
--     inactiveSince  TIMESTAMP NOT NULL DEFAULT Date('2999-12-31'),
--     changed_at     TIMESTAMP NOT NULL DEFAULT now(),
--     changed_by     TEXT      NOT NULL,
--     flag           INT       NOT NULL DEFAULT 1,
--     active         INT       NOT NULL DEFAULT 1,
--     CONSTRAINT PK_Participant_meter_state PRIMARY KEY (metering_point, tenant, active),
--     CONSTRAINT FK_Participant_state FOREIGN KEY (participant_id) REFERENCES base.participant (id) ON DELETE CASCADE,
--     CONSTRAINT FK_Metering_state FOREIGN KEY (metering_point, tenant) REFERENCES base.meteringpoint (metering_point_id, tenant) ON DELETE CASCADE
-- );

CREATE TABLE IF NOT EXISTS base.notification
(
    id           SERIAL PRIMARY KEY,
    tenant       TEXT      NOT NULL,
    type         TEXT      NOT NULL DEFAULT 'MESSAGE',/* MESSAGE TYPE DESCRIBE 'ERROR' | 'MESSAGE' | 'NOTIFICATION' */
    process      TEXT      NOT NULL DEFAULT 'EDA_PROCESS',
    notification json      NOT NULL DEFAULT '{}',
    date         TIMESTAMP NOT NULL DEFAULT now(),
    role         VARCHAR   NOT NULL DEFAULT 'ADMIN' /* 'USER' | 'ADMIN' */
);

CREATE TABLE IF NOT EXISTS base.processhistory
(
    id               UUID      NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant           TEXT      NOT NULL,
    "conversationId" TEXT      NOT NULL,
    type             TEXT      NOT NULL,
    date             TIMESTAMP NOT NULL             DEFAULT now(),
    issuer           TEXT      NOT NULL,
    message          json      NOT NULL             DEFAULT '{}',
    direction        TEXT      NOT NULL             DEFAULT 'OUT', /* MESSAGE DIRECTION 'OUT' | 'IN' */
    protocol         VARCHAR
);

CREATE TABLE IF NOT EXISTS base.gridoperators
(
    id   VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    CONSTRAINT PK_GridOperators PRIMARY KEY (id, name)
);

CREATE OR REPLACE VIEW base.activeMeteringPartition AS
SELECT * FROM (
    SELECT *, ROW_NUMBER() OVER (
    PARTITION BY "metering_point_id", "participant_id"
    ORDER BY
     version DESC
    ) AS rowid
    FROM base.metering_partition_factor) AS partp WHERE rowid = 1;

CREATE OR REPLACE VIEW base.activeTariff AS
SELECT id,
       name,
       tenant,
       "billingPeriod",
       "useVat",
       "vatInPercent",
       "vatSupplementaryText",
       "accountNetAmount",
       "accountGrossAmount",
       "participantFee",
       "baseFee",
       "businessNr",
       version,
       type,
       "centPerKWh",
       discount,
       "freeKWh",
       "meteringPointFee",
       "meteringPointVat",
       "useMeteringPointFee"
FROM base.tariff,
     (SELECT id as tid, MAX(version) as tversion FROM base.tariff GROUP BY id) as x
WHERE id = x.tid
  AND version = x.tversion
  AND status != 'ARCHIVED';

CREATE OR REPLACE VIEW
    base.billing_masterdata AS
SELECT p.id                                                    AS participant_id,
       p."titleBefore"                                         AS participant_title_before,
       p.firstname                                             AS participant_firstname,
       p."participantNumber"                                   AS participant_number,
       p.lastname                                              AS participant_lastname,
       p."titleAfter"                                          AS participant_title_after,
       p."vatNumber"                                           AS participant_vat_id,
       p."taxNumber"                                           AS participant_tax_id,
       p."companyRegisterNumber"                               AS participant_company_register_number,
       p."participantNumber"                                   AS participant_sepa_mandate_reference,
       p."participantSince"                                    AS participant_sepa_mandate_issue_date,
       pm.metering_point_id,
       pm."equipmentNumber"                                    AS equipment_number,
       pm."equipmentName"                                      AS metering_equipment_name,
       CASE
           WHEN pm.direction = 'GENERATION'::text THEN 0
           ELSE 1
           END                                                 AS metering_point_type,
       c.tenant                                                AS eec_id,
       c."rcNumber"                                            AS tenant_id,
       c.description                                           AS eec_name,
       c."vatNumber"                                           AS eec_vat_id,
       c."taxNumber"                                           AS eec_tax_id,
       c."businessNr"                                          AS eec_company_register_number,
       c.subjecttovat                                          AS eec_subject_to_vat,
       c.phone                                                 AS eec_phone,
       c.email                                                 AS eec_email,
       c.website                                               AS eec_website,
       concat(c.street, ' ', c."streetNumber")                 AS eec_street,
       c.zip                                                   AS eec_zip_code,
       c.city                                                  AS eec_city,
       concat(p_address.street, ' ', p_address."streetNumber") AS participant_street,
       p_address.zip                                           AS participant_zip_code,
       p_address.city                                          AS participant_city,
       t.type                                                  AS tariff_type,
       t.name                                                  AS tariff_name,
       t."billingPeriod"                                       AS tariff_billing_period,
       t."useVat"                                              AS tariff_use_vat,
       t."vatSupplementaryText"                                AS tariff_text,
       t."vatInPercent"                                        AS tariff_vat_in_percent,
       t."useMeteringPointFee"                                 AS tariff_use_metering_point_fee,
       t."meteringPointFee"                                    AS tariff_metering_point_fee,
       t."meteringPointVat"                                    AS tariff_metering_point_vat,
       ''::text                                                AS tariff_metering_point_fee_text,
       ''::text                                                AS tariff_participant_fee_text,
       COALESCE(tp.version, 0)                                 AS tariff_participant_version,
       COALESCE(tp."participantFee", 0::double precision)      AS tariff_participant_fee,
       COALESCE(tp.name, ''::text)                             AS tariff_participant_fee_name,
       COALESCE(tp."useVat", false)                            AS tariff_participant_fee_use_vat,
       COALESCE(tp."vatInPercent", 0::numeric)                 AS tariff_participant_fee_vat_in_percent,
       COALESCE(tp.discount, 0)                                AS tariff_participant_fee_discount,
       t."baseFee"                                             AS tariff_basic_fee,
       t.discount                                              AS tariff_discount,
       t."centPerKWh"                                          AS tariff_working_fee_per_consumedkwh,
       t."centPerKWh"                                          AS tariff_credit_amount_per_producedkwh,
       t."freeKWh"                                             AS tariff_freekwh,
       t.version                                               AS tariff_version,
       t.id                                                    AS tariff_id,
       COALESCE(b."bankName", ''::text)                        AS participant_bank_name,
       b.iban                                                  AS participant_bank_iban,
       b.owner                                                 AS participant_bank_owner,
       split_part(o.email, ';'::text, 1)                       AS participant_email,
       COALESCE(c."bankName", ''::text)                        AS eec_bank_name,
       c.iban                                                  AS eec_bank_iban,
       c.owner                                                 AS eec_bank_owner
FROM base.participant p
         LEFT JOIN base.eeg c ON c.tenant::text = p.tenant::text
         LEFT JOIN base.meteringpoint pm ON pm.participant_id = p.id
         LEFT JOIN base.address p_address ON p.id = p_address.participant_id AND p_address.type = 'BILLING'::text
         LEFT JOIN base.activetariff t ON t.id = pm.tariff_id
         LEFT JOIN base.activetariff tp ON tp.id = p."tariffId" AND tp.type::text = 'EEG'::text
         LEFT JOIN base.bankaccount b ON b.participant_id = p.id
         LEFT JOIN base.contactdetail o ON o.participant_id = p.id