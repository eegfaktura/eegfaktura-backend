package model

type VfeegError struct {
	Code int
	Err  error
}

func (r *VfeegError) Error() string {
	return r.Err.Error()
}

func Wrap(err error, code int) *VfeegError {
	return &VfeegError{
		Code: code,
		Err:  err,
	}
}

func PartialWrap(code int) func(err error) *VfeegError {
	return func(err error) *VfeegError {
		return &VfeegError{
			Code: code,
			Err:  err,
		}
	}
}

var ErrParseJson = PartialWrap(2000)

var ErrConnectDatabase = PartialWrap(999)
var ErrOpenTx = PartialWrap(998)

var ErrGetEeg = PartialWrap(1000)

var ErrGetParticipant = PartialWrap(1100)
var ErrCompleteParticipant = PartialWrap(1101)
var ErrUpdateParticipant = PartialWrap(1102)
var ErrInsertParticipant = PartialWrap(1103)
var ErrRegisterParticipant = PartialWrap(1104)
var ErrArchiveParticipant = PartialWrap(1105)
var ErrFindParticipant = PartialWrap(1106)

var ErrRemoveMeteringPoint = PartialWrap(1201)
var ErrStatusMeter = PartialWrap(1202)
var ErrSaveMeteringPoint = PartialWrap(1203)

var ErrEdaCommunication = PartialWrap(5000)
var ErrRequestEnergyData = PartialWrap(5100)
var ErrRevokeMeter = PartialWrap(5010)

var ErrFindMeter = PartialWrap(3100)
var ErrUpdateMeter = PartialWrap(3101)

var ErrGetTariff = PartialWrap(4000)
var ErrUpdateTariff = PartialWrap(4001)
var ErrTariffUtilized = PartialWrap(4002)
