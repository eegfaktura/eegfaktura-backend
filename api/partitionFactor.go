package api

import (
	"context"
	"errors"
	"fmt"

	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
)

// assertPartitionFactorMeters checks that every metering point of a partition factor
// change belongs to the caller's community before anything is sent (261005-ca4).
// Used by POST /meteringpoint/changepartitionfactor and POST /master/updatepartfact.
func assertPartitionFactorMeters(ctx context.Context, db database.Database, tenant string, meters []*model.ChangePartitionFactorRequest) error {
	if len(meters) == 0 {
		return nil
	}
	ids := make([]string, 0, len(meters))
	for _, m := range meters {
		if m == nil { // {"meteringPoints":[null]} (261005-ca6)
			return model.ErrParseJson(errors.New("empty metering point entry"))
		}
		ids = append(ids, m.MeteringPoint)
	}
	own, err := db.FindMeteringByIds(ctx, tenant, ids)
	if err != nil {
		return model.ErrFindMeter(err)
	}
	known := map[string]bool{}
	for _, m := range own {
		known[m.MeteringPoint] = true
	}
	for _, id := range ids {
		if !known[id] {
			return model.ErrFindMeter(fmt.Errorf("metering point %s not found", id))
		}
	}
	return nil
}
