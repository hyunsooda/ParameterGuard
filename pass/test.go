package pass

import (
	"fmt"

	"golang.org/x/tools/go/analysis"
)

func reportOnTest(pass *analysis.Pass, unsanitized []*ParamUsage) {
	if Testing.on {
		Testing.lock.Lock()
		for _, violated := range unsanitized {
			declVar := fmt.Sprintf("Declared '%s'", violated.param.Name())
			useVar := fmt.Sprintf("Unsafely used '%s'", violated.param.Name())

			declVarWithPos := fmt.Sprintf("%d-%s", violated.declaredAt, declVar)
			useVarWithPos := fmt.Sprintf("%d-%s", violated.useAt.Pos(), useVar)

			if !Testing.reportedMsgs[declVarWithPos] {
				pass.Reportf(violated.declaredAt, declVar)
				Testing.reportedMsgs[declVarWithPos] = true
			}
			if !Testing.reportedMsgs[useVarWithPos] {
				pass.Reportf(violated.useAt.Pos(), useVar)
				Testing.reportedMsgs[useVarWithPos] = true
			}

		}
		Testing.lock.Unlock()
	}
}
