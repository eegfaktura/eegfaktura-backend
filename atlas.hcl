env "local" {
  src = "./schema-masterdata.sql"
  dev = "docker://postgres/16/dev"
  migration {
    dir    = "file://migrations"
    format = golang-migrate
  }
  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}