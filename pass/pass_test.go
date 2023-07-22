package pass_test

import (
	"testing"

	"github.com/hyunsooda/mylinter/pass"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestNilDerefBytes(t *testing.T) {
	testdata := analysistest.TestData()
	pass.TestOn = true
	pass.Init()
	// analysistest.Run(t, testdata, pass.Analyzer, "pointer")

	tcs := []string{"slice", "pointer", "map", "interface", "struct"}
	analysistest.Run(t, testdata, pass.Analyzer, tcs...)
}
