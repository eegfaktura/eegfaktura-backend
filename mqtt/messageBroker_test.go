package mqttclient

import (
	"testing"
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
