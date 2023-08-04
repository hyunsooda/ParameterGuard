package test

import (
	"fmt"

	"github.com/hyunsooda/paramguard/checker/passtyps"
	"golang.org/x/tools/go/analysis"
)

func ReportOnTest(pass *analysis.Pass, unsanitized []*passtyps.ParamUsage) {
	if passtyps.Testing.On {
		passtyps.Testing.Lock()
		for _, violated := range unsanitized {
			declVar := fmt.Sprintf("Declared '%s'", violated.Param.Name())
			useVar := fmt.Sprintf("Unsafely used '%s'", violated.Param.Name())

			declVarWithPos := fmt.Sprintf("%d-%s", violated.DeclaredAt, declVar)
			useVarWithPos := fmt.Sprintf("%d-%s", violated.UseAt.Pos(), useVar)

			if !passtyps.Testing.ReportedMsgs[declVarWithPos] {
				pass.Reportf(violated.DeclaredAt, declVar)
				passtyps.Testing.ReportedMsgs[declVarWithPos] = true
			}
			if !passtyps.Testing.ReportedMsgs[useVarWithPos] {
				pass.Reportf(violated.UseAt.Pos(), useVar)
				passtyps.Testing.ReportedMsgs[useVarWithPos] = true
			}

		}
		passtyps.Testing.Unlock()
	}
}
