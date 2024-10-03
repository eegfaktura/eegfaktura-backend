package eda

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
)

type RecorderMock struct {
	mock.Mock
	dbOpen database.OpenDbXConnection
}

func newRecorderMock(t *testing.T) *RecorderMock {
	var mockDb, err = database.GetDatabaseMock()
	require.NoError(t, err)
	return &RecorderMock{dbOpen: mockDb.OpenMockDb}
}

func (_m *RecorderMock) saveNotification(notificationValue map[string]interface{}, tenant, notificationType, role string) error {
	args := _m.Called(notificationValue, tenant, notificationType, role)
	return args.Error(0)
}
func (_m *RecorderMock) saveHistory(tenant string, messageCode model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error {
	args := _m.Called(tenant, messageCode, conversationId, role, dir, protocol, msg)
	return args.Error(0)
}

func (_m *RecorderMock) databaseConnectFunc() database.OpenDbXConnection {
	return _m.dbOpen
}

func (_m *RecorderMock) databaseConnection() (*sqlx.DB, error) {
	return _m.dbOpen()
}

func (_m *RecorderMock) meteringPointPerformAnswerMsg(tenant string, meterId []string) error {
	return nil
}

var extractMeters = func(p model.EbmsMessage, proto model.EbMsMessageType) []string {
	meters := []string{}
	switch proto {
	case model.EBMS_ONLINE_REG_APPROVAL, model.EBMS_ONLINE_REG_ANSWER:
		_, meters, _, _ = extractResponseCodeAndMeteringPoint(&p)
	default:
		meters = p.Meters()
	}
	return meters
}

func TestProtcolCrMsgHandler(t *testing.T) {
	var mockDb, err = database.GetDatabaseMock()
	require.NoError(t, err)
	recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}

	jsonString := `{"messageId":"AT003000202208201421374610104995950","conversationId":"AT003000202208191420233640008300242","sender":"AT003000","receiver":"RC100130","messageCode":"DATEN_CRMSG","meter":{"meteringPoint":"AT0030000000000000000000000200959"},"energy":{"start":1660773600000,"end":1660860000000,"interval":"QH","nInterval":288,"data":[{"meterCode":"1-1:1.9.0 G.01","value":[{"from":1660773600000,"to":1660774500000,"method":"L1","value":0.00525},{"from":1660774500000,"to":1660775400000,"method":"L1","value":0.0055},{"from":1660775400000,"to":1660776300000,"method":"L1","value":0.0055},{"from":1660776300000,"to":1660777200000,"method":"L1","value":0.00925},{"from":1660777200000,"to":1660778100000,"method":"L1","value":0.0075},{"from":1660778100000,"to":1660779000000,"method":"L1","value":0.005},{"from":1660779000000,"to":1660779900000,"method":"L1","value":0.006},{"from":1660779900000,"to":1660780800000,"method":"L1","value":0.0055},{"from":1660780800000,"to":1660781700000,"method":"L1","value":0.006},{"from":1660781700000,"to":1660782600000,"method":"L1","value":0.00525},{"from":1660782600000,"to":1660783500000,"method":"L1","value":0.00625},{"from":1660783500000,"to":1660784400000,"method":"L1","value":0.00625},{"from":1660784400000,"to":1660785300000,"method":"L1","value":0.0065},{"from":1660785300000,"to":1660786200000,"method":"L1","value":0.006},{"from":1660786200000,"to":1660787100000,"method":"L1","value":0.006},{"from":1660787100000,"to":1660788000000,"method":"L1","value":0.0085},{"from":1660788000000,"to":1660788900000,"method":"L1","value":0.00875},{"from":1660788900000,"to":1660789800000,"method":"L1","value":0.00975},{"from":1660789800000,"to":1660790700000,"method":"L1","value":0.01},{"from":1660790700000,"to":1660791600000,"method":"L1","value":0.009},{"from":1660791600000,"to":1660792500000,"method":"L1","value":0.008},{"from":1660792500000,"to":1660793400000,"method":"L1","value":0.0065},{"from":1660793400000,"to":1660794300000,"method":"L1","value":0.007},{"from":1660794300000,"to":1660795200000,"method":"L1","value":0.0065},{"from":1660795200000,"to":1660796100000,"method":"L1","value":0.00725},{"from":1660796100000,"to":1660797000000,"method":"L1","value":0.00725},{"from":1660797000000,"to":1660797900000,"method":"L1","value":0.00625},{"from":1660797900000,"to":1660798800000,"method":"L1","value":0.006},{"from":1660798800000,"to":1660799700000,"method":"L1","value":0},{"from":1660799700000,"to":1660800600000,"method":"L1","value":0.00025},{"from":1660800600000,"to":1660801500000,"method":"L1","value":0.00175},{"from":1660801500000,"to":1660802400000,"method":"L1","value":0.00075},{"from":1660802400000,"to":1660803300000,"method":"L1","value":0.00325},{"from":1660803300000,"to":1660804200000,"method":"L1","value":0.00725},{"from":1660804200000,"to":1660805100000,"method":"L1","value":0.01675},{"from":1660805100000,"to":1660806000000,"method":"L1","value":0.0155},{"from":1660806000000,"to":1660806900000,"method":"L1","value":0},{"from":1660806900000,"to":1660807800000,"method":"L1","value":0},{"from":1660807800000,"to":1660808700000,"method":"L1","value":0},{"from":1660808700000,"to":1660809600000,"method":"L1","value":0},{"from":1660809600000,"to":1660810500000,"method":"L1","value":0},{"from":1660810500000,"to":1660811400000,"method":"L1","value":0},{"from":1660811400000,"to":1660812300000,"method":"L1","value":0},{"from":1660812300000,"to":1660813200000,"method":"L1","value":0},{"from":1660813200000,"to":1660814100000,"method":"L1","value":0},{"from":1660814100000,"to":1660815000000,"method":"L1","value":0},{"from":1660815000000,"to":1660815900000,"method":"L1","value":0},{"from":1660815900000,"to":1660816800000,"method":"L1","value":0},{"from":1660816800000,"to":1660817700000,"method":"L1","value":0},{"from":1660817700000,"to":1660818600000,"method":"L1","value":0},{"from":1660818600000,"to":1660819500000,"method":"L1","value":0},{"from":1660819500000,"to":1660820400000,"method":"L1","value":0},{"from":1660820400000,"to":1660821300000,"method":"L1","value":0},{"from":1660821300000,"to":1660822200000,"method":"L1","value":0},{"from":1660822200000,"to":1660823100000,"method":"L1","value":0},{"from":1660823100000,"to":1660824000000,"method":"L1","value":0},{"from":1660824000000,"to":1660824900000,"method":"L1","value":0},{"from":1660824900000,"to":1660825800000,"method":"L1","value":0},{"from":1660825800000,"to":1660826700000,"method":"L1","value":0},{"from":1660826700000,"to":1660827600000,"method":"L1","value":0},{"from":1660827600000,"to":1660828500000,"method":"L1","value":0},{"from":1660828500000,"to":1660829400000,"method":"L1","value":0},{"from":1660829400000,"to":1660830300000,"method":"L1","value":0},{"from":1660830300000,"to":1660831200000,"method":"L1","value":0},{"from":1660831200000,"to":1660832100000,"method":"L1","value":0},{"from":1660832100000,"to":1660833000000,"method":"L1","value":0},{"from":1660833000000,"to":1660833900000,"method":"L1","value":0.0035},{"from":1660833900000,"to":1660834800000,"method":"L1","value":0.00275},{"from":1660834800000,"to":1660835700000,"method":"L1","value":0},{"from":1660835700000,"to":1660836600000,"method":"L1","value":0},{"from":1660836600000,"to":1660837500000,"method":"L1","value":0},{"from":1660837500000,"to":1660838400000,"method":"L1","value":0},{"from":1660838400000,"to":1660839300000,"method":"L1","value":0},{"from":1660839300000,"to":1660840200000,"method":"L1","value":0},{"from":1660840200000,"to":1660841100000,"method":"L1","value":0},{"from":1660841100000,"to":1660842000000,"method":"L1","value":0},{"from":1660842000000,"to":1660842900000,"method":"L1","value":0},{"from":1660842900000,"to":1660843800000,"method":"L1","value":0},{"from":1660843800000,"to":1660844700000,"method":"L1","value":0.0015},{"from":1660844700000,"to":1660845600000,"method":"L1","value":0.00825},{"from":1660845600000,"to":1660846500000,"method":"L1","value":0.0075},{"from":1660846500000,"to":1660847400000,"method":"L1","value":0.00725},{"from":1660847400000,"to":1660848300000,"method":"L1","value":0.00675},{"from":1660848300000,"to":1660849200000,"method":"L1","value":0.0065},{"from":1660849200000,"to":1660850100000,"method":"L1","value":0.0075},{"from":1660850100000,"to":1660851000000,"method":"L1","value":0.006},{"from":1660851000000,"to":1660851900000,"method":"L1","value":0.008},{"from":1660851900000,"to":1660852800000,"method":"L1","value":0.0095},{"from":1660852800000,"to":1660853700000,"method":"L1","value":0.00975},{"from":1660853700000,"to":1660854600000,"method":"L1","value":0.00825},{"from":1660854600000,"to":1660855500000,"method":"L1","value":0.01},{"from":1660855500000,"to":1660856400000,"method":"L1","value":0.009},{"from":1660856400000,"to":1660857300000,"method":"L1","value":0.00625},{"from":1660857300000,"to":1660858200000,"method":"L1","value":0.00575},{"from":1660858200000,"to":1660859100000,"method":"L1","value":0.00625},{"from":1660859100000,"to":1660860000000,"method":"L1","value":0.006}]},{"meterCode":"1-1:2.9.0 G.02","value":[{"from":1660773600000,"to":1660774500000,"method":"L1","value":0},{"from":1660774500000,"to":1660775400000,"method":"L1","value":0},{"from":1660775400000,"to":1660776300000,"method":"L1","value":0},{"from":1660776300000,"to":1660777200000,"method":"L1","value":0},{"from":1660777200000,"to":1660778100000,"method":"L1","value":0},{"from":1660778100000,"to":1660779000000,"method":"L1","value":0},{"from":1660779000000,"to":1660779900000,"method":"L1","value":0},{"from":1660779900000,"to":1660780800000,"method":"L1","value":0},{"from":1660780800000,"to":1660781700000,"method":"L1","value":0},{"from":1660781700000,"to":1660782600000,"method":"L1","value":0},{"from":1660782600000,"to":1660783500000,"method":"L1","value":0},{"from":1660783500000,"to":1660784400000,"method":"L1","value":0},{"from":1660784400000,"to":1660785300000,"method":"L1","value":0},{"from":1660785300000,"to":1660786200000,"method":"L1","value":0},{"from":1660786200000,"to":1660787100000,"method":"L1","value":0},{"from":1660787100000,"to":1660788000000,"method":"L1","value":0},{"from":1660788000000,"to":1660788900000,"method":"L1","value":0},{"from":1660788900000,"to":1660789800000,"method":"L1","value":0},{"from":1660789800000,"to":1660790700000,"method":"L1","value":0},{"from":1660790700000,"to":1660791600000,"method":"L1","value":0},{"from":1660791600000,"to":1660792500000,"method":"L1","value":0},{"from":1660792500000,"to":1660793400000,"method":"L1","value":0},{"from":1660793400000,"to":1660794300000,"method":"L1","value":0},{"from":1660794300000,"to":1660795200000,"method":"L1","value":0},{"from":1660795200000,"to":1660796100000,"method":"L1","value":0},{"from":1660796100000,"to":1660797000000,"method":"L1","value":0},{"from":1660797000000,"to":1660797900000,"method":"L1","value":0},{"from":1660797900000,"to":1660798800000,"method":"L1","value":0},{"from":1660798800000,"to":1660799700000,"method":"L1","value":0},{"from":1660799700000,"to":1660800600000,"method":"L1","value":0},{"from":1660800600000,"to":1660801500000,"method":"L1","value":0},{"from":1660801500000,"to":1660802400000,"method":"L1","value":0},{"from":1660802400000,"to":1660803300000,"method":"L1","value":0},{"from":1660803300000,"to":1660804200000,"method":"L1","value":0},{"from":1660804200000,"to":1660805100000,"method":"L1","value":0},{"from":1660805100000,"to":1660806000000,"method":"L1","value":0.0005},{"from":1660806000000,"to":1660806900000,"method":"L1","value":0},{"from":1660806900000,"to":1660807800000,"method":"L1","value":0},{"from":1660807800000,"to":1660808700000,"method":"L1","value":0},{"from":1660808700000,"to":1660809600000,"method":"L1","value":0},{"from":1660809600000,"to":1660810500000,"method":"L1","value":0},{"from":1660810500000,"to":1660811400000,"method":"L1","value":0},{"from":1660811400000,"to":1660812300000,"method":"L1","value":0},{"from":1660812300000,"to":1660813200000,"method":"L1","value":0},{"from":1660813200000,"to":1660814100000,"method":"L1","value":0},{"from":1660814100000,"to":1660815000000,"method":"L1","value":0},{"from":1660815000000,"to":1660815900000,"method":"L1","value":0},{"from":1660815900000,"to":1660816800000,"method":"L1","value":0},{"from":1660816800000,"to":1660817700000,"method":"L1","value":0},{"from":1660817700000,"to":1660818600000,"method":"L1","value":0},{"from":1660818600000,"to":1660819500000,"method":"L1","value":0},{"from":1660819500000,"to":1660820400000,"method":"L1","value":0},{"from":1660820400000,"to":1660821300000,"method":"L1","value":0},{"from":1660821300000,"to":1660822200000,"method":"L1","value":0},{"from":1660822200000,"to":1660823100000,"method":"L1","value":0},{"from":1660823100000,"to":1660824000000,"method":"L1","value":0},{"from":1660824000000,"to":1660824900000,"method":"L1","value":0},{"from":1660824900000,"to":1660825800000,"method":"L1","value":0},{"from":1660825800000,"to":1660826700000,"method":"L1","value":0},{"from":1660826700000,"to":1660827600000,"method":"L1","value":0},{"from":1660827600000,"to":1660828500000,"method":"L1","value":0},{"from":1660828500000,"to":1660829400000,"method":"L1","value":0},{"from":1660829400000,"to":1660830300000,"method":"L1","value":0},{"from":1660830300000,"to":1660831200000,"method":"L1","value":0},{"from":1660831200000,"to":1660832100000,"method":"L1","value":0},{"from":1660832100000,"to":1660833000000,"method":"L1","value":0},{"from":1660833000000,"to":1660833900000,"method":"L1","value":0},{"from":1660833900000,"to":1660834800000,"method":"L1","value":0},{"from":1660834800000,"to":1660835700000,"method":"L1","value":0},{"from":1660835700000,"to":1660836600000,"method":"L1","value":0},{"from":1660836600000,"to":1660837500000,"method":"L1","value":0},{"from":1660837500000,"to":1660838400000,"method":"L1","value":0},{"from":1660838400000,"to":1660839300000,"method":"L1","value":0},{"from":1660839300000,"to":1660840200000,"method":"L1","value":0},{"from":1660840200000,"to":1660841100000,"method":"L1","value":0},{"from":1660841100000,"to":1660842000000,"method":"L1","value":0},{"from":1660842000000,"to":1660842900000,"method":"L1","value":0},{"from":1660842900000,"to":1660843800000,"method":"L1","value":0},{"from":1660843800000,"to":1660844700000,"method":"L1","value":0},{"from":1660844700000,"to":1660845600000,"method":"L1","value":0},{"from":1660845600000,"to":1660846500000,"method":"L1","value":0},{"from":1660846500000,"to":1660847400000,"method":"L1","value":0},{"from":1660847400000,"to":1660848300000,"method":"L1","value":0},{"from":1660848300000,"to":1660849200000,"method":"L1","value":0},{"from":1660849200000,"to":1660850100000,"method":"L1","value":0},{"from":1660850100000,"to":1660851000000,"method":"L1","value":0},{"from":1660851000000,"to":1660851900000,"method":"L1","value":0},{"from":1660851900000,"to":1660852800000,"method":"L1","value":0},{"from":1660852800000,"to":1660853700000,"method":"L1","value":0},{"from":1660853700000,"to":1660854600000,"method":"L1","value":0},{"from":1660854600000,"to":1660855500000,"method":"L1","value":0},{"from":1660855500000,"to":1660856400000,"method":"L1","value":0},{"from":1660856400000,"to":1660857300000,"method":"L1","value":0},{"from":1660857300000,"to":1660858200000,"method":"L1","value":0},{"from":1660858200000,"to":1660859100000,"method":"L1","value":0},{"from":1660859100000,"to":1660860000000,"method":"L1","value":0}]},{"meterCode":"1-1:2.9.0 G.03","value":[{"from":1660773600000,"to":1660774500000,"method":"L1","value":0},{"from":1660774500000,"to":1660775400000,"method":"L1","value":0},{"from":1660775400000,"to":1660776300000,"method":"L1","value":0},{"from":1660776300000,"to":1660777200000,"method":"L1","value":0},{"from":1660777200000,"to":1660778100000,"method":"L1","value":0},{"from":1660778100000,"to":1660779000000,"method":"L1","value":0},{"from":1660779000000,"to":1660779900000,"method":"L1","value":0},{"from":1660779900000,"to":1660780800000,"method":"L1","value":0},{"from":1660780800000,"to":1660781700000,"method":"L1","value":0},{"from":1660781700000,"to":1660782600000,"method":"L1","value":0},{"from":1660782600000,"to":1660783500000,"method":"L1","value":0},{"from":1660783500000,"to":1660784400000,"method":"L1","value":0},{"from":1660784400000,"to":1660785300000,"method":"L1","value":0},{"from":1660785300000,"to":1660786200000,"method":"L1","value":0},{"from":1660786200000,"to":1660787100000,"method":"L1","value":0},{"from":1660787100000,"to":1660788000000,"method":"L1","value":0},{"from":1660788000000,"to":1660788900000,"method":"L1","value":0},{"from":1660788900000,"to":1660789800000,"method":"L1","value":0},{"from":1660789800000,"to":1660790700000,"method":"L1","value":0},{"from":1660790700000,"to":1660791600000,"method":"L1","value":0},{"from":1660791600000,"to":1660792500000,"method":"L1","value":0},{"from":1660792500000,"to":1660793400000,"method":"L1","value":0},{"from":1660793400000,"to":1660794300000,"method":"L1","value":0},{"from":1660794300000,"to":1660795200000,"method":"L1","value":0},{"from":1660795200000,"to":1660796100000,"method":"L1","value":0},{"from":1660796100000,"to":1660797000000,"method":"L1","value":0},{"from":1660797000000,"to":1660797900000,"method":"L1","value":0},{"from":1660797900000,"to":1660798800000,"method":"L1","value":0},{"from":1660798800000,"to":1660799700000,"method":"L1","value":0},{"from":1660799700000,"to":1660800600000,"method":"L1","value":0},{"from":1660800600000,"to":1660801500000,"method":"L1","value":0},{"from":1660801500000,"to":1660802400000,"method":"L1","value":0},{"from":1660802400000,"to":1660803300000,"method":"L1","value":0},{"from":1660803300000,"to":1660804200000,"method":"L1","value":0},{"from":1660804200000,"to":1660805100000,"method":"L1","value":0},{"from":1660805100000,"to":1660806000000,"method":"L1","value":0.0005},{"from":1660806000000,"to":1660806900000,"method":"L1","value":0},{"from":1660806900000,"to":1660807800000,"method":"L1","value":0},{"from":1660807800000,"to":1660808700000,"method":"L1","value":0},{"from":1660808700000,"to":1660809600000,"method":"L1","value":0},{"from":1660809600000,"to":1660810500000,"method":"L1","value":0},{"from":1660810500000,"to":1660811400000,"method":"L1","value":0},{"from":1660811400000,"to":1660812300000,"method":"L1","value":0},{"from":1660812300000,"to":1660813200000,"method":"L1","value":0},{"from":1660813200000,"to":1660814100000,"method":"L1","value":0},{"from":1660814100000,"to":1660815000000,"method":"L1","value":0},{"from":1660815000000,"to":1660815900000,"method":"L1","value":0},{"from":1660815900000,"to":1660816800000,"method":"L1","value":0},{"from":1660816800000,"to":1660817700000,"method":"L1","value":0},{"from":1660817700000,"to":1660818600000,"method":"L1","value":0},{"from":1660818600000,"to":1660819500000,"method":"L1","value":0},{"from":1660819500000,"to":1660820400000,"method":"L1","value":0},{"from":1660820400000,"to":1660821300000,"method":"L1","value":0},{"from":1660821300000,"to":1660822200000,"method":"L1","value":0},{"from":1660822200000,"to":1660823100000,"method":"L1","value":0},{"from":1660823100000,"to":1660824000000,"method":"L1","value":0},{"from":1660824000000,"to":1660824900000,"method":"L1","value":0},{"from":1660824900000,"to":1660825800000,"method":"L1","value":0},{"from":1660825800000,"to":1660826700000,"method":"L1","value":0},{"from":1660826700000,"to":1660827600000,"method":"L1","value":0},{"from":1660827600000,"to":1660828500000,"method":"L1","value":0},{"from":1660828500000,"to":1660829400000,"method":"L1","value":0},{"from":1660829400000,"to":1660830300000,"method":"L1","value":0},{"from":1660830300000,"to":1660831200000,"method":"L1","value":0},{"from":1660831200000,"to":1660832100000,"method":"L1","value":0},{"from":1660832100000,"to":1660833000000,"method":"L1","value":0},{"from":1660833000000,"to":1660833900000,"method":"L1","value":0},{"from":1660833900000,"to":1660834800000,"method":"L1","value":0},{"from":1660834800000,"to":1660835700000,"method":"L1","value":0},{"from":1660835700000,"to":1660836600000,"method":"L1","value":0},{"from":1660836600000,"to":1660837500000,"method":"L1","value":0},{"from":1660837500000,"to":1660838400000,"method":"L1","value":0},{"from":1660838400000,"to":1660839300000,"method":"L1","value":0},{"from":1660839300000,"to":1660840200000,"method":"L1","value":0},{"from":1660840200000,"to":1660841100000,"method":"L1","value":0},{"from":1660841100000,"to":1660842000000,"method":"L1","value":0},{"from":1660842000000,"to":1660842900000,"method":"L1","value":0},{"from":1660842900000,"to":1660843800000,"method":"L1","value":0},{"from":1660843800000,"to":1660844700000,"method":"L1","value":0},{"from":1660844700000,"to":1660845600000,"method":"L1","value":0},{"from":1660845600000,"to":1660846500000,"method":"L1","value":0},{"from":1660846500000,"to":1660847400000,"method":"L1","value":0},{"from":1660847400000,"to":1660848300000,"method":"L1","value":0},{"from":1660848300000,"to":1660849200000,"method":"L1","value":0},{"from":1660849200000,"to":1660850100000,"method":"L1","value":0},{"from":1660850100000,"to":1660851000000,"method":"L1","value":0},{"from":1660851000000,"to":1660851900000,"method":"L1","value":0},{"from":1660851900000,"to":1660852800000,"method":"L1","value":0},{"from":1660852800000,"to":1660853700000,"method":"L1","value":0},{"from":1660853700000,"to":1660854600000,"method":"L1","value":0},{"from":1660854600000,"to":1660855500000,"method":"L1","value":0},{"from":1660855500000,"to":1660856400000,"method":"L1","value":0},{"from":1660856400000,"to":1660857300000,"method":"L1","value":0},{"from":1660857300000,"to":1660858200000,"method":"L1","value":0},{"from":1660858200000,"to":1660859100000,"method":"L1","value":0},{"from":1660859100000,"to":1660860000000,"method":"L1","value":0}]}]}}`
	msg := model.SubscribeMessage{
		MessageCode: model.EBMS_ENERGY_FILE_RESPONSE,
		Protocol:    model.CR_MSG,
		Tenant:      "TE1000001",
		Payload:     model.EbmsMessage{},
	}
	err = json.Unmarshal([]byte(jsonString), &msg.Payload)
	require.NoError(t, err)

	stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"

	rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
		"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
		"taxNumber", "vatNumber", "online", "contactPerson"}).
		AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
			"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
	mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)

	historyValue := map[string]interface{}{"meter": msg.Payload.Meter.MeteringPoint, "from": msg.Payload.Energy.Start, "to": msg.Payload.Energy.End}
	recorder.Mock.On(
		"saveHistory", "TE1000001", model.EBMS_ENERGY_FILE_RESPONSE, "AT003000202208191420233640008300242", "ADMIN", "IN", model.CR_MSG, historyValue).Return(nil)

	protocolCrMsgHandler(msg, recorder)
	recorder.AssertExpectations(t)
}

func TestProtocolCrReqPtHandler(t *testing.T) {
	type test struct {
		name        string
		message     string
		codes       []string
		messageType model.EbMsMessageType
	}

	tests := []test{
		{
			name:        "Antwort",
			message:     `{"conversationId":"AT003000202208191420233640008300242","messageId":"AT003000202308140722134490185248575","sender":"AT003000","receiver":"RC100298","messageCode":"ANTWORT_PT","meter":{"meteringPoint":"AT0030000000000000000000000446232","direction":"CONSUMPTION"},"responseData":[{"responseCode":[70]}]}`,
			codes:       []string{"Änderung/Anforderung akzeptiert"},
			messageType: model.EBMS_ZP_RES,
		},
		{
			name:        "Ablehnung",
			message:     `{"conversationId":"AT003000202208191420233640008300242","messageId":"AT003000202308140722134490185248575","sender":"AT003000","receiver":"RC100298","messageCode":"ABLEHNUNG_PT","meter":{"meteringPoint":"AT0030000000000000000000000446232","direction":"CONSUMPTION"},"responseData":[{"responseCode":[56]}]}`,
			codes:       []string{"Zählpunkt nicht gefunden"},
			messageType: model.EBMS_ZP_REJ,
		},
		{
			name:        "Anforderung",
			message:     `{"conversationId":"AT003000202208191420233640008300242","messageId":"RC100298202308141691990530000000319","sender":"RC100298","receiver":"AT003000","messageCode":"ANFORDERUNG_PT","requestId":"JOVM6US5","meter":{"meteringPoint":"AT0030000000000000000000000446232","direction":"CONSUMPTION"},"timeline":{"from":1691445600000,"to":1691703900000}}`,
			codes:       []string{},
			messageType: model.EBMS_ZP_SYNC,
		},
	}

	for _, m := range tests {
		t.Run(m.name, func(t *testing.T) {
			var mockDb, err = database.GetDatabaseMock()
			require.NoError(t, err)
			recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}

			msg := model.SubscribeMessage{
				MessageCode: m.messageType,
				Protocol:    model.CR_REQ_PT,
				Tenant:      "TE1000001",
				Payload:     model.EbmsMessage{},
			}
			err = json.Unmarshal([]byte(m.message), &msg.Payload)
			require.NoError(t, err)

			stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"

			rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
				"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
				"taxNumber", "vatNumber", "online", "contactPerson"}).
				AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
					"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
			mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)

			recorder.Mock.On("saveNotification", map[string]interface{}{
				"type":           msg.MessageCode,
				"meteringPoints": msg.Payload.Meters(),
				"responseCodes":  m.codes,
			}, msg.Tenant, "EDA_PROCESS", "ADMIN").Return(nil)
			recorder.Mock.On("saveHistory", "TE1000001", msg.MessageCode, "AT003000202208191420233640008300242", "ADMIN", "IN", msg.Protocol, msg.Payload).Return(nil)

			protocolCrReqPtHandler(msg, recorder)
			recorder.AssertExpectations(t)
		})
	}
}

func TestProtocolEcReqOnlHandler(t *testing.T) {

	type test struct {
		name        string
		prepareMock func() (*RecorderMock, model.SubscribeMessage, sqlmock.Sqlmock)
	}

	tests := []test{
		{
			name: "Anforderung",
			prepareMock: func() (*RecorderMock, model.SubscribeMessage, sqlmock.Sqlmock) {
				mockDb, err := database.GetDatabaseMock()
				require.NoError(t, err)

				recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}
				msg := model.SubscribeMessage{
					MessageCode: model.EBMS_ONLINE_REG_INIT,
					Protocol:    model.EC_REQ_ONL,
					Tenant:      "TE1000001",
					Payload:     model.EbmsMessage{},
				}
				message := `{"conversationId":"RC100298202308171692252620000000321","messageId":"RC100417202402181708275060000001443","sender":"RC100417","receiver":"AT002000","messageCode":"ANFORDERUNG_ECON","requestId":"QLKXKAO4","meter":{"meteringPoint":"AT0030000000000000000000000459143","direction":"CONSUMPTION"},"ecId":"AT00200000000RC100417000000000209"}`
				codes := []string{"0"}
				err = json.Unmarshal([]byte(message), &msg.Payload)
				require.NoError(t, err)

				stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"
				rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
					"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
					"taxNumber", "vatNumber", "online", "contactPerson"}).
					AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
						"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
				mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
				mockDb.Mock.ExpectExec(`UPDATE "base"."meteringpoint" SET (.+"process_state"='INIT'.+)`).WillReturnResult(sqlmock.NewResult(1, 1))

				recorder.Mock.On("saveNotification", map[string]interface{}{
					"type":           msg.MessageCode,
					"meteringPoints": extractMeters(msg.Payload, model.EBMS_ONLINE_REG_COMPLETION),
					"responseCodes":  codes,
				}, msg.Tenant, "EDA_PROCESS", "ADMIN").Return(nil)

				recorder.Mock.On("saveHistory", "TE1000001", msg.MessageCode, "RC100298202308171692252620000000321", "ADMIN", "IN", model.EC_REQ_ONL, msg.Payload).Return(nil)

				return recorder, msg, mockDb.Mock
			},
		},
		{
			name: "Zustimmung",
			prepareMock: func() (*RecorderMock, model.SubscribeMessage, sqlmock.Sqlmock) {
				mockDb, err := database.GetDatabaseMock()
				require.NoError(t, err)

				recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}
				msg := model.SubscribeMessage{
					MessageCode: model.EBMS_ONLINE_REG_APPROVAL,
					Protocol:    model.EC_REQ_ONL,
					Tenant:      "TE1000001",
					Payload:     model.EbmsMessage{},
				}
				message := `{"conversationId":"RC100298202308171692252620000000321","messageId":"AT003000202308170810324070187796715","sender":"AT003000","receiver":"RC100298","messageCode":"ZUSTIMMUNG_ECON","requestId":"XV3VFJN2","responseData":[{"meteringPoint":"AT0030000000000000000000000459143","responseCode":[175]}]}`
				codes := []string{"Zustimmung erteilt"}
				err = json.Unmarshal([]byte(message), &msg.Payload)
				require.NoError(t, err)
				stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"
				rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
					"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
					"taxNumber", "vatNumber", "online", "contactPerson"}).
					AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
						"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
				mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
				//mockDb.Mock.ExpectExec("UPDATE (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
				mockDb.Mock.ExpectExec(
					`UPDATE "base"."meteringpoint" SET ("modifiedAt"='\d\d\d\d-\d\d-\d\dT\d\d:\d\d:\d\dZ',"modifiedBy"='EVU',"process_state"='APPROVED') WHERE`).WillReturnResult(sqlmock.NewResult(1, 1))

				recorder.Mock.On("saveNotification", map[string]interface{}{
					"type":           msg.MessageCode,
					"meteringPoints": extractMeters(msg.Payload, model.EBMS_ONLINE_REG_APPROVAL),
					"responseCodes":  codes,
				}, msg.Tenant, "EDA_PROCESS", "ADMIN").Return(nil)

				recorder.Mock.On("saveHistory", "TE1000001", msg.MessageCode, "RC100298202308171692252620000000321", "ADMIN", "IN", model.EC_REQ_ONL, msg.Payload).Return(nil)

				return recorder, msg, mockDb.Mock
			},
		},
		{
			name: "Zustimmung with consent-Id",
			prepareMock: func() (*RecorderMock, model.SubscribeMessage, sqlmock.Sqlmock) {
				mockDb, err := database.GetDatabaseMock()
				require.NoError(t, err)

				recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}
				msg := model.SubscribeMessage{
					MessageCode: model.EBMS_ONLINE_REG_APPROVAL,
					Protocol:    model.EC_REQ_ONL,
					Tenant:      "TE1000001",
					Payload:     model.EbmsMessage{},
				}
				message := `{"conversationId":"RC100298202308171692252620000000321","messageId":"AT003000202308170810324070187796715","sender":"AT003000","receiver":"RC100298","messageCode":"ZUSTIMMUNG_ECON","requestId":"XV3VFJN2","responseData":[{"meteringPoint":"AT0030000000000000000000000459143", "consentId": "1726617600000","responseCode":[175]}]}`
				codes := []string{"Zustimmung erteilt"}
				err = json.Unmarshal([]byte(message), &msg.Payload)
				require.NoError(t, err)
				stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"
				rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
					"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
					"taxNumber", "vatNumber", "online", "contactPerson"}).
					AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
						"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
				mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
				//mockDb.Mock.ExpectExec(fmt.Sprintf("UPDATE (.+) SET (.+) %s WHERE (.+)", regexp.QuoteMeta(`"process_state"='APPROVED'`))).WillReturnResult(sqlmock.NewResult(1, 1))
				//mockDb.Mock.ExpectExec(fmt.Sprintf(`UPDATE \"base\".\"meteringpoint\" SET (.+) %s WHERE`, regexp.QuoteMeta(`"process_state"='APPROVED'`))).WillReturnResult(sqlmock.NewResult(1, 1))
				mockDb.Mock.ExpectExec(
					`UPDATE "base"."meteringpoint" SET ("consent_id"='1726617600000',"modifiedAt"='\d\d\d\d-\d\d-\d\dT\d\d:\d\d:\d\dZ',"modifiedBy"='EVU',"process_state"='APPROVED') WHERE`).WillReturnResult(sqlmock.NewResult(1, 1))

				recorder.Mock.On("saveNotification", map[string]interface{}{
					"type":           msg.MessageCode,
					"meteringPoints": extractMeters(msg.Payload, model.EBMS_ONLINE_REG_APPROVAL),
					"responseCodes":  codes,
				}, msg.Tenant, "EDA_PROCESS", "ADMIN").Return(nil)

				recorder.Mock.On("saveHistory", "TE1000001", msg.MessageCode, "RC100298202308171692252620000000321", "ADMIN", "IN", model.EC_REQ_ONL, msg.Payload).Return(nil)

				return recorder, msg, mockDb.Mock
			},
		},
		{
			name: "Antwort",
			prepareMock: func() (*RecorderMock, model.SubscribeMessage, sqlmock.Sqlmock) {
				mockDb, err := database.GetDatabaseMock()
				require.NoError(t, err)

				recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}
				msg := model.SubscribeMessage{
					MessageCode: model.EBMS_ONLINE_REG_ANSWER,
					Protocol:    model.EC_REQ_ONL,
					Tenant:      "TE1000001",
					Payload:     model.EbmsMessage{},
				}
				message := `{"conversationId":"RC100298202308171692252620000000321","messageId":"AT003000202307070957427130168201034","sender":"AT003000","receiver":"RC100298","messageCode":"ANTWORT_ECON","requestId":"6P2EU64Z","responseData":[{"meteringPoint":"AT0030000000000000000000000410702","responseCode":[99]}]}`
				codes := []string{"Meldung erhalten"}
				err = json.Unmarshal([]byte(message), &msg.Payload)
				require.NoError(t, err)

				stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"
				rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
					"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
					"taxNumber", "vatNumber", "online", "contactPerson"}).
					AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
						"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
				mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
				mockDb.Mock.ExpectExec("UPDATE (.+)").WillReturnResult(sqlmock.NewResult(1, 1))

				recorder.Mock.On("saveNotification", map[string]interface{}{
					"type":           msg.MessageCode,
					"meteringPoints": extractMeters(msg.Payload, model.EBMS_ONLINE_REG_ANSWER),
					"responseCodes":  codes,
				}, msg.Tenant, "EDA_PROCESS", "ADMIN").Return(nil)

				recorder.Mock.On("saveHistory", "TE1000001", msg.MessageCode, "RC100298202308171692252620000000321", "ADMIN", "IN", model.EC_REQ_ONL, msg.Payload).Return(nil)
				return recorder, msg, mockDb.Mock
			},
		},
		{
			name: "Abschluss",
			prepareMock: func() (*RecorderMock, model.SubscribeMessage, sqlmock.Sqlmock) {
				mockDb, err := database.GetDatabaseMock()
				require.NoError(t, err)

				recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}

				msg := model.SubscribeMessage{
					MessageCode: model.EBMS_ONLINE_REG_COMPLETION,
					Protocol:    model.EC_REQ_ONL,
					Tenant:      "TE1000001",
					Payload:     model.EbmsMessage{},
				}
				//message := `{"conversationId":"RC100298202308171692252620000000321","messageId":"AT003000202308180842215740187694787","sender":"AT003000","receiver":"RC100298","messageCode":"ABSCHLUSS_ECON","meterList":[{"meteringPoint":"AT0030000000000000000000000519928","direction":"CONSUMPTION"}]}`
				message := `{"conversationId":"RC100346202406091843475020000046464","messageId":"AT003300202406292044374080000009571","sender":"AT003300","receiver":"TE1000001","messageCode":"ABSCHLUSS_ECON","messageCodeVersion":"02.00","ecId":"AT00330004600RC100346000000000001","meterList":[{"meteringPoint":"AT0030000000000000000000000519928","direction":"CONSUMPTION","from":1719612000000,"partFact":100, "consentId":"AT1111122222"}]}`
				err = json.Unmarshal([]byte(message), &msg.Payload)
				require.NoError(t, err)
				codes := []string{}

				stmt := `SELECT (.+) FROM "base"."eeg" WHERE (.+)`
				rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
					"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
					"taxNumber", "vatNumber", "online", "contactPerson"}).
					AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
						"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
				mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
				//mockDb.Mock.ExpectExec("UPDATE (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
				mockDb.Mock.ExpectExec(
					`UPDATE "base"."meteringpoint" SET ("activesince"=.+'2024-06-29T00:00:00Z'.,"consent_id"='AT1111122222',"inactivesince"='2999-12-31T00:00:00Z',"modifiedAt"='\d\d\d\d-\d\d-\d\dT\d\d:\d\d:\d\dZ',"modifiedBy"='EVU',"process_state"='ACTIVE',"status"='ACTIVE') WHERE`).WillReturnResult(sqlmock.NewResult(1, 1))

				recorder.Mock.On("saveNotification", map[string]interface{}{
					"type":           msg.MessageCode,
					"meteringPoints": extractMeters(msg.Payload, model.EBMS_ONLINE_REG_COMPLETION),
					"responseCodes":  codes,
				}, msg.Tenant, "EDA_PROCESS", "ADMIN").Return(nil)

				recorder.Mock.On("saveHistory", "TE1000001", msg.MessageCode, "RC100346202406091843475020000046464", "ADMIN", "IN", model.EC_REQ_ONL, msg.Payload).Return(nil)
				return recorder, msg, mockDb.Mock
			},
		},
	}

	for _, m := range tests {
		t.Run(m.name, func(t *testing.T) {
			recorder, msg, mockdb := m.prepareMock()

			protocolEcReqOnlHandler(msg, recorder)
			recorder.AssertExpectations(t)
			mockdb.MatchExpectationsInOrder(true)
			assert.NoError(t, mockdb.ExpectationsWereMet())
		})
	}
}

func TestProtocolCmRevImpHandler(t *testing.T) {
	type test struct {
		name        string
		message     string
		codes       []string
		messageType model.EbMsMessageType
	}

	tests := []test{
		{
			name:        "Aufhebung CCMI",
			message:     `{"conversationId":"AT003000202403310311592520011775087","messageId":"AT003000202403310311592520262459850","sender":"AT003000","receiver":"RC100181","messageCode":"AUFHEBUNG_CCMI","responseData":[{"meteringPoint":"AT0030000000000000000000030042666","responseCode":[1099],"consentEnd":1720994400000}]}`,
			codes:       []string{"1099"},
			messageType: model.EBMS_AUFHEBUNG_CCMI,
		},
	}
	for _, m := range tests {
		t.Run(m.name, func(t *testing.T) {
			var mockDb, err = database.GetDatabaseMock()
			require.NoError(t, err)
			recorder := &RecorderMock{dbOpen: mockDb.OpenMockDb}

			//jsonString := `{"conversationId":"RC100298202308171692252620000000321","messageId":"AT003000202308170810324070187796715","sender":"AT003000","receiver":"RC100298","messageCode":"ZUSTIMMUNG_ECON","requestId":"XV3VFJN2","responseData":[{"meteringPoint":"AT0030000000000000000000000459143","responseCode":[175]}]}`
			msg := model.SubscribeMessage{
				MessageCode: m.messageType,
				Protocol:    model.CM_REV_IMP,
				Tenant:      "TE1000001",
				Payload:     model.EbmsMessage{},
			}
			err = json.Unmarshal([]byte(m.message), &msg.Payload)
			require.NoError(t, err)

			//mockDb.Mock.ExpectBegin()
			//stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"
			//rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
			//	"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
			//	"taxNumber", "vatNumber", "online", "contactPerson"}).
			//	AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
			//		"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
			//mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
			//
			//paRows := sqlmock.NewRows([]string{"participantNumber", "firstname", "lastname", "role", "businessRole", "titleBefore", "titleAfter", "participantSince",
			//	"vatNumber", "taxNumber", "status", "createdBy"}).
			//	AddRow("001", "Max", "Mustermann", "EEG_USER", "EEG_PRIVATE", "", "", time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local),
			//		"1234", "5678", "ACTIVE", "test")
			//mockDb.Mock.ExpectQuery(`SELECT (.+) FROM "base"."participant" WHERE (.+)`).WillReturnRows(paRows)
			//mockDb.Mock.ExpectQuery(`SELECT (.+) FROM "base"."contactdetail" WHERE (.+)`).WillReturnRows(sqlmock.NewRows([]string{"phone"}).AddRow("11"))
			//mockDb.Mock.ExpectQuery(`SELECT (.+) FROM "base"."bankaccount" WHERE (.+)`).WillReturnRows(sqlmock.NewRows([]string{"iban"}).AddRow("11"))
			//mockDb.Mock.ExpectQuery(`SELECT (.+) FROM "base"."address" WHERE (.+)`).WillReturnRows(sqlmock.NewRows([]string{"type"}).AddRow("11"))
			//mockDb.Mock.ExpectQuery(`SELECT (.+) FROM "base"."address" WHERE (.+)`).WillReturnRows(sqlmock.NewRows([]string{"type"}).AddRow("11"))
			//mockDb.Mock.ExpectQuery(`SELECT (.+) FROM "base"."meteringpoint"(.+)`).WillReturnRows(sqlmock.NewRows([]string{"metering_point_id"}).AddRow("1212"))

			mockDb.Mock.ExpectBegin()
			//mockDb.Mock.ExpectExec("UPDATE (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
			mockDb.Mock.ExpectQuery("UPDATE (.+)").WillReturnRows(sqlmock.NewRows([]string{"tenant"}).AddRow("TE1000001"))
			mockDb.Mock.ExpectCommit()
			stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"
			rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
				"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "bankName",
				"taxNumber", "vatNumber", "online", "contactPerson"}).
				AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
					"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "", "", "", "Max Mustermann", false, "Bankname", "", "", false, "Max Mustermann")
			mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)

			recorder.Mock.On("saveNotification", map[string]interface{}{
				"type":           msg.MessageCode,
				"meteringPoints": []string{"AT0030000000000000000000030042666"},
				"responseCodes":  m.codes,
			}, msg.Tenant, "EDA_PROCESS", "ADMIN").Return(nil)

			recorder.Mock.On("saveHistory", "TE1000001", msg.MessageCode, "AT003000202403310311592520011775087", "ADMIN", "IN", model.CM_REV_IMP, msg.Payload).Return(nil)

			protocolCmRevImpHandler(msg, recorder)
			recorder.AssertExpectations(t)

		})
	}

}
