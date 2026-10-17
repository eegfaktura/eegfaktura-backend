package mqttclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageBroker_command(t *testing.T) {

	type args struct {
		cmd CommandMessage
	}
	tests := []struct {
		name string
		args args
	}{
		{name: "Sepp", args: args{cmd: CommandMessage{
			tenant: "TE100100",
			cmd:    "pontononlinestate",
			msg:    []byte(`{"online": true}`),
		}}},
	}
	//repository.InitRepositories()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MessageBroker{}
			m.command(tt.args.cmd)
		})
	}
}

func TestMessageBroker_command_malformedOnlineState(t *testing.T) {
	payloads := map[string]string{
		"string": `{"online": "true"}`,
		"null":   `{"online": null}`,
		"number": `{"online": 1}`,
		"object": `{"online": {}}`,
		"array":  `[true]`,
		"noJson": `online`,
	}
	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			m := &MessageBroker{}
			require.NotPanics(t, func() {
				m.command(CommandMessage{tenant: "TE100100", cmd: "pontononlinestate", msg: []byte(payload)})
			})
		})
	}
}
