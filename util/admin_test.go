package util

import (
	"bytes"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"testing"
)

func init() {
	viper.Set("services.mail-server", "localhost:9092")
}

func TestSendMail(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("Hallo")
	b.WriteString("Jürgen")

	err := SendMail("tenant", "obermueller.peter@gmail.com", "Ihre Anmeldung", &b, nil, nil)
	require.NoError(t, err)
}
