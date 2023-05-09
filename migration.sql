CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE SCHEMA IF NOT EXISTS base;

CREATE TABLE IF NOT EXISTS base.EEG
(
    tenant             VARCHAR PRIMARY KEY,
    name               TEXT    NOT NULL,
    description        TEXT,
    periods            JSON             DEFAULT ('[]'),
    rcNumber           TEXT    NOT NULL,
    area               TEXT    NOT NULL, /* Ortsgebiet (LOCAL | REGIONAL) */
    legal              TEXT    NOT NULL, /* Unternehmensform*/
    gridoperator_code  TEXT    NOT NULL,
    gridoperator_name  TEXT    NOT NULL,
    communityId        TEXT    NOT NULL,
    businessNr         INTEGER NOT NULL,
    allocationMode     TEXT    NOT NULL DEFAULT 'DYNAMIC', /* "DYNAMIC" | "STATIC" */
    settlementInterval TEXT    NOT NULL DEFAULT 'MONTHLY', /* "MONTHLY" | "ANNUAL" */
    providerBusinessNr INTEGER,
    -- Address Info
    street             TEXT    NOT NULL,
    street_number      INTEGER NOT NULL,
    city               TEXT    NOT NULL,
    zip                TEXT    NOT NULL,
    -- Account Info
    iban               TEXT    NOT NULL,
    owner              TEXT    NOT NULL,
    sepa               BOOLEAN NOT NULL DEFAULT false,
    -- Contact Info
    phone              TEXT,
    email              TEXT    NOT NULL,
    website            TEXT

);
CREATE UNIQUE INDEX IF NOT EXISTS idx_unique_eeg ON base.EEG (tenant, name, rcNumber);

CREATE TABLE IF NOT EXISTS base.tariff
(
    id                 UUID    NOT NULL DEFAULT uuid_generate_v4(),
    tenant             VARCHAR NOT NULL,
    type               VARCHAR NOT NULL, /* 'tariff type like EEG, VZP, EZP, AKONTO' */
    name               TEXT    NOT NULL,
    billingPeriod      TEXT             DEFAULT 'monthly',
    useVat             BOOLEAN          DEFAULT FALSE,
    vatInPercent       NUMERIC,
    accountNetAmount   NUMERIC,
    accountGrossAmount NUMERIC,
    participantFee     NUMERIC,
    baseFee            FLOAT   NOT NULL,
    freeKwh            INTEGER,
    businessNr         INTEGER,
    createdBy          TEXT,
    createdDate        DATE,
    lastModifiedDate   DATE,
    version            INTEGER,
    centPerKWH         FLOAT,
    discount           INTEGER,
    status             TEXT    NOT NULL DEFAULT 'ACTIVE', /* ACTIVE | INACTIVE */
    inactiveSince      DATE,
    CONSTRAINT TariffPK PRIMARY KEY (id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tariff ON base.tariff (id, tenant, name, type, version);

CREATE TABLE IF NOT EXISTS base.participant
(
    id                    UUID    NOT NULL DEFAULT uuid_generate_v4(),
    tenant                VARCHAR NOT NULL,
    firstName             VARCHAR NOT NULL,
    lastName              VARCHAR NOT NULL,
    role                  VARCHAR NOT NULL DEFAULT 'EEG_USER', /* 'EEG_USER' | 'EEG_ADMIN' */
    businessRole          VARCHAR NOT NULL DEFAULT 'EEG_PRIVATE', /* 'EEG_PRIVATE' | 'EEG_BUSINESS' */
    titleBefore           VARCHAR,
    titleAfter            VARCHAR,
    participantSince      DATE             DEFAULT now(),
    vatId                 VARCHAR,
    taxId                 VARCHAR,
    companyRegisterNumber VARCHAR,
    status                VARCHAR NOT NULL DEFAULT 'NEW', /* 'NEW' | 'PENDING' | 'ACCEPTED' | 'ACTIVE' | 'INACTIVE' */
    createdBy             VARCHAR NOT NULL,
    createdDate           DATE             DEFAULT now(),
    lastModifiedBy        VARCHAR NOT NULL,
    lastModifiedDate      DATE             DEFAULT now(),
    version               INTEGER          DEFAULT 1,
    tariffid              uuid,
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
    CONSTRAINT FK_ParticipantDetail FOREIGN KEY (participant_id) REFERENCES base.participant (id)
);

CREATE TABLE IF NOT EXISTS base.address
(
    id             UUID NOT NULL DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL,
    type           TEXT NOT NULL DEFAULT 'RESIDENCE', /*Address-Types: 'RESIDENCE' | 'BILLING' */
    street         TEXT,
    street_number  INTEGER,
    city           TEXT,
    zip            TEXT,
    CONSTRAINT addressPK PRIMARY KEY (id),
    CONSTRAINT FK_ParticipantAddress FOREIGN KEY (participant_id) REFERENCES base.participant (id)
);

CREATE TABLE IF NOT EXISTS base.bankaccount
(
    id             UUID NOT NULL DEFAULT uuid_generate_v4(),
    participant_id UUID NOT NULL,
    iban           TEXT NOT NULL,
    owner          TEXT,
    CONSTRAINT bankaccountPK PRIMARY KEY (id),
    CONSTRAINT FK_ParticipantBankaccount FOREIGN KEY (participant_id) REFERENCES base.participant (id)
);

CREATE TABLE IF NOT EXISTS base.meteringpoint
(
    metering_point_id TEXT NOT NULL,
    participant_id    UUID NOT NULL,
    tenant            TEXT NOT NULL,
    transformer       TEXT,
    direction         TEXT NOT NULL DEFAULT 'CONSUMPTION', /* 'GENERATOR' | 'CONSUMPTION' */
    status            TEXT NOT NULL DEFAULT 'NEW', /* "NEW" | "PENDING" | "ACCEPTED" | "ACTIVE" | "INACTIVE" */
    tariff_id         UUID,
    inverterid        TEXT,
    equipmentname     TEXT,
    street            TEXT,
    street_number     TEXT,
    city              TEXT,
    zip               TEXT,
    CONSTRAINT meteringpointPK PRIMARY KEY (metering_point_id, tenant),
    CONSTRAINT FK_ParticipantMeteringpoint FOREIGN KEY (participant_id) REFERENCES base.participant (id),
    CONSTRAINT FK_TariffMeteringpoint FOREIGN KEY (tariff_id) REFERENCES base.tariff (id)
);

CREATE TABLE IF NOT EXISTS base.notification
(
    id           UUID NOT NULL PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant       TEXT NOT NULL,
    type         TEXT NOT NULL DEFAULT 'MESSAGE',/* MESSAGE TYPE DESCRIBE 'ERROR' | 'MESSAGE' | 'NOTIFICATION' */
    notification json NOT NULL DEFAULT '{}',
    date         DATE NOT NULL DEFAULT now(),
    role         VARCHAR NOT NULL DEFAULT 'ADMIN' /* 'USER' | 'ADMIN' */
);

CREATE VIEW base.activeTariff AS
SELECT id,
       name,
       tenant,
       billingperiod,
       usevat,
       vatinpercent,
       accountnetamount,
       accountgrossamount,
       participantfee,
       basefee,
       businessnr,
       version,
       type,
       centperKWH,
       discount,
       freeKwh
FROM base.tariff,
     (SELECT id as tid, MAX(version) as tversion FROM base.tariff GROUP BY id) as x
WHERE id = x.tid
  AND version = x.tversion;



SELECT p.id participant_id
     , p.title_before participant_title_before
     , p.first_name participant_first_name
     , p.last_name participant_last_name
     , p.title_after participant_title_after
     , p.vat_id participant_vat_id
     , p.tax_id participant_tax_id
     , p.company_register_number participant_company_register_number
     , m.metering_point_id metering_point_id
     , m.metering_point_type metering_point_type
     , c.tenant eec_tenant_id
     , c.name eec_name
     , c.vat_id eec_vat_id
     , c.tax_id eec_tax_id
     , c.company_register_number eec_company_register_number
     , c.subject_to_vat eec_subject_to_vat
     , '' eec_invoice_number_prefix
     , '' eec_credit_note_prefix
     , c_contact.phone eec_phone
     , c_contact.email eec_email
     , c_contact.website eec_website
     , c_address.street_name eec_street_name
     , c_address.zip_code eec_zip_code
     , c_address.city eec_city
     , p_address.street_name participant_street_name
     , p_address.zip_code participant_zip_code
     , p_address.city participant_city
     , '' tariff_invoice_number_prefix
     , '' tariff_credit_note_prefix
     , t.tariff_type tariff_type
     , t.name tariff_name
     , t.billing_period tariff_billing_period
     , t.use_vat tariff_use_vat
     , t.vat_in_percent tariff_vat_in_percent
     , t.participant_fee tariff_participant_fee
     , t.basic_fee tariff_basic_fee
     , t.discount tariff_discount
     , t.working_fee_per_consumedkwh tariff_working_fee_per_consumedkwh
     , t.credit_amount_per_producedkwh tariff_credit_amount_per_producedkwh
     , t.freekwh tariff_freekwh
FROM base.participant p, base.tariff t, base.eeg c, base.contactdetail c_contact, base.address c_address, base.address p_address