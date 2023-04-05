package database

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/xuri/excelize/v2"
	"io"
	"regexp"
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

	for rows.Next() {
		if cols, err := rows.Columns(); err == nil && len(cols) > 0 {
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
					if _, err := fmt.Sscanf(cols[colMap["Name 1"]], "%s %s", &firstname, &lastname); err != nil {
						fmt.Printf("Error Name extracting: %s (%s)\n", err, cols[colMap["Name 1"]])
						continue
					}

					role := "UNKNOWN"
					switch cols[colMap["Energierichtung"]] {
					case "GENERATION":
						role = "GENERATOR"
					case "CONSUMPTION":
						role = "CONSUMER"
					}

					participantData := map[string]string{}
					participantData["firstname"] = firstname
					participantData["lastname"] = lastname
					participantData["city"] = cols[colMap["Ort"]]
					participantData["street"] = fmt.Sprintf("%s %s", cols[colMap["Straße"]], cols[colMap["Hausnummer"]])
					participantData["zip"] = cols[colMap["PLZ"]]
					participantData["role"] = role
					participantData["counter_point"] = cols[colMap["Zählpunkt-ID"]]
					participantData["communityId"] = cols[colMap["Anlagen-ID"]]
					participantData["region"] = cols[colMap["Ortsgebiet"]]
					participantData["cp_state"] = cols[colMap["Zählpunktstatus"]]

					fmt.Printf("Import Masterdata: %+v\n", participantData)
				}
			}
		}
	}
	return nil
}
