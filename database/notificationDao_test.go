package database

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFetchEdaHistory(t *testing.T) {
	var mockDb, err = openDb()
	require.NoError(t, err)

	res, err := FetchEdaHistory(mockDb.mockDb, "RC100298")
	require.NoError(t, err)

	for k, v := range res {
		fmt.Printf("K: %v\n", k)
		for _, e := range v {
			fmt.Printf("    V: %v\n", e)
		}
	}
}
