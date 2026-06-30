package database

import (
	"errors"
	"reflect"
	"strings"

	"github.com/doug-martin/goqu/v9"
	// Register goqu's postgres dialect so goqu.Dialect("postgres") / goqu.New("postgres", …)
	// resolve to the real postgres dialect. Without this blank import goqu silently falls back
	// to the default dialect, which emits "?" placeholders in Prepared mode — lib/pq then fails
	// with `pq: syntax error at or near ")"` (broke getEegById/getEegByEcId after #13 added
	// .Prepared(true)).
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"gopkg.in/guregu/null.v4"
)

func toRecord(v interface{}, extras map[string]interface{}) goqu.Record {
	rec := goqu.Record{}

	val := reflect.ValueOf(v)
	typ := reflect.TypeOf(v)

	// handle pointers
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
		typ = typ.Elem()
	}

	// loop through struct fields
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		key := field.Tag.Get("db")
		if key == "" {
			key = strings.ToLower(field.Name)
		}

		fv := val.Field(i).Interface()

		// Handle null.* types
		switch x := fv.(type) {
		case null.String:
			if x.Valid {
				rec[key] = x.String
			} else {
				rec[key] = nil
			}
		case null.Int:
			if x.Valid {
				rec[key] = x.Int64
			} else {
				rec[key] = nil
			}
		case null.Float:
			if x.Valid {
				rec[key] = x.Float64
			} else {
				rec[key] = nil
			}
		case null.Bool:
			if x.Valid {
				rec[key] = x.Bool
			} else {
				rec[key] = nil
			}
		default:
			rec[key] = fv
		}
	}

	// Add/override with extras
	for k, v := range extras {
		rec[k] = v
	}

	return rec
}

func buildRecordMap(t interface{}, values map[string]interface{}) (goqu.Record, error) {
	val := reflect.ValueOf(t)
	if val.Kind() != reflect.Struct && val.Kind() != reflect.Ptr {
		return nil, errors.New("type must be a struct")
	}

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	updateMap := goqu.Record{}
	typ := val.Type()

	searchTag := func(p string) string {
		for i := 0; i < val.NumField(); i++ {
			field := typ.Field(i)
			jsonTags := field.Tag.Get("json")
			if jsonTags != "" {
				jsonTag := strings.TrimSpace(strings.Split(jsonTags, ",")[0])
				if jsonTag == p {
					dbTags := field.Tag.Get("db")
					if dbTags == "" {
						return jsonTag
					}

					return strings.TrimSpace(strings.Split(dbTags, ",")[0])
				}
			}
		}
		return p
	}

	for key, value := range values {
		name := searchTag(key)
		updateMap[name] = value
	}

	return updateMap, nil
}
