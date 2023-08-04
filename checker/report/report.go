package report

import (
	"fmt"
	"go/token"
	"go/types"
	"sync"

	"github.com/fatih/color"
	"github.com/hyunsooda/paramguard/checker/callgraph"
	"github.com/hyunsooda/paramguard/checker/passtyps"
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

func AddReports(pass *analysis.Pass, fnDecl types.Object, violatedUses []*passtyps.ParamUsage) {
	callgraph.InitCallGraph(pass)
	for _, violatedUse := range violatedUses {
		declPos, usePos := violatedUse.DeclaredAt, violatedUse.UseAt.Pos()
		declLoc, useLoc := pass.Fset.Position(declPos), pass.Fset.Position(usePos)

		paramName := violatedUse.Param.Name()
		violatedAtContext := ""
		if violatedUse.Context != nil {
			violatedAtContext = fmt.Sprintf("(member: '%s')", violatedUse.Param.Name())
			paramName = violatedUse.Context.Name()
		}

		idx := color.New(color.FgRed).Sprintf("%4d", ReportIdx)
		funcFullName := violatedUse.Fn.FullName()
		declMsg := fmt.Sprintf("[%s] Declared '%s' at %s -> %s", idx, paramName, funcFullName, declLoc)
		prefix := "Unsafely used"
		useMsg := fmt.Sprintf("  --> %s '%s' %s at -> %s", prefix, paramName, violatedAtContext, useLoc)
		cgpInfo := ""
		if callgraph.CGP != nil {
			if callPaths, ok := callgraph.CGP[funcFullName]; ok {
				cgpInfo = fmt.Sprintf("  ==> Feasible Callgraph path => %s", callPaths)
			}
		}

		ReportRWMutex.Lock()
		if len(Reports[declPos]) == 0 {
			Reports[declPos] = append(Reports[declPos], newReportMsg(declPos, declMsg))
			ReportIdx++
		}
		if cgpInfo != "" {
			Reports[declPos] = append(Reports[declPos], newReportMsg(usePos, cgpInfo))
		}
		Reports[declPos] = append(Reports[declPos], newReportMsg(usePos, useMsg))
		ReportRWMutex.Unlock()
	}
}

func PrintReports(pass *analysis.Pass) {
	ReportRWMutex.RLock()
	defer ReportRWMutex.RUnlock()

	for _, reportByDecl := range Reports {
		for _, report := range reportByDecl {
			if !passtyps.Testing.On {
				pass.Reportf(0, report.msg)
			}
		}
	}
}
