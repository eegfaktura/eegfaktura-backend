package database

import (
	"fmt"
	"sort"
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

// tariffKind names the tariff types the import assigns in its hints, as the web app names them.
var tariffKind = map[model.TariffModelType]string{
	model.EEG: "Mitgliedstarif",
	model.VZP: "Verbrauchertarif",
	model.EZP: "Erzeugertarif",
}

// importTariffs holds the community's active tariffs (view base.activetariff: latest version, not archived) by name,
// lower case and without surrounding blanks, and the "Mitgliedstarif" cell of each member (noteMember).
type importTariffs struct {
	byName      map[string][]model.Tariff
	memberCells map[*model.EegParticipant]memberCell
}

// memberCell is the noted "Mitgliedstarif" cell of a member and its row in the file, for the hints.
type memberCell struct {
	name string
	row  int
}

func newImportTariffs(tariffs []model.Tariff) *importTariffs {
	t := &importTariffs{byName: map[string][]model.Tariff{}, memberCells: map[*model.EegParticipant]memberCell{}}
	for _, tariff := range tariffs {
		key := strings.ToLower(strings.TrimSpace(tariff.Name))
		t.byName[key] = append(t.byName[key], tariff)
	}
	return t
}

// noteMember keeps the first non-empty member tariff cell of a member's rows; a different name on a later row
// (trimmed, case-insensitive) is reported and ignored. The cell is looked up when the member is imported, as only
// then it is known whether the member exists already (memberTariff, existingMemberHint).
func (t *importTariffs) noteMember(p *model.EegParticipant, raw string, row int, identifier string, importLog *model.Log) {
	cell, first := strings.TrimSpace(raw), t.memberCells[p].name
	switch {
	case cell == "":
	case first == "":
		t.memberCells[p] = memberCell{name: cell, row: row}
	case !strings.EqualFold(first, cell):
		importLog.Messages = append(importLog.Messages, model.NewLogMessage("WARNING", identifier, "W_TARIFF_CONFLICT",
			fmt.Sprintf("%s: Das Mitglied hat in mehreren Zeilen verschiedene Mitgliedstarife („%s“, „%s“). Es gilt "+
				"der erste.", importCell(row, importMemberTariffColumn), first, cell)))
	}
}

// memberTariff is the member tariff (type EEG) noted for p, nil without one. The hints are returned, not logged: the
// caller reports them only for a member the import creates.
func (t *importTariffs) memberTariff(p *model.EegParticipant, identifier string) (*model.Tariff, []*model.LogMessage) {
	hints := &model.Log{Messages: []*model.LogMessage{}}
	cell := t.memberCells[p]
	tariff := t.find(cell.name, cell.row, importMemberTariffColumn, identifier, model.EEG, hints)
	return tariff, hints.Messages
}

// existingMemberHint: the import found the member by name and only adds its metering points, so a member tariff cell
// is ignored; the hint says so (nil for an empty cell).
func (t *importTariffs) existingMemberHint(p *model.EegParticipant, identifier string) *model.LogMessage {
	cell := t.memberCells[p]
	if cell.name == "" {
		return nil
	}
	return model.NewLogMessage("WARNING", identifier, "W_TARIFF_MEMBER_EXISTS",
		fmt.Sprintf("%s: Das Mitglied gibt es schon, sein Mitgliedstarif wird nicht geändert („%s“ nicht übernommen).",
			importCell(cell.row, importMemberTariffColumn), cell.name))
}

// meter is the metering point tariff named in the column "Zp. Tarifname": type VZP for a consumption, EZP for a
// generation metering point.
func (t *importTariffs) meter(raw string, row int, meteringPointId string, direction model.DirectionType, importLog *model.Log) null.String {
	want := model.VZP
	if direction == model.GENERATOR {
		want = model.EZP
	}
	if tariff := t.find(raw, row, importMeterTariffColumn, meteringPointId, want, importLog); tariff != nil {
		return null.StringFrom(tariff.Id.String())
	}
	return null.String{}
}

// find returns the one active tariff with this name (case-insensitive, trimmed) and type. An empty cell means no
// tariff. An unknown name, a tariff of another type or several tariffs of this name and type are reported as a hint:
// the member or metering point is imported without this tariff.
func (t *importTariffs) find(raw string, row int, column, identifier string, want model.TariffModelType, importLog *model.Log) *model.Tariff {
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

	code, reason := "W_TARIFF_UNKNOWN", fmt.Sprintf("Einen aktiven Tarif „%s“ gibt es in dieser Gemeinschaft nicht.", name)
	switch {
	case len(suitable) > 1:
		code, reason = "W_TARIFF_AMBIGUOUS", fmt.Sprintf("Es gibt mehrere aktive %se mit dem Namen „%s“.", tariffKind[want], name)
	case len(named) > 0:
		code, reason = "W_TARIFF_TYPE", fmt.Sprintf("Der Tarif „%s“ ist kein %s.", name, tariffKind[want])
	}
	importLog.Messages = append(importLog.Messages, model.NewLogMessage("WARNING", identifier, code,
		fmt.Sprintf("%s: %s Der Tarif wird nicht zugeordnet.", importCell(row, column), reason)))
	return nil
}

// joiningFeeMessage is the one import message about new members that got a member tariff with a joining fee (user
// decision 24.11.2026): billing charges the fee with their first energy billing, also to members of a community that
// moves to eegFaktura and paid it before. nil when there are none.
func joiningFeeMessage(members int, tariffNames map[string]bool) *model.LogMessage {
	if members == 0 {
		return nil
	}
	names := make([]string, 0, len(tariffNames))
	for name := range tariffNames {
		names = append(names, name)
	}
	sort.Strings(names)
	who := fmt.Sprintf("%d neue Mitglieder haben", members)
	if members == 1 {
		who = "1 neues Mitglied hat"
	}
	return model.NewLogMessage("WARNING", importMemberTariffColumn, "W_TARIFF_JOINING_FEE",
		fmt.Sprintf("%s einen Mitgliedstarif mit Beitrittsgebühr (%s). Sie wird mit der ersten Energieabrechnung "+
			"verrechnet. Eine schon bezahlte Beitrittsgebühr erfasst du beim Mitglied mit „Außerhalb bezahlt erfassen“.",
			who, strings.Join(names, ", ")))
}
