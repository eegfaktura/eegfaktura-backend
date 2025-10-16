CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE SCHEMA IF NOT EXISTS base;

CREATE TABLE IF NOT EXISTS base.EEG
(
    tenant               VARCHAR PRIMARY KEY,
    name                 TEXT    NOT NULL,
    description          VARCHAR(100),
    periods              JSON             DEFAULT ('[]'),
    "rcNumber"           TEXT    NOT NULL,
    area                 TEXT    NOT NULL, /* Ortsgebiet (LOCAL | REGIONAL) | BEG | GEA */
    legal                TEXT    NOT NULL DEFAULT 'verein', /* Unternehmensform ("verein" | "genossenschaft" | "gesellschaft") */
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
    creditor_id          TEXT,
    bic                  TEXT,
    "bankPurpose"        TEXT,
    -- Contact Info
    phone                TEXT,
    email                TEXT    NOT NULL,
    website              TEXT,

    online               BOOLEAN NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_eeg ON base.EEG (tenant, name, "rcNumber");

CREATE TABLE IF NOT EXISTS "base"."tariff"
(
    id                     uuid             DEFAULT public.uuid_generate_v4() NOT NULL,
    tenant                 varchar                                            NOT NULL,
    type                   varchar                                            NOT NULL,
    name                   text                                               NOT NULL,
    "billingPeriod"        text             DEFAULT 'monthly'::text,
    "useVat"               boolean          DEFAULT false,
    "vatSupplementaryText" text             DEFAULT ''::text                  NOT NULL,
    "vatInPercent"         numeric          DEFAULT 0                         NOT NULL,
    "accountNetAmount"     numeric          DEFAULT 0,
    "accountGrossAmount"   numeric          DEFAULT 0,
    "participantFee"       real             DEFAULT 0                         NOT NULL,
    "baseFee"              double precision DEFAULT 0                         NOT NULL,
    "freeKWh"              integer          DEFAULT 0,
    "businessNr"           integer,
    "createdBy"            text,
    "createdDate"          date             DEFAULT now()::date,
    "lastModifiedDate"     date             DEFAULT now()::date,
    version                integer                                            NOT NULL,
    "centPerKWh"           double precision DEFAULT 0,
    discount               integer          DEFAULT 0,
    status                 text             DEFAULT 'ACTIVE'::text            NOT NULL,
    "inactiveSince"        date,
    "meteringPointFee"     double precision,
    "meteringPointVat"     integer,
    "useMeteringPointFee"  boolean          DEFAULT false                     NOT NULL,
    CONSTRAINT tariffpk PRIMARY KEY (id, version)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tariff ON base.tariff (id, tenant, name, type, version);

CREATE TABLE IF NOT EXISTS base.participant
(
    id                      UUID    NOT NULL DEFAULT uuid_generate_v4(),
    "participantNumber"     VARCHAR,
    tenant                  VARCHAR NOT NULL,
    firstname               VARCHAR NOT NULL,
    lastname                VARCHAR NOT NULL DEFAULT ''::character varying,
    role                    VARCHAR NOT NULL DEFAULT 'EEG_USER'::character varying,    /* 'EEG_USER' | 'EEG_ADMIN' */
    "businessRole"          VARCHAR NOT NULL DEFAULT 'EEG_PRIVATE'::character varying, /* 'EEG_PRIVATE' | 'EEG_BUSINESS' */
    "titleBefore"           VARCHAR,
    "titleAfter"            VARCHAR,
    "participantSince"      DATE    NOT NULL DEFAULT (now())::date,
    "vatNumber"             VARCHAR,
    "taxNumber"             VARCHAR,
    "companyRegisterNumber" VARCHAR,
    status                  VARCHAR NOT NULL DEFAULT 'NEW'::character varying,         /* 'NEW' | 'PENDING' | 'ACCEPTED' | 'ACTIVE' | 'INACTIVE' | 'ARCHIVED' | 'REJECTED' */
    "createdBy"             VARCHAR NOT NULL,
    "createdDate"           DATE             DEFAULT (now())::date,
    "lastModifiedBy"        VARCHAR NOT NULL,
    "lastModifiedDate"      DATE             DEFAULT (now())::date,
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
    mandate_reference VARCHAR,
    mandate_date   DATE DEFAULT now()::DATE,
    sepa_direct_debit VARCHAR,
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
    last_process_state TEXT,
    "statusCode"       INTEGER,
    tariff_id          UUID,
    inverterid         TEXT,
    "equipmentNumber"  TEXT,
    "equipmentName"    TEXT,
    street             TEXT,
    "streetNumber"     TEXT,
    city               TEXT,
    zip                TEXT,
    "registeredSince"  DATE      NOT NULL DEFAULT (now())::date,
    "modifiedAt"       TIMESTAMP NOT NULL DEFAULT (now())::date,
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
    "createdAt"       DATE    NOT NULL DEFAULT (now())::date,
    "createdBy"       VARCHAR NOT NULL,
    CONSTRAINT meteringpointPartitionPK PRIMARY KEY (metering_point_id, version),
    CONSTRAINT FK_MeteringpointPartition FOREIGN KEY (metering_point_id, tenant, participant_id)
        REFERENCES base.meteringpoint (metering_point_id, tenant, participant_id) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE TABLE IF NOT EXISTS base.notification
(
    id           SERIAL PRIMARY KEY,
    tenant       TEXT      NOT NULL,
    type         TEXT      NOT NULL DEFAULT 'MESSAGE',/* MESSAGE TYPE DESCRIBE 'ERROR' | 'MESSAGE' | 'NOTIFICATION' */
    process      TEXT      NOT NULL DEFAULT 'EDA_PROCESS',
    notification json      NOT NULL DEFAULT '{}',
    date         TIMESTAMP NOT NULL DEFAULT (now())::date,
    role         VARCHAR   NOT NULL DEFAULT 'ADMIN' /* 'USER' | 'ADMIN' */
);

CREATE TABLE IF NOT EXISTS base.processhistory
(
    id               UUID      NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant           TEXT      NOT NULL,
    "conversationId" TEXT      NOT NULL,
    type             TEXT      NOT NULL,
    date             TIMESTAMP NOT NULL             DEFAULT (now())::date,
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
SELECT partp.metering_point_id,
       partp.version,
       partp.participant_id,
       partp.tenant,
       partp."partFact",
       partp."createdAt",
       partp."createdBy",
       partp.rowid
FROM (SELECT metering_partition_factor.metering_point_id,
             metering_partition_factor.version,
             metering_partition_factor.participant_id,
             metering_partition_factor.tenant,
             metering_partition_factor."partFact",
             metering_partition_factor."createdAt",
             metering_partition_factor."createdBy",
             row_number()
             OVER (PARTITION BY metering_partition_factor.metering_point_id, metering_partition_factor.participant_id ORDER BY metering_partition_factor.version DESC) AS rowid
      FROM base.metering_partition_factor) partp
WHERE partp.rowid = 1;

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
       COALESCE(b.mandate_reference, p."participantNumber")    AS participant_sepa_mandate_reference,
       COALESCE(b.mandate_date, p."participantSince")          AS participant_sepa_mandate_issue_date,
       b.sepa_direct_debit                                     AS participant_sepa_direct_debit,
       pm.metering_point_id,
       pm."equipmentNumber"                                    AS equipment_number,
       pm."equipmentName"                                      AS metering_equipment_name,
       CASE
           WHEN pm.direction = 'GENERATION'::text THEN 0
           ELSE 1
           END                                                 AS metering_point_type,
       c.tenant                                                AS tenant_id,
       c."rcNumber"                                            AS eec_id,
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
       o.email                                                 AS participant_email,
       COALESCE(c."bankName", ''::text)                        AS eec_bank_name,
       c.iban                                                  AS eec_bank_iban,
       c.owner                                                 AS eec_bank_owner,
       c.creditor_id                                           AS eec_bank_creditor_id
FROM base.participant p
         LEFT JOIN base.eeg c ON c.tenant::text = p.tenant::text
         LEFT JOIN base.meteringpoint pm ON pm.participant_id = p.id
         LEFT JOIN base.address p_address ON p.id = p_address.participant_id AND p_address.type = 'BILLING'::text
         LEFT JOIN base.activetariff t ON t.id = pm.tariff_id
         LEFT JOIN base.activetariff tp ON tp.id = p."tariffId" AND tp.type::text = 'EEG'::text
         LEFT JOIN base.bankaccount b ON b.participant_id = p.id
         LEFT JOIN base.contactdetail o ON o.participant_id = p.id
