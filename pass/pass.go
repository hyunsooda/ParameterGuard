package pass

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var (
	Testing Test

	MainAnalyzer = &analysis.Analyzer{
		Doc:      "Perform static analysis on Go source files to identify unsafe practices, such as nil dereferences, using a heuristic-based approach.",
		Name:     "paramguard",
		Run:      run,
		Requires: []*analysis.Analyzer{inspect.Analyzer, ParamCollector},
	}
)

func run(pass *analysis.Pass) (interface{}, error) {
	config := parseConfig(pass)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	funcParams := *pass.ResultOf[ParamCollector].(*FuncParams)
	filterNodes := []ast.Node{
		(*ast.FuncDecl)(nil),
	}
	insp.Preorder(filterNodes, func(n ast.Node) {
		if fnDecl, ok := n.(*ast.FuncDecl); ok {
			if !isInExcludes(pass, fnDecl, config) {
				if fn, ok := pass.TypesInfo.Defs[fnDecl.Name]; ok {
					// sig := fn.Type().(*types.Signature)
					// interestingParams := getNilableParams(sig.Params())
					// namedTyps := aggregateNamedTyps(pass)
					// typCollection := getAllInnerTyps(pass, nil, interestingParams, namedTyps)

					interestingParams := funcParams[fn].params
					if len(interestingParams) > 0 {
						// ctx := NewContext(pass, interestingParams, typCollection)
						ctx := NewContext(pass, interestingParams, funcParams[fn].typCollection)
						unsanitized := runBlk(ctx, fnDecl.Body)
						addReports(pass, fn, unsanitized)
						reportOnTest(pass, unsanitized)
					}
				}
			}
		}
	})
	printReports(pass)
	return nil, nil
}

func runBlk(ctx Context, blkStmt *ast.BlockStmt) []*ParamUsage {
	var (
		uses         []*ParamUsage
		guards       []*ParamUsage
		sanitizedUse = make(map[*ParamUsage]bool)
		unsanitized  []*ParamUsage
	)
	if blkStmt != nil {
		ast.Inspect(blkStmt, func(n ast.Node) bool {
			paramUsages := runExpr(ctx, n)
			for _, usage := range paramUsages {
				if usage != nil {
					if usage.guardAt != nil {
						guards = append(guards, usage)
					}
					if usage.useAt != nil {
						uses = append(uses, usage)
					}

				}
			}
			return true
		})
	}

	for _, guard := range guards {
		for _, use := range uses {
			if guard.param.Id() == use.param.Id() &&
				guard.guardAt.Pos() <= use.useAt.Pos() {
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
			unsanitized = append(unsanitized, use)
		}
	}
	return unsanitized
}

func runExpr(ctx Context, n ast.Node) []*ParamUsage {
	switch expr := n.(type) {
	case *ast.CallExpr:
		if fnIdent := isTargetedParam(ctx, expr.Fun); fnIdent != nil {
			return []*ParamUsage{NewParamUsage(fnIdent, nil, expr, fnIdent.Pos())}
		}
	case *ast.BinaryExpr:
		return runBinaryExpr(ctx, expr)
	case *ast.TypeSwitchStmt:
		return runTypSwitchStmt(ctx, expr)
	case *ast.StarExpr:
		children := getSelectorExprChildren(expr.X)
		// depth n: e.g., *s.member1.member2
		if len(children) > 0 {
			mostParentExpr := children[len(children)-1].X
			lastChildIdent := children[0].Sel
			if v := isTargetedParam(ctx, mostParentExpr); v != nil {
				for _, innerChildren := range ctx.typCollection {
					for ident, typ := range innerChildren {
						if ident.Name == lastChildIdent.Name {
							if _, ok := typ.Type().Underlying().(*types.Pointer); ok {
								usage := ctx.pass.TypesInfo.Uses[lastChildIdent]
								paramUsage := NewParamUsage(usage, nil, expr, v.Pos())
								paramUsage.context = v
								return []*ParamUsage{paramUsage}
							}
						}
					}
				}
			}
		}

		// depth 1: e.g., *v
		if v := isTargetedParam(ctx, expr); v != nil {
			return []*ParamUsage{NewParamUsage(v, nil, expr, v.Pos())}
		}
	case *ast.SelectorExpr: // rule: if a struct is pointer && access its member
		// depth n: e.g., s.member1.member2
		if paramUsage := runSelectorExprTree(ctx, expr, true); paramUsage != nil {
			return paramUsage
		}

		// depth 1: e.g., s.member
		if v := isTargetedParam(ctx, expr.X); v != nil {
			switch v.Type().Underlying().(type) {
			case *types.Pointer, *types.Interface:
				return []*ParamUsage{NewParamUsage(v, nil, expr, v.Pos())}
			}
		}
	case *ast.SliceExpr:
		if v := isTargetedParam(ctx, expr.X); v != nil {
			return []*ParamUsage{NewParamUsage(v, nil, expr, v.Pos())}
		}
	case *ast.IndexExpr:
		if v := isTargetedParam(ctx, expr.X); v != nil {
			return []*ParamUsage{NewParamUsage(v, nil, expr, v.Pos())}
		}
	}
	return nil
}

func lenCompGuard(ctx Context, expr ast.Expr) *ParamUsage {
	if callExpr, isCallExpr := expr.(*ast.CallExpr); isCallExpr {
		if fnIdent := cast2Ident(callExpr); fnIdent != nil && fnIdent.Name == "len" {
			if lenParam := isTargetedParam(ctx, callExpr.Args[0]); lenParam != nil {
				return NewParamUsage(lenParam, expr, nil, lenParam.Pos())
			}
		}
	}
	return nil
}

func runBinaryExpr(ctx Context, binaryExpr *ast.BinaryExpr) []*ParamUsage {
	// Guard Definition 1
	lhs, op, rhs := binaryExpr.X, binaryExpr.Op, binaryExpr.Y

	// N detpth
	if paramUsages := runSelectorExprTree(ctx, lhs, false); paramUsages != nil {
		for _, usage := range paramUsages {
			usage.guardAt = binaryExpr
		}
		return paramUsages
	}
	if paramUsages := runSelectorExprTree(ctx, rhs, false); paramUsages != nil {
		for _, usage := range paramUsages {
			usage.guardAt = binaryExpr
		}
		return paramUsages
	}

	lchildren := getSelectorExprChildren(lhs)
	if len(lchildren) > 0 {
		lhs = lchildren[len(lchildren)-1].X
	}
	rchildren := getSelectorExprChildren(rhs)
	if len(rchildren) > 0 {
		rhs = rchildren[len(rchildren)-1].X
	}
	lhsV := isTargetedParam(ctx, lhs)
	rhsV := isTargetedParam(ctx, rhs)

	if op == token.EQL || op == token.NEQ {
		// 0 or 1 depth
		if lhsV != nil {
			if rhsIdent := cast2Ident(rhs); rhsIdent != nil && rhsIdent.Name == "nil" {
				paramUsage := NewParamUsage(lhsV, binaryExpr, nil, lhsV.Pos())
				if len(lchildren) > 0 {
					paramUsage.param = ctx.pass.TypesInfo.Uses[lchildren[0].Sel]
				}
				return []*ParamUsage{paramUsage}
			}
		}
		if rhsV != nil {
			if lhsIdent := cast2Ident(lhs); lhsIdent != nil && lhsIdent.Name == "nil" {
				paramUsage := NewParamUsage(rhsV, binaryExpr, nil, rhsV.Pos())
				if len(rchildren) > 0 {
					paramUsage.param = ctx.pass.TypesInfo.Uses[rchildren[0].Sel]
				}
				return []*ParamUsage{paramUsage}
			}
		}
	}

	// Guard Definition 2
	if op == token.EQL || op == token.NEQ || op == token.LEQ || op == token.GEQ || op == token.LSS || op == token.GTR {
		if lenParamUsage := lenCompGuard(ctx, lhs); lenParamUsage != nil {
			if isSliceTyp(lenParamUsage.param) {
				return []*ParamUsage{lenParamUsage}
			}
		}
		if lenParamUsage := lenCompGuard(ctx, rhs); lenParamUsage != nil {
			if isSliceTyp(lenParamUsage.param) {
				return []*ParamUsage{lenParamUsage}
			}
		}
	}
	return nil
}

func runTypSwitchStmt(ctx Context, typSwitchStmt *ast.TypeSwitchStmt) []*ParamUsage {
	// Guard Definition 3
	if assignExpr, ok := typSwitchStmt.Assign.(*ast.ExprStmt); ok {
		if typAssertExpr, ok := assignExpr.X.(*ast.TypeAssertExpr); ok {

			// depth n: e.g., s.member1.member2.(type)
			if paramUsage := runSelectorExprTree(ctx, typAssertExpr.X, false); paramUsage != nil {
				return paramUsage
			}

			// depth 1: e.g., itf.(type)
			if v := isTargetedParam(ctx, typAssertExpr.X); v != nil {
				if _, ok := v.Type().Underlying().(*types.Interface); ok {
					return []*ParamUsage{NewParamUsage(v, typSwitchStmt, nil, v.Pos())}
				}
			}
		}
	}
	return nil
}

func runSelectorExprTree(ctx Context, expr ast.Expr, use bool) []*ParamUsage {
	children := getSelectorExprChildren(expr)
	if len(children) == 0 {
		return nil
	}

	mostParentExpr := children[len(children)-1].X
	lastChildIdent := children[0].Sel

	var usages []*ParamUsage
	if v := isTargetedParam(ctx, mostParentExpr); v != nil {
		mostParentIdent := cast2Ident(mostParentExpr)
		mostParentTyp := unwrapPtrTyp(ctx.pass.TypesInfo.Uses[mostParentIdent].Type())
		for typ, innerChildren := range ctx.typCollection {
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
								usage := ctx.pass.TypesInfo.Uses[children[i].Sel]
								if usage.Type() == typ.Type() {
									if use {
										paramUsage := NewParamUsage(usage, nil, expr, v.Pos())
										paramUsage.context = v
										usages = append(usages, paramUsage)
									} else {
										if lastChildIdent.Name == ident.Name {
											paramUsage := NewParamUsage(usage, expr, nil, v.Pos())
											paramUsage.context = v
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
