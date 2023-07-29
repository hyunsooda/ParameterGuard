package pass_test

import (
	"testing"

	"github.com/hyunsooda/unsafeuse/pass"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestChecker(t *testing.T) {
	testdata := analysistest.TestData()
	pass.InitTest()
	pass.Init()

	tcs := []string{"slice", "pointer", "map", "interface", "struct"}
	analysistest.Run(t, testdata, pass.Analyzer, tcs...)
}
