package internal

import (
	"context"
	"testing"

	test2 "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/util/test"
)

func TestCatchupServiceCatchup_Errors(t *testing.T) {
	logger, _ := test2.NewNullLogger()
	genesis := test.MakeGenesis()

	err := CatchupServiceCatchup(context.Background(), logger, "", "", genesis)
	require.ErrorContains(t, err, "catchpoint missing")

	err = CatchupServiceCatchup(context.Background(), logger, "1234#aaaaa", "", genesis)
	require.ErrorContains(t, err, "invalid catchpoint err")
}
