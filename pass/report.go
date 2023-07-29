package pass

import (
	"fmt"
	"go/token"
	"go/types"
	"sync"

	"github.com/fatih/color"
	"golang.org/x/tools/go/analysis"
)

type ReportMsg struct {
	pos token.Pos
	msg string
}

var (
	ReportRWMutex = new(sync.RWMutex)
	Reports       = make(map[token.Pos][]ReportMsg)
	ReportIdx     = 0
)

func newReportMsg(pos token.Pos, msg string) ReportMsg {
	return ReportMsg{pos, msg}
}

func addReports(pass *analysis.Pass, fnDecl types.Object, violatedUses []*ParamUsage) {
	for _, violatedUse := range violatedUses {
		declPos, usePos := violatedUse.declaredAt, violatedUse.useAt.Pos()
		declLoc, useLoc := pass.Fset.Position(declPos), pass.Fset.Position(usePos)

		paramName := violatedUse.param.Name()
		violatedAtContext := ""
		if violatedUse.context != nil {
			violatedAtContext = fmt.Sprintf("(member: '%s')", violatedUse.param.Name())
			paramName = violatedUse.context.Name()
		}

		idx := color.New(color.FgRed).Sprintf("%4d", ReportIdx)
		declMsg := fmt.Sprintf("[%s] Declared '%s' at -> %s", idx, paramName, declLoc)
		prefix := "Unsafely used"
		useMsg := fmt.Sprintf("  --> %s '%s' %s at -> %s", prefix, paramName, violatedAtContext, useLoc)

		ReportRWMutex.Lock()
		if len(Reports[declPos]) == 0 {
			Reports[declPos] = append(Reports[declPos], newReportMsg(declPos, declMsg))
			ReportIdx++
		}
		Reports[declPos] = append(Reports[declPos], newReportMsg(usePos, useMsg))
		ReportRWMutex.Unlock()
	}
}

func printReports(pass *analysis.Pass) {
	ReportRWMutex.RLock()
	defer ReportRWMutex.RUnlock()

	for _, reportByDecl := range Reports {
		for _, report := range reportByDecl {
			if !Testing.on {
				pass.Reportf(0, report.msg)
			}
		}
	}
}
