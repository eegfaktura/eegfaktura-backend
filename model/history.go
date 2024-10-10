package model

import (
	"gopkg.in/guregu/null.v4"
	"time"
)

type EdaProcessHistory struct {
	Tenant         string                 `db:"tenant"`
	ConversationId string                 `json:"conversationId" db:"conversationId"`
	ProcessType    EbMsMessageType        `json:"processType" db:"type"`
	Date           time.Time              `json:"date" goqu:"skipinsert,defaultifempty"`
	Protocol       null.String            `json:"protocol"`
	Issuer         string                 `json:"issuer"`
	MessageByte    []byte                 `json:"-" db:"message"`
	MessageMap     map[string]interface{} `json:"message" db:"-"`
	Direction      string                 `json:"direction"`
}
