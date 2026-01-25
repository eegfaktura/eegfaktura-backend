table "address" {
  schema = schema.base
  column "id" {
    null    = false
    type    = uuid
    default = sql("public.uuid_generate_v4()")
  }
  column "participant_id" {
    null = false
    type = uuid
  }
  column "type" {
    null    = false
    type    = text
    default = "RESIDENCE"
  }
  column "street" {
    null = true
    type = text
  }
  column "streetNumber" {
    null = true
    type = text
  }
  column "city" {
    null = true
    type = text
  }
  column "zip" {
    null = true
    type = text
  }
  primary_key "addresspk" {
    columns = [column.id]
  }
  foreign_key "fk_participantaddress" {
    columns     = [column.participant_id]
    ref_columns = [table.participant.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
}
table "bankaccount" {
  schema = schema.base
  column "id" {
    null    = false
    type    = uuid
    default = sql("public.uuid_generate_v4()")
  }
  column "participant_id" {
    null = false
    type = uuid
  }
  column "iban" {
    null = true
    type = text
  }
  column "owner" {
    null = true
    type = text
  }
  column "bankName" {
    null = true
    type = text
  }
  primary_key "bankaccountpk" {
    columns = [column.id]
  }
  foreign_key "fk_participantbankaccount" {
    columns     = [column.participant_id]
    ref_columns = [table.participant.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
}
table "contactdetail" {
  schema = schema.base
  column "id" {
    null    = false
    type    = uuid
    default = sql("public.uuid_generate_v4()")
  }
  column "participant_id" {
    null = false
    type = uuid
  }
  column "email" {
    null = true
    type = text
  }
  column "phone" {
    null = true
    type = text
  }
  primary_key "contactdetailspk" {
    columns = [column.id]
  }
  foreign_key "fk_participantdetail" {
    columns     = [column.participant_id]
    ref_columns = [table.participant.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
}
table "eeg" {
  schema = schema.base
  column "tenant" {
    null = false
    type = character_varying
  }
  column "name" {
    null = false
    type = text
  }
  column "description" {
    null = true
    type = character_varying(40)
  }
  column "periods" {
    null    = true
    type    = json
    default = "[]"
  }
  column "rcNumber" {
    null = false
    type = text
  }
  column "area" {
    null = false
    type = text
  }
  column "legal" {
    null    = false
    type    = text
    default = "verein"
  }
  column "gridoperator_code" {
    null = false
    type = text
  }
  column "gridoperator_name" {
    null = false
    type = text
  }
  column "communityId" {
    null = false
    type = text
  }
  column "businessNr" {
    null = true
    type = text
  }
  column "allocationMode" {
    null    = false
    type    = text
    default = "DYNAMIC"
  }
  column "settlementInterval" {
    null    = false
    type    = text
    default = "MONTHLY"
  }
  column "providerBusinessNr" {
    null = true
    type = integer
  }
  column "taxNumber" {
    null = true
    type = text
  }
  column "vatNumber" {
    null = true
    type = text
  }
  column "subjecttovat" {
    null = true
    type = boolean
  }
  column "contactPerson" {
    null = true
    type = text
  }
  column "street" {
    null = false
    type = text
  }
  column "streetNumber" {
    null = false
    type = text
  }
  column "city" {
    null = false
    type = text
  }
  column "zip" {
    null = false
    type = text
  }
  column "iban" {
    null = true
    type = text
  }
  column "owner" {
    null = true
    type = text
  }
  column "sepa" {
    null    = false
    type    = boolean
    default = false
  }
  column "bankName" {
    null = true
    type = text
  }
  column "phone" {
    null = true
    type = text
  }
  column "email" {
    null = false
    type = text
  }
  column "website" {
    null = true
    type = text
  }
  column "online" {
    null    = false
    type    = boolean
    default = false
  }
  column "createdat" {
    null    = false
    type    = date
    default = sql("(now())::date")
  }
  primary_key {
    columns = [column.tenant]
  }
  index "idx_unique_eeg" {
    unique  = true
    columns = [column.tenant, column.name, column.rcNumber]
  }
}
table "gridoperators" {
  schema = schema.base
  column "id" {
    null = false
    type = character_varying
  }
  column "name" {
    null = false
    type = character_varying
  }
  primary_key "pk_gridoperators" {
    columns = [column.id, column.name]
  }
}
table "metering_partition_factor" {
  schema = schema.base
  column "metering_point_id" {
    null = false
    type = text
  }
  column "version" {
    null = false
    type = serial
  }
  column "participant_id" {
    null = false
    type = uuid
  }
  column "tenant" {
    null = false
    type = text
  }
  column "partFact" {
    null = false
    type = integer
  }
  column "createdAt" {
    null    = false
    type    = date
    default = sql("now()")
  }
  column "createdBy" {
    null = false
    type = character_varying
  }
  primary_key "meteringpointpartitionpk" {
    columns = [column.metering_point_id, column.version]
  }
  foreign_key "fk_meteringpointpartition" {
    columns     = [column.metering_point_id, column.tenant, column.participant_id]
    ref_columns = [table.meteringpoint.column.metering_point_id, table.meteringpoint.column.tenant, table.meteringpoint.column.participant_id]
    on_update   = CASCADE
    on_delete   = CASCADE
  }
}
table "meteringpoint" {
  schema = schema.base
  column "metering_point_id" {
    null = false
    type = text
  }
  column "consent_id" {
    null = true
    type = text
  }
  column "participant_id" {
    null = false
    type = uuid
  }
  column "tenant" {
    null = false
    type = text
  }
  column "grid_operator_name" {
    null = true
    type = character_varying
  }
  column "grid_operator_id" {
    null = true
    type = character_varying
  }
  column "transformer" {
    null = true
    type = text
  }
  column "direction" {
    null    = false
    type    = text
    default = "CONSUMPTION"
  }
  column "status" {
    null    = false
    type    = text
    default = "INIT"
  }
  column "process_state" {
    null    = false
    type    = text
    default = "NEW"
  }
  column "statusCode" {
    null = true
    type = integer
  }
  column "tariff_id" {
    null = true
    type = uuid
  }
  column "inverterid" {
    null = true
    type = text
  }
  column "equipmentNumber" {
    null = true
    type = text
  }
  column "equipmentName" {
    null = true
    type = text
  }
  column "street" {
    null = true
    type = text
  }
  column "streetNumber" {
    null = true
    type = text
  }
  column "city" {
    null = true
    type = text
  }
  column "zip" {
    null = true
    type = text
  }
  column "registeredSince" {
    null    = false
    type    = date
    default = sql("(now())::date")
  }
  column "modifiedAt" {
    null    = false
    type    = timestamp
    default = sql("now()")
  }
  column "modifiedBy" {
    null = true
    type = text
  }
  column "activesince" {
    null = true
    type = date
  }
  column "inactivesince" {
    null = true
    type = date
  }
  column "active" {
    null    = false
    type    = integer
    default = 1
  }
  column "flag" {
    null    = false
    type    = integer
    default = 1
  }
  column "allocation_factor" {
    null = true
    type = double_precision
  }
  primary_key "meteringpointpk" {
    columns = [column.metering_point_id, column.tenant, column.participant_id]
  }
  foreign_key "fk_participantmeteringpoint" {
    columns     = [column.participant_id]
    ref_columns = [table.participant.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "idx_unique_meteringpoint_active" {
    unique  = true
    columns = [column.metering_point_id, column.tenant, column.flag]
    where   = "(flag = 1)"
  }
}
table "notification" {
  schema = schema.base
  column "id" {
    null = false
    type = serial
  }
  column "tenant" {
    null = false
    type = text
  }
  column "type" {
    null    = false
    type    = text
    default = "MESSAGE"
  }
  column "notification" {
    null    = false
    type    = json
    default = "{}"
  }
  column "date" {
    null    = false
    type    = timestamp
    default = sql("now()")
  }
  column "role" {
    null    = false
    type    = character_varying
    default = "ADMIN"
  }
  column "process" {
    null    = false
    type    = character_varying
    default = "EDA_PROCESS"
  }
  primary_key {
    columns = [column.id]
  }
}
table "participant" {
  schema = schema.base
  column "id" {
    null    = false
    type    = uuid
    default = sql("public.uuid_generate_v4()")
  }
  column "participantNumber" {
    null = true
    type = character_varying
  }
  column "tenant" {
    null = false
    type = character_varying
  }
  column "firstname" {
    null = false
    type = character_varying
  }
  column "lastname" {
    null = false
    type = character_varying
  }
  column "role" {
    null    = false
    type    = character_varying
    default = "EEG_USER"
  }
  column "businessRole" {
    null    = false
    type    = character_varying
    default = "EEG_PRIVATE"
  }
  column "titleBefore" {
    null = true
    type = character_varying
  }
  column "titleAfter" {
    null = true
    type = character_varying
  }
  column "participantSince" {
    null    = false
    type    = date
    default = sql("now()")
  }
  column "vatNumber" {
    null = true
    type = character_varying
  }
  column "taxNumber" {
    null = true
    type = character_varying
  }
  column "companyRegisterNumber" {
    null = true
    type = character_varying
  }
  column "status" {
    null    = false
    type    = character_varying
    default = "NEW"
  }
  column "createdBy" {
    null = false
    type = character_varying
  }
  column "createdDate" {
    null    = true
    type    = date
    default = sql("now()")
  }
  column "lastModifiedBy" {
    null = false
    type = character_varying
  }
  column "lastModifiedDate" {
    null    = true
    type    = date
    default = sql("now()")
  }
  column "version" {
    null    = true
    type    = integer
    default = 1
  }
  column "tariffId" {
    null = true
    type = uuid
  }
  primary_key "participantpk" {
    columns = [column.id]
  }
  index "idx_unique_participant_tenant" {
    unique  = true
    columns = [column.id, column.tenant, column.version]
  }
}
table "processhistory" {
  schema = schema.base
  column "id" {
    null    = false
    type    = uuid
    default = sql("public.uuid_generate_v4()")
  }
  column "tenant" {
    null = false
    type = text
  }
  column "conversationId" {
    null = false
    type = text
  }
  column "type" {
    null = false
    type = text
  }
  column "date" {
    null    = false
    type    = timestamp
    default = sql("now()")
  }
  column "issuer" {
    null = false
    type = text
  }
  column "message" {
    null    = false
    type    = json
    default = "{}"
  }
  column "direction" {
    null    = false
    type    = text
    default = "OUT"
  }
  column "protocol" {
    null = true
    type = character_varying
  }
  primary_key {
    columns = [column.id]
  }
  index "processhistory_tenant_index" {
    on {
      column = column.tenant
    }
    on {
      desc   = true
      column = column.conversationId
    }
    on {
      column = column.id
    }
  }
}
table "schema_migrations" {
  schema = schema.base
  column "version" {
    null = false
    type = bigint
  }
  column "dirty" {
    null = false
    type = boolean
  }
  primary_key {
    columns = [column.version]
  }
}
table "tariff" {
  schema = schema.base
  column "id" {
    null    = false
    type    = uuid
    default = sql("public.uuid_generate_v4()")
  }
  column "tenant" {
    null = false
    type = character_varying
  }
  column "type" {
    null = false
    type = character_varying
  }
  column "name" {
    null = false
    type = text
  }
  column "billingPeriod" {
    null    = true
    type    = text
    default = "monthly"
  }
  column "useVat" {
    null    = true
    type    = boolean
    default = false
  }
  column "vatSupplementaryText" {
    null    = false
    type    = text
    default = ""
  }
  column "vatInPercent" {
    null    = false
    type    = numeric
    default = 0
  }
  column "accountNetAmount" {
    null = true
    type = numeric
  }
  column "accountGrossAmount" {
    null = true
    type = numeric
  }
  column "participantFee" {
    null    = false
    type    = double_precision
    default = 0
  }
  column "baseFee" {
    null    = false
    type    = double_precision
    default = 0
  }
  column "freeKWh" {
    null = true
    type = integer
  }
  column "businessNr" {
    null = true
    type = integer
  }
  column "createdBy" {
    null = true
    type = text
  }
  column "createdDate" {
    null    = true
    type    = date
    default = sql("now()")
  }
  column "lastModifiedDate" {
    null    = true
    type    = date
    default = sql("now()")
  }
  column "version" {
    null = false
    type = integer
  }
  column "centPerKWh" {
    null    = true
    type    = double_precision
    default = 0
  }
  column "discount" {
    null = true
    type = integer
  }
  column "status" {
    null    = false
    type    = text
    default = "ACTIVE"
  }
  column "inactiveSince" {
    null = true
    type = date
  }
  column "meteringPointFee" {
    null = true
    type = double_precision
  }
  column "meteringPointVat" {
    null = true
    type = integer
  }
  column "useMeteringPointFee" {
    null    = false
    type    = boolean
    default = false
  }
  primary_key "tariffpk" {
    columns = [column.id, column.version]
  }
  index "idx_tariff" {
    unique  = true
    columns = [column.id, column.tenant, column.name, column.type, column.version]
  }
}
schema "base" {
}
