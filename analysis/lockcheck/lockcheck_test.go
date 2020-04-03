package lockcheck_test

import (
	"testing"

	"gitlab.com/scpcorp/ScPrime/analysis/lockcheck"

	"golang.org/x/tools/go/analysis/analysistest"
)

func Test(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), lockcheck.Analyzer, "a")
}
