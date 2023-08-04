package passes_test

import (
	"testing"

	"github.com/hyunsooda/paramguard/checker/passes"
	"github.com/hyunsooda/paramguard/checker/passtyps"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestChecker(t *testing.T) {
	testdata := analysistest.TestData()
	passtyps.InitTest()
	passes.Init()

	tcs := []string{"slice", "pointer", "map", "interface", "struct"}
	analysistest.Run(t, testdata, passes.MainAnalyzer, tcs...)
}
