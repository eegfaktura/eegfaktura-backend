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
// lower case and without surrounding blanks, and the "Mitgliedstarif" cell of each member's first row.
type importTariffs struct {
	byName      map[string][]model.Tariff
	memberCells map[*model.EegParticipant]string
}

func newImportTariffs(tariffs []model.Tariff) *importTariffs {
	t := &importTariffs{byName: map[string][]model.Tariff{}, memberCells: map[*model.EegParticipant]string{}}
	for _, tariff := range tariffs {
		key := strings.ToLower(strings.TrimSpace(tariff.Name))
		t.byName[key] = append(t.byName[key], tariff)
	}
	return t
}

// noteMember keeps the member tariff cell of a member's first row. It is looked up when the member is imported, as
// only then it is known whether the member exists already (memberTariff, existingMemberHint).
func (t *importTariffs) noteMember(p *model.EegParticipant, raw string) {
	t.memberCells[p] = strings.TrimSpace(raw)
}

// memberTariff is the member tariff (type EEG) noted for p, nil without one. The hints are returned, not logged: the
// caller reports them only for a member the import creates.
func (t *importTariffs) memberTariff(p *model.EegParticipant, identifier string) (*model.Tariff, []*model.LogMessage) {
	hints := &model.Log{Messages: []*model.LogMessage{}}
	tariff := t.find(t.memberCells[p], importMemberTariffColumn, identifier, model.EEG, hints)
	return tariff, hints.Messages
}

// existingMemberHint: the import found the member by name and only adds its metering points, so a member tariff cell
// is ignored; the hint says so (nil for an empty cell).
func (t *importTariffs) existingMemberHint(p *model.EegParticipant, identifier string) *model.LogMessage {
	cell := t.memberCells[p]
	if cell == "" {
		return nil
	}
	return model.NewLogMessage("WARNING", identifier, "W_TARIFF_MEMBER_EXISTS",
		fmt.Sprintf("column '%s': the member exists already, its member tariff is not changed (%q ignored)",
			importMemberTariffColumn, cell))
}

// meter is the metering point tariff named in the column "Zp. Tarifname": type VZP for a consumption, EZP for a
// generation metering point.
func (t *importTariffs) meter(raw, meteringPointId string, direction model.DirectionType, importLog *model.Log) null.String {
	want := model.VZP
	if direction == model.GENERATOR {
		want = model.EZP
	}
	if tariff := t.find(raw, importMeterTariffColumn, meteringPointId, want, importLog); tariff != nil {
		return null.StringFrom(tariff.Id.String())
	}
	return null.String{}
}

// find returns the one active tariff with this name (case-insensitive, trimmed) and type. An empty cell means no
// tariff. An unknown name, a tariff of another type or several tariffs of this name and type are reported as a hint:
// the member or metering point is imported without this tariff.
func (t *importTariffs) find(raw, column, identifier string, want model.TariffModelType, importLog *model.Log) *model.Tariff {
	name := strings.TrimSpace(raw)
	if name == "" {
		return nil
	}
	named := t.byName[strings.ToLower(name)]
	var suitable []model.Tariff
	for _, tariff := range named {
		if tariff.Type == want {
			suitable = append(suitable, tariff)
		}
	}
	if len(suitable) == 1 {
		return &suitable[0]
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
	return nil
}
