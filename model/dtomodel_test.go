package model

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestStandardizeMeteringPointList(t *testing.T) {

	jsonRaw, err := os.ReadFile("../tests/ZP_LIST_multi_partfact.json")
	require.NoError(t, err)

	msg := EbmsMessage{}
	err = json.Unmarshal(jsonRaw, &msg)
	require.NoError(t, err)

	m := StandardizeMeteringPointList(msg.MeterList)
	assert.Equal(t, 5, len(m))
	for _, meter := range m {
		fmt.Printf("Meter: %+v\n", meter)
	}
}

func TestRC100181Meter(t *testing.T) {

	jsonRaw, err := os.ReadFile("../tests/ZP_LIST_RC100181.json")
	require.NoError(t, err)

	msg := EbmsMessage{}
	err = json.Unmarshal(jsonRaw, &msg)
	require.NoError(t, err)

	for _, meter := range msg.MeterList {
		fmt.Printf("Meter;%s;%s;%s;\n", meter.MeteringPoint, meter.Direction, time.UnixMilli(meter.Activation).Format("2006-01-02 15:04:05"))
	}

}
