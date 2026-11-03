// Package configtest sets configuration keys for one test. Only tests import it.
package configtest

import (
	"testing"

	"github.com/spf13/viper"
)

// Set sets the viper key for the running test and puts the previous value back when the test
// ends, so no test leaves its value to the next (the order of the tests does not matter).
func Set(t testing.TB, key string, value interface{}) {
	t.Helper()
	old := viper.Get(key)
	viper.Set(key, value)
	t.Cleanup(func() { viper.Set(key, old) })
}
