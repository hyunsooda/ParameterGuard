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

	Analyzer = &analysis.Analyzer{
		Doc:      "Perform static analysis on Go source files to identify unsafe practices, such as nil dereferences, using a heuristic-based approach.",
		Name:     "unsafeuse",
		Run:      run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
)

func aggregateNamedTyps(pass *analysis.Pass) map[types.Object]*ast.StructType {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filterNodes := []ast.Node{
		(*ast.GenDecl)(nil),
	}
	typObjs := make(map[types.Object]*ast.StructType)
	insp.Preorder(filterNodes, func(n ast.Node) {
		if genDecl, ok := n.(*ast.GenDecl); ok {
			for _, spec := range genDecl.Specs {
				if typSpec, ok := spec.(*ast.TypeSpec); ok {
					if structTyp, ok := typSpec.Type.(*ast.StructType); ok {
						obj := pass.TypesInfo.Defs[typSpec.Name]
						typObjs[obj] = structTyp
					}
				}
			}
		}
	})
	return typObjs
}

func getStructTyp(typObjs map[types.Object]*ast.StructType, targetTyp types.Type) *ast.StructType {
	for typ, structTyp := range typObjs {
		if unwrapPtrTyp(typ.Type()) == unwrapPtrTyp(targetTyp) {
			return structTyp
		}
	}
	return nil
}

func unwrapPtrTyp(typ types.Type) types.Type {
	if t, ok := typ.(*types.Pointer); ok {
		return unwrapPtrTyp(t.Elem())
	}
	return typ
}

func mapMerge(m1, m2 NamedTypes) NamedTypes {
	m := make(map[types.Type]map[*ast.Ident]types.Object)
	for declTyp, structTyp := range m1 {
		m[declTyp] = make(map[*ast.Ident]types.Object)
		for ident, obj := range structTyp {
			m[declTyp][ident] = obj
		}
	}
	for declTyp, structTyp := range m2 {
		m[declTyp] = make(map[*ast.Ident]types.Object)
		for ident, obj := range structTyp {
			m[declTyp][ident] = obj
		}
	}
	return m
}

func isCollected(collected NamedTypes, target types.Object) bool {
	for _, structTyp := range collected {
		for _, obj := range structTyp {
			if obj == target {
				return true
			}
		}
	}
	return false
}

/// getAllInnerTyps recursively collect struct inner types
func getAllInnerTyps(pass *analysis.Pass, collected NamedTypes, params []types.Object, namedTyps map[types.Object]*ast.StructType) map[types.Type]map[*ast.Ident]types.Object {
	m := make(NamedTypes)

	for _, param := range params {
		for declTyp, structTyp := range namedTyps {
			objTyp := declTyp.Type()
			_, ok := objTyp.(*types.Named)
			if !ok { // Not a struct type
				continue
			}
			if unwrapPtrTyp(param.Type()) != objTyp {
				continue
			}
			m[objTyp] = make(map[*ast.Ident]types.Object)
			for _, targetField := range structTyp.Fields.List {
				for _, targetFieldNm := range targetField.Names {
					fieldObj := pass.TypesInfo.Defs[targetFieldNm]
					m[objTyp][targetFieldNm] = fieldObj

					// self-reference structure create infinite loop
					if isCollected(collected, fieldObj) {
						continue
					}

					for _, innerTyps := range getAllInnerTyps(pass, mapMerge(m, collected), []types.Object{fieldObj}, namedTyps) {
						for innerNm, innerT := range innerTyps {
							m[objTyp][innerNm] = innerT
						}
					}
				}
			}
		}
	}
	return m
}

func run(pass *analysis.Pass) (interface{}, error) {
	config := parseConfig(pass)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filterNodes := []ast.Node{
		(*ast.FuncDecl)(nil),
	}
	insp.Preorder(filterNodes, func(n ast.Node) {
		if fnDecl, ok := n.(*ast.FuncDecl); ok {
			if !isInExcludes(pass, fnDecl, config) {
				if fn, ok := pass.TypesInfo.Defs[fnDecl.Name]; ok {
					sig := fn.Type().(*types.Signature)
					interestingParams := getNilableParams(sig.Params())
					namedTyps := aggregateNamedTyps(pass)
					typCollection := getAllInnerTyps(pass, nil, interestingParams, namedTyps)

					if len(interestingParams) > 0 {
						ctx := NewContext(pass, interestingParams, typCollection)
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

func getNilableParams(params *types.Tuple) []types.Object {
	var ptrParams []types.Object
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		switch p.Type().Underlying().(type) {
		case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Signature, *types.Struct:
			ptrParams = append(ptrParams, p)
		}
	}
	return ptrParams
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

func getSelectorExprChildren(expr ast.Expr) []*ast.SelectorExpr {
	e, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	children := []*ast.SelectorExpr{e}
	for {
		if child, ok := e.X.(*ast.SelectorExpr); ok {
			e = child
			children = append(children, child)
		} else {
			break
		}
	}
	return children
}

func isSliceTyp(paramTyp types.Object) bool {
	_, isSliceTyp := paramTyp.Type().Underlying().(*types.Slice)
	return isSliceTyp
}

func cast2Ident(expr ast.Expr) *ast.Ident {
	switch typ := expr.(type) {
	case *ast.StarExpr:
		return cast2Ident(typ.X)
	case *ast.CallExpr:
		return cast2Ident(typ.Fun)
	case *ast.Ident:
		return typ
	default:
		return nil
	}
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

func isTargetedParam(ctx Context, expr ast.Expr) types.Object {
	obj := ctx.pass.TypesInfo.ObjectOf(cast2Ident(expr))
	if obj == nil {
		return nil
	}
	if v := findParam(obj.Type(), ctx.params); v != nil && v.Type().String() != "*testing.T" {
		return v
	}
	return nil
}

func findParam(objTyp types.Type, params []types.Object) types.Object {
	for _, param := range params {
		if objTyp == param.Type() {
			return param
		}
	}
	return nil
}
