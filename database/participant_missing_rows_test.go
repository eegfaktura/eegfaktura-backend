package database

import (
	"context"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

// 261029-mm1: the member list LEFT JOINs both addresses, the bank account and the
// contact row. One member without one of these rows must not break the list of the
// whole community, and the single read must still work.
func TestGetParticipantsToleratesMissingChildRows(t *testing.T) {
	cases := []struct {
		name   string
		tenant string
		delete string
	}{
		{"without billing address", "TE029901", `DELETE FROM base.address WHERE participant_id = $1 AND type = 'BILLING'`},
		{"without residence address", "TE029902", `DELETE FROM base.address WHERE participant_id = $1 AND type = 'RESIDENCE'`},
		{"without bank account", "TE029903", `DELETE FROM base.bankaccount WHERE participant_id = $1`},
		{"without contact", "TE029904", `DELETE FROM base.contactdetail WHERE participant_id = $1`},
	}

	db, err := GetTestDB(context.Background(), testDB)
	require.NoError(t, err)
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &model.EegParticipant{
				EegParticipantBase: model.EegParticipantBase{FirstName: "Anna", LastName: "Berger", BusinessRole: "EEG_PRIVATE", Role: "EEG_USER", MeteringPoint: []*model.MeteringPoint{}},
				Contact:            model.ContactInfo{Email: null.StringFrom("anna.berger@example.at")},
				BillingAddress:     model.Address{City: null.StringFrom("Wien")},
				ResidentAddress:    model.Address{City: null.StringFrom("Wien")},
				BankAccount:        model.BankInfo{Owner: null.StringFrom("Anna Berger")},
			}
			require.NoError(t, db.RegisterParticipant(ctx, tc.tenant, "test", p))
			_, err := testDB.DbInstance.ExecContext(ctx, tc.delete, p.Id.String())
			require.NoError(t, err)

			list, err := db.GetParticipants(ctx, tc.tenant)
			require.NoError(t, err)
			require.Len(t, list, 1)
			assert.Equal(t, "Anna", list[0].FirstName)

			single, err := db.GetParticipant(ctx, tc.tenant, p.Id.String())
			require.NoError(t, err)
			assert.Equal(t, p.Id.String(), single.Id.String())
		})
	}
}
