package database

import (
	"at.ourproject/vfeeg-backend/model"
	"fmt"
	"github.com/golang/glog"
	log "github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"
	"gopkg.in/guregu/null.v4"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var netOperatorMatch = regexp.MustCompile(`^[A-Z]{2}[0-9]*$`)

func openReader(r io.Reader, filename string, opt ...excelize.Options) (*excelize.File, error) {
	f, err := excelize.OpenReader(r, opt...)
	if err != nil {
		return nil, err
	}
	f.Path = filename
	return f, nil
}

func ImportMasterdataFromExcel(r io.Reader, filename, sheet, tenant string) error {
	var f *excelize.File
	var err error

	if f, err = openReader(r, filename); err != nil {
		return err
	}

	defer f.Close()
	fmt.Println("Successfully open stream")

	rows, err := f.Rows(sheet)
	if err != nil {
		glog.Error(err)
		return err
	}
	fmt.Printf("Rows: %+v\n", rows)
	colMap := map[string]int{}
	participants := []*model.EegParticipant{}

	for rows.Next() {
		if cols, err := rows.Columns(excelize.Options{RawCellValue: true}); err == nil && len(cols) > 0 {
			switch cols[0] {
			case "[### Leerzeile für Importer ###]":
				continue
			case "Netzbetreiber", "Grid Operator":
				for i, c := range cols {
					colMap[c] = i
				}

				continue
			default:
				switch {
				case netOperatorMatch.MatchString(cols[0]):
					var firstname string
					var lastname string

					excelName1 := getColumValue(cols, colMap, "Name 2", "Name2")
					excelName2 := getColumValue(cols, colMap, "Name 1", "Name1")

					if len(excelName2) == 0 || len(excelName2) < 2 {
						if _, err := fmt.Sscanf(getColumValue(cols, colMap, "Name 2", "Name2"), "%s %s", &lastname, &firstname); err != nil {
							fmt.Printf("Error Name extracting: %s (%s)\n", err, getColumValue(cols, colMap, "Name 1", "Name1"))
							continue
						}
					} else {
						firstname = excelName1
						lastname = excelName2
					}

					role := model.UNKNOWN
					switch getColumValue(cols, colMap, "Energierichtung", "Energy Direction") {
					case "GENERATION":
						role = model.GENERATOR
					case "CONSUMPTION":
						role = model.CONSUMPTION
					}

					streetNumber := getColumValue(cols, colMap, "Hausnummer", "Street Number")
					var participantSince time.Time
					docSignedAt := getColumValue(cols, colMap, "Dokument unterschrieben", "Document Signature Date")
					if len(docSignedAt) > 0 {
						participantSince = parseExcelDate(docSignedAt)
					} else {
						participantSince = time.Now()
					}

					cpStatus := getColumValue(cols, colMap, "Zählpunktstatus", "Metering Point State")
					if cpStatus == "ACTIVATED" || cpStatus == "REGISTERED" || len(cpStatus) == 0 {
						var participant *model.EegParticipant
						if p, ok := findParticipant(participants, firstname, lastname); ok {
							participant = p
						} else {
							participant = &model.EegParticipant{
								FirstName:   firstname,
								LastName:    lastname,
								TitleBefore: getColumValue(cols, colMap, "TitelVor", "TitleBefor"),
								TitleAfter:  getColumValue(cols, colMap, "TitelNach", "TitleAfter"),
								ResidentAddress: model.Address{
									Type:         model.RESIDENCE,
									Street:       getColumValue(cols, colMap, "Straße", "Street"),
									StreetNumber: streetNumber,
									Zip:          getColumValue(cols, colMap, "PLZ", "ZIP"),
									City:         getColumValue(cols, colMap, "Ort", "City"),
								},
								BillingAddress: model.Address{
									Type:         model.BILLING,
									Street:       getColumValue(cols, colMap, "Straße", "Street"),
									StreetNumber: streetNumber,
									Zip:          getColumValue(cols, colMap, "PLZ", "ZIP"),
									City:         getColumValue(cols, colMap, "Ort", "City"),
								},
								Status:           model.StatusType(model.ACTIVE),
								ParticipantSince: participantSince,
								MeteringPoint:    []*model.MeteringPoint{},
								BankAccount: model.BankInfo{
									Iban:  null.StringFrom(getColumValue(cols, colMap, "IBAN", "IBAN")),
									Owner: null.StringFrom(getColumValue(cols, colMap, "Kontoinhaber", "Accountname"))},
								Contact:               model.ContactInfo{Email: null.StringFrom(getColumValue(cols, colMap, "email", "email"))},
								CompanyRegisterNumber: getColumValue(cols, colMap, "RegisterNr", "companyRegisterNumber"),
								Version:               0,
							}
							participants = append(participants, participant)
						}
						participant.MeteringPoint = append(participant.MeteringPoint, &model.MeteringPoint{
							MeteringPoint: getColumValue(cols, colMap, "Zählpunkt", "MeteringPoint Id"),
							Transformer:   null.String{},
							Direction:     model.DirectionType(role),
							Status:        model.StatusType(model.ACTIVE),
							TariffId:      null.String{},
							EquipmentName: null.String{},
							InverterId:    null.String{},
							Street:        null.StringFrom(getColumValue(cols, colMap, "Straße", "Street")),
							StreetNumber:  null.StringFrom(getColumValue(cols, colMap, "Hausnummer", "Street Number")),
							City:          null.StringFrom(getColumValue(cols, colMap, "Ort", "City")),
							Zip:           null.StringFrom(getColumValue(cols, colMap, "PLZ", "ZIP")),
						})
					}
				}
			}
		}
	}
	fmt.Printf("LEN _ Import participants: %+v\n", len(participants))
	for _, p := range participants {
		//fmt.Printf("Import participants: %+v\n", p)
		err = ImportParticipant(strings.ToUpper(tenant), "petero", p)
		if err != nil {
			log.Errorf("Error Import Participant from Excel: %s", err.Error())
		}
	}

	return nil
}

func findParticipant(participants []*model.EegParticipant, firstname, lastname string) (*model.EegParticipant, bool) {
	for _, p := range participants {
		if p.FirstName == firstname && p.LastName == lastname {
			return p, true
		}
	}
	return nil, false
}

func getColumValue(cols []string, values map[string]int, deName, enName string) string {
	idx := -1
	if _, ok := values[deName]; ok {
		idx = values[deName]
	} else if _, ok := values[enName]; ok {
		idx = values[enName]
	}

	if idx < 0 {
		return ""
	}
	if idx >= len(cols) {
		return ""
	}
	return cols[idx]
}

var numberPattern = regexp.MustCompile(`^[0-9\\.,]+$`)

func isDate(cell string) bool {
	if len(cell) > 0 && numberPattern.MatchString(cell) {
		return true
	}
	println(cell)
	return false
}

func parseExcelDate(cell string) time.Time {
	if isDate(cell) {
		var excelEpoch = time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC)
		var days, _ = strconv.ParseFloat(cell, 64)
		return excelEpoch.Add(time.Second * time.Duration(days*86400))
	}
	return time.Now()
}
