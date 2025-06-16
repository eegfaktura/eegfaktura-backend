package util

import (
	"github.com/jjeffery/civil"
	"testing"
)

func TestCalcProcessDate(t *testing.T) {
	type args struct {
		date civil.Date
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{
			name: "Process Date lesser",
			args: args{
				date: civil.Today().AddDate(0, 0, -2),
			},
			want: civil.Today().AddDate(0, 0, 1).Unix() * 1000,
		},
		{
			name: "Process Date greater",
			args: args{
				date: civil.Today().AddDate(0, 0, 2),
			},
			want: civil.Today().AddDate(0, 0, 2).Unix() * 1000,
		},
		{
			name: "Process Date equal",
			args: args{
				date: civil.Today(),
			},
			want: civil.Today().AddDate(0, 0, 1).Unix() * 1000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalcProcessDate(tt.args.date); got != tt.want {
				t.Errorf("CalcProcessDate() = %v, want %v", got, tt.want)
			}
		})
	}
}
