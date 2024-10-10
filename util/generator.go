package util

import (
	"fmt"
	"math/rand"
	"time"
)

func NewMessageId(ecnumber string) string {
	t_now := time.Now()
	n_rand := rand.Intn(9999999999-100) + 100
	return fmt.Sprintf("%s%.04d%.02d%.02d%.02d%.02d%.02d%.03d%.010d", ecnumber,
		t_now.Year(), t_now.Month(), t_now.Day(),
		t_now.Hour(), t_now.Minute(), t_now.Second(), int64(int64(t_now.Nanosecond())/int64(time.Millisecond)),
		n_rand)
}
