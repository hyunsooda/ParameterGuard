package passes

import (
	"flag"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/hyunsooda/paramguard/checker/common"
	"github.com/hyunsooda/paramguard/checker/passes/test"
	"github.com/hyunsooda/paramguard/checker/passtyps"
	"github.com/hyunsooda/paramguard/checker/report"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var MainAnalyzer = &analysis.Analyzer{
	Doc:      "Perform static analysis on Go source files to identify unsafe practices, such as nil dereferences, using a heuristic-based approach.",
	Name:     "paramguard",
	Run:      run,
	Requires: []*analysis.Analyzer{inspect.Analyzer, ParamCollector},
}

func Init() {
	customFlags := flag.NewFlagSet("unsafeuse-flags", flag.ExitOnError)
	customFlags.String(passtyps.FLAG_CONFIG_FILE_PATH, "", "Set the configuration file path (default=none)")
	MainAnalyzer.Flags = *customFlags
	ParamCollector.Flags = *customFlags
}

func run(pass *analysis.Pass) (interface{}, error) {
	config := passtyps.ParseConfig(pass)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	funcParams := *pass.ResultOf[ParamCollector].(*passtyps.FuncParams)
	filterNodes := []ast.Node{
		(*ast.FuncDecl)(nil),
	}
	insp.Preorder(filterNodes, func(n ast.Node) {
		if fnDecl, ok := n.(*ast.FuncDecl); ok {
			if !passtyps.IsInExcludes(pass, fnDecl, config) {
				if fn, ok := pass.TypesInfo.Defs[fnDecl.Name]; ok {
					interestingParams := funcParams[fn].Params
					if len(interestingParams) > 0 {
						ctx := passtyps.NewContext(pass, interestingParams, funcParams[fn].TypCollection)
						unsanitized := runBlk(ctx, fnDecl.Body, fn.(*types.Func))
						report.AddReports(pass, fn, unsanitized)
						test.ReportOnTest(pass, unsanitized)
					}
				}
			}
		}
	})
	report.PrintReports(pass)
	return nil, nil
}

func runBlk(ctx passtyps.Context, blkStmt *ast.BlockStmt, fn *types.Func) []*passtyps.ParamUsage {
	var (
		uses         []*passtyps.ParamUsage
		guards       []*passtyps.ParamUsage
		sanitizedUse = make(map[*passtyps.ParamUsage]bool)
		unsanitized  []*passtyps.ParamUsage
	)
	if blkStmt != nil {
		ast.Inspect(blkStmt, func(n ast.Node) bool {
			paramUsages := runExpr(ctx, n)
			for _, usage := range paramUsages {
				if usage != nil {
					if usage.GuardAt != nil {
						guards = append(guards, usage)
					}
					if usage.UseAt != nil {
						uses = append(uses, usage)
					}

				}
			}
			return true
		})
	}

	for _, guard := range guards {
		for _, use := range uses {
			if guard.Param.Id() == use.Param.Id() &&
				guard.GuardAt.Pos() <= use.UseAt.Pos() {
				// violated: guard expression must appear before the use expression
				sanitizedUse[use] = true
			}
		}
	}
	for _, use := range uses {
		asserted := false
		for sanitized := range sanitizedUse {
			if use == sanitized {
				asserted = true
				break
			}
		}
		if !asserted {
			use.Fn = fn
			unsanitized = append(unsanitized, use)
		}
	}
	return unsanitized
}

func runExpr(ctx passtyps.Context, n ast.Node) []*passtyps.ParamUsage {
	switch expr := n.(type) {
	case *ast.CallExpr:
		if fnIdent := common.IsTargetedParam(ctx, expr.Fun); fnIdent != nil {
			return []*passtyps.ParamUsage{passtyps.NewParamUsage(fnIdent, nil, expr, fnIdent.Pos())}
		}
	case *ast.BinaryExpr:
		return runBinaryExpr(ctx, expr)
	case *ast.TypeSwitchStmt:
		return runTypSwitchStmt(ctx, expr)
	case *ast.StarExpr:
		children := common.GetSelectorExprChildren(expr.X)
		// depth n: e.g., *s.member1.member2
		if len(children) > 0 {
			mostParentExpr := children[len(children)-1].X
			lastChildIdent := children[0].Sel
			if v := common.IsTargetedParam(ctx, mostParentExpr); v != nil {
				for _, innerChildren := range ctx.TypCollection {
					for ident, typ := range innerChildren {
						if ident.Name == lastChildIdent.Name {
							if _, ok := typ.Type().Underlying().(*types.Pointer); ok {
								usage := ctx.Pass.TypesInfo.Uses[lastChildIdent]
								paramUsage := passtyps.NewParamUsage(usage, nil, expr, v.Pos())
								paramUsage.Context = v
								return []*passtyps.ParamUsage{paramUsage}
							}
						}
					}
				}
			}
		}

		// depth 1: e.g., *v
		if v := common.IsTargetedParam(ctx, expr); v != nil {
			return []*passtyps.ParamUsage{passtyps.NewParamUsage(v, nil, expr, v.Pos())}
		}
	case *ast.SelectorExpr: // rule: if a struct is pointer && access its member
		// depth n: e.g., s.member1.member2
		if paramUsage := runSelectorExprTree(ctx, expr, true); paramUsage != nil {
			return paramUsage
		}

		// depth 1: e.g., s.member
		if v := common.IsTargetedParam(ctx, expr.X); v != nil {
			switch v.Type().Underlying().(type) {
			case *types.Pointer, *types.Interface:
				return []*passtyps.ParamUsage{passtyps.NewParamUsage(v, nil, expr, v.Pos())}
			}
		}
	case *ast.SliceExpr:
		if v := common.IsTargetedParam(ctx, expr.X); v != nil {
			return []*passtyps.ParamUsage{passtyps.NewParamUsage(v, nil, expr, v.Pos())}
		}
	case *ast.IndexExpr:
		if v := common.IsTargetedParam(ctx, expr.X); v != nil {
			return []*passtyps.ParamUsage{passtyps.NewParamUsage(v, nil, expr, v.Pos())}
		}
	}
	return nil
}

func lenCompGuard(ctx passtyps.Context, expr ast.Expr) *passtyps.ParamUsage {
	if callExpr, isCallExpr := expr.(*ast.CallExpr); isCallExpr {
		if fnIdent := common.Cast2Ident(callExpr); fnIdent != nil && fnIdent.Name == "len" {
			if lenParam := common.IsTargetedParam(ctx, callExpr.Args[0]); lenParam != nil {
				return passtyps.NewParamUsage(lenParam, expr, nil, lenParam.Pos())
			}
		}
	}
	return nil
}

func runBinaryExpr(ctx passtyps.Context, binaryExpr *ast.BinaryExpr) []*passtyps.ParamUsage {
	// Guard Definition 1
	lhs, op, rhs := binaryExpr.X, binaryExpr.Op, binaryExpr.Y

	// N detpth
	if paramUsages := runSelectorExprTree(ctx, lhs, false); paramUsages != nil {
		for _, usage := range paramUsages {
			usage.GuardAt = binaryExpr
		}
		return paramUsages
	}
	if paramUsages := runSelectorExprTree(ctx, rhs, false); paramUsages != nil {
		for _, usage := range paramUsages {
			usage.GuardAt = binaryExpr
		}
		return paramUsages
	}

	lchildren := common.GetSelectorExprChildren(lhs)
	if len(lchildren) > 0 {
		lhs = lchildren[len(lchildren)-1].X
	}
	rchildren := common.GetSelectorExprChildren(rhs)
	if len(rchildren) > 0 {
		rhs = rchildren[len(rchildren)-1].X
	}
	lhsV := common.IsTargetedParam(ctx, lhs)
	rhsV := common.IsTargetedParam(ctx, rhs)

	if op == token.EQL || op == token.NEQ {
		// 0 or 1 depth
		if lhsV != nil {
			if rhsIdent := common.Cast2Ident(rhs); rhsIdent != nil && rhsIdent.Name == "nil" {
				paramUsage := passtyps.NewParamUsage(lhsV, binaryExpr, nil, lhsV.Pos())
				if len(lchildren) > 0 {
					paramUsage.Param = ctx.Pass.TypesInfo.Uses[lchildren[0].Sel]
				}
				return []*passtyps.ParamUsage{paramUsage}
			}
		}
		if rhsV != nil {
			if lhsIdent := common.Cast2Ident(lhs); lhsIdent != nil && lhsIdent.Name == "nil" {
				paramUsage := passtyps.NewParamUsage(rhsV, binaryExpr, nil, rhsV.Pos())
				if len(rchildren) > 0 {
					paramUsage.Param = ctx.Pass.TypesInfo.Uses[rchildren[0].Sel]
				}
				return []*passtyps.ParamUsage{paramUsage}
			}
		}
	}

	// Guard Definition 2
	if op == token.EQL || op == token.NEQ || op == token.LEQ || op == token.GEQ || op == token.LSS || op == token.GTR {
		if lenParamUsage := lenCompGuard(ctx, lhs); lenParamUsage != nil {
			if common.IsSliceTyp(lenParamUsage.Param) {
				return []*passtyps.ParamUsage{lenParamUsage}
			}
		}
		if lenParamUsage := lenCompGuard(ctx, rhs); lenParamUsage != nil {
			if common.IsSliceTyp(lenParamUsage.Param) {
				return []*passtyps.ParamUsage{lenParamUsage}
			}
		}
	}
	return nil
}

func runTypSwitchStmt(ctx passtyps.Context, typSwitchStmt *ast.TypeSwitchStmt) []*passtyps.ParamUsage {
	// Guard Definition 3
	if assignExpr, ok := typSwitchStmt.Assign.(*ast.ExprStmt); ok {
		if typAssertExpr, ok := assignExpr.X.(*ast.TypeAssertExpr); ok {

			// depth n: e.g., s.member1.member2.(type)
			if paramUsage := runSelectorExprTree(ctx, typAssertExpr.X, false); paramUsage != nil {
				return paramUsage
			}

			// depth 1: e.g., itf.(type)
			if v := common.IsTargetedParam(ctx, typAssertExpr.X); v != nil {
				if _, ok := v.Type().Underlying().(*types.Interface); ok {
					return []*passtyps.ParamUsage{passtyps.NewParamUsage(v, typSwitchStmt, nil, v.Pos())}
				}
			}
		}
	}
	return nil
}

func runSelectorExprTree(ctx passtyps.Context, expr ast.Expr, use bool) []*passtyps.ParamUsage {
	children := common.GetSelectorExprChildren(expr)
	if len(children) == 0 {
		return nil
	}

	mostParentExpr := children[len(children)-1].X
	lastChildIdent := children[0].Sel

	var usages []*passtyps.ParamUsage
	if v := common.IsTargetedParam(ctx, mostParentExpr); v != nil {
		mostParentIdent := common.Cast2Ident(mostParentExpr)
		mostParentTyp := common.UnwrapPtrTyp(ctx.Pass.TypesInfo.Uses[mostParentIdent].Type())
		for typ, innerChildren := range ctx.TypCollection {
			if typ == mostParentTyp {
				for ident, typ := range innerChildren {
					switch typ.Type().Underlying().(type) {
					case *types.Pointer, *types.Interface:
						start := 0
						if use {
							// Skip the first child, which is the last property (e.g., a.b.c.last)
							start = 1
						}
						for i := start; i < len(children); i++ {
							if children[i].Sel.Name == ident.Name {
								usage := ctx.Pass.TypesInfo.Uses[children[i].Sel]
								if usage.Type() == typ.Type() {
									if use {
										paramUsage := passtyps.NewParamUsage(usage, nil, expr, v.Pos())
										paramUsage.Context = v
										usages = append(usages, paramUsage)
									} else {
										if lastChildIdent.Name == ident.Name {
											paramUsage := passtyps.NewParamUsage(usage, expr, nil, v.Pos())
											paramUsage.Context = v
											usages = append(usages, paramUsage)
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return usages
}
