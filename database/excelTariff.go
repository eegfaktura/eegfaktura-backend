package database

import (
	"fmt"
	"strings"

	"at.ourproject/vfeeg-backend/model"
	"gopkg.in/guregu/null.v4"
)

// Tariff columns of the master data export, read by the master data import (platform#268). They have no English
// alternative: the import passes the same name twice to getColumValue (an empty one would match an empty header cell).
const (
	importMemberTariffColumn = "Mitgliedstarif"
	importMeterTariffColumn  = "Zp. Tarifname"
)

// tariffKind names the tariff types the import assigns, for the hint about a tariff of another type.
var tariffKind = map[model.TariffModelType]string{
	model.EEG: "a member tariff",
	model.VZP: "a tariff for consumption",
	model.EZP: "a tariff for generation",
}

// importTariffs holds the community's active tariffs (view base.activetariff: latest version, not archived) by name,
// lower case and without surrounding blanks.
type importTariffs map[string][]model.Tariff

func newImportTariffs(tariffs []model.Tariff) importTariffs {
	byName := importTariffs{}
	for _, t := range tariffs {
		key := strings.ToLower(strings.TrimSpace(t.Name))
		byName[key] = append(byName[key], t)
	}
	return byName
}

// member is the member tariff (type EEG) named in the column "Mitgliedstarif".
func (t importTariffs) member(raw, identifier string, importLog *model.Log) null.String {
	return t.find(raw, importMemberTariffColumn, identifier, model.EEG, importLog)
}

// meter is the metering point tariff named in the column "Zp. Tarifname": type VZP for a consumption, EZP for a
// generation metering point.
func (t importTariffs) meter(raw, meteringPointId string, direction model.DirectionType, importLog *model.Log) null.String {
	if direction == model.GENERATOR {
		return t.find(raw, importMeterTariffColumn, meteringPointId, model.EZP, importLog)
	}
	return t.find(raw, importMeterTariffColumn, meteringPointId, model.VZP, importLog)
}

// find returns the id of the one active tariff with this name (case-insensitive, trimmed) and type. An empty cell
// means no tariff. An unknown name, a tariff of another type or several tariffs of this name and type are reported
// as a hint: the member or metering point is imported without this tariff.
func (t importTariffs) find(raw, column, identifier string, want model.TariffModelType, importLog *model.Log) null.String {
	name := strings.TrimSpace(raw)
	if name == "" {
		return null.String{}
	}
	named := t[strings.ToLower(name)]
	var suitable []model.Tariff
	for _, tariff := range named {
		if tariff.Type == want {
			suitable = append(suitable, tariff)
		}
	}
	if len(suitable) == 1 {
		return null.StringFrom(suitable[0].Id.String())
	}

	code, reason := "W_TARIFF_UNKNOWN", fmt.Sprintf("no active tariff named %q in this community", name)
	switch {
	case len(suitable) > 1:
		code, reason = "W_TARIFF_AMBIGUOUS", fmt.Sprintf("several active tariffs of type %s are named %q", want, name)
	case len(named) > 0:
		code, reason = "W_TARIFF_TYPE", fmt.Sprintf("tariff %q is not %s (type %s)", name, tariffKind[want], want)
	}
	importLog.Messages = append(importLog.Messages, model.NewLogMessage("WARNING", identifier, code,
		fmt.Sprintf("column '%s': %s; imported without this tariff", column, reason)))
	return null.String{}
}
