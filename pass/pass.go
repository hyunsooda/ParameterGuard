package pass

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

type ParamUsage struct {
	Param      *types.Var
	GuardAt    ast.Node
	UseAt      ast.Node
	DeclaredAt token.Pos
}

var (
	TestOn   = false
	Analyzer = &analysis.Analyzer{
		Doc:      "Perform static analysis on Go source files to identify unsafe practices, such as nil dereferences, using a heuristic-based approach.",
		Name:     "unsafeuse",
		Run:      run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
)

func NewParamUsage(param *types.Var, GuardAt, UseAt ast.Node, DeclaredAt token.Pos) *ParamUsage {
	return &ParamUsage{
		Param:      param,
		GuardAt:    GuardAt,
		UseAt:      UseAt,
		DeclaredAt: DeclaredAt,
	}
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
					params := sig.Params()
					ptrParams := getNilableParams(params)
					if len(ptrParams) > 0 {
						unsanitized := runBlk(pass, fnDecl.Body, ptrParams)
						addReports(pass, fn, unsanitized)
						if TestOn {
							for _, violated := range unsanitized {
								pass.Reportf(violated.DeclaredAt, fmt.Sprintf("Declared '%s'", violated.Param.Name()))
								pass.Reportf(violated.UseAt.Pos(), fmt.Sprintf("Unsafely used '%s'", violated.Param.Name()))
							}
						}
					}
				}
			}
		}
	})
	printReports(pass)
	return nil, nil
}

func getNilableParams(params *types.Tuple) []*types.Var {
	var ptrParams []*types.Var
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		switch p.Type().Underlying().(type) {
		case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Signature:
			ptrParams = append(ptrParams, p)
		}
	}
	return ptrParams
}

func runBlk(pass *analysis.Pass, blkStmt *ast.BlockStmt, params []*types.Var) []*ParamUsage {
	var (
		uses         []*ParamUsage
		guards       []*ParamUsage
		sanitizedUse = make(map[*ParamUsage]bool)
		unsanitized  []*ParamUsage
	)
	if blkStmt != nil {
		ast.Inspect(blkStmt, func(n ast.Node) bool {
			paramUsage := runExpr(pass, n, params)
			if paramUsage != nil {
				if paramUsage.GuardAt != nil {
					guards = append(guards, paramUsage)
				}
				if paramUsage.UseAt != nil {
					uses = append(uses, paramUsage)
				}
			}
			return true
		})
	}

	for _, guard := range guards {
		for _, use := range uses {
			if guard.Param.Id() == use.Param.Id() &&
				guard.GuardAt.Pos() < use.UseAt.Pos() {
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

func runExpr(pass *analysis.Pass, n ast.Node, params []*types.Var) *ParamUsage {
	switch expr := n.(type) {
	case *ast.CallExpr:
		if fnIdent := match(pass, expr.Fun, params); fnIdent != nil {
			return NewParamUsage(fnIdent, nil, expr, fnIdent.Pos())
		}
	case *ast.BinaryExpr:
		return runBinaryExpr(pass, expr, params)

	case *ast.TypeSwitchStmt:
		return runTypSwitchStmt(pass, expr, params)
	case *ast.StarExpr:
		if v := match(pass, expr, params); v != nil {
			return NewParamUsage(v, nil, expr, v.Pos())
		}
	case *ast.SelectorExpr:
		if v := match(pass, expr.X, params); v != nil {
			if v.Type().String() != "*testing.T" {
				return NewParamUsage(v, nil, expr, v.Pos())
			}
		}
	case *ast.SliceExpr:
		if v := match(pass, expr.X, params); v != nil {
			return NewParamUsage(v, nil, expr, v.Pos())
		}
	case *ast.IndexExpr:
		if v := match(pass, expr.X, params); v != nil {
			return NewParamUsage(v, nil, expr, v.Pos())
		}
	}
	return nil
}

func lenCompGuard(pass *analysis.Pass, expr ast.Expr, params []*types.Var) *ParamUsage {
	if callExpr, isCallExpr := expr.(*ast.CallExpr); isCallExpr {
		if fnIdent := cast2Ident(callExpr); fnIdent != nil && fnIdent.Name == "len" {
			if lenParam := match(pass, callExpr.Args[0], params); lenParam != nil {
				return NewParamUsage(lenParam, expr, nil, lenParam.Pos())
			}
		}
	}
	return nil
}

func runBinaryExpr(pass *analysis.Pass, binaryExpr *ast.BinaryExpr, params []*types.Var) *ParamUsage {
	// Guard Definition 1
	lhs, op, rhs := binaryExpr.X, binaryExpr.Op, binaryExpr.Y
	lhsV := match(pass, lhs, params)
	rhsV := match(pass, rhs, params)
	if op == token.EQL || op == token.NEQ {
		if lhsV != nil {
			if rhsIdent := cast2Ident(rhs); rhsIdent != nil && rhsIdent.Name == "nil" {
				return NewParamUsage(lhsV, binaryExpr, nil, lhsV.Pos())
			}
		}
		if rhsV != nil {
			if lhsIdent := cast2Ident(lhs); lhsIdent != nil && lhsIdent.Name == "nil" {
				return NewParamUsage(rhsV, binaryExpr, nil, rhsV.Pos())
			}
		}
	}

	// Guard Definition 2
	if op == token.EQL || op == token.NEQ || op == token.LEQ || op == token.GEQ || op == token.LSS || op == token.GTR {
		if lenParamUsage := lenCompGuard(pass, lhs, params); lenParamUsage != nil {
			if isSliceTyp(lenParamUsage.Param) {
				return lenParamUsage
			}
		}
		if lenParamUsage := lenCompGuard(pass, rhs, params); lenParamUsage != nil {
			if isSliceTyp(lenParamUsage.Param) {
				return lenParamUsage
			}
		}
	}
	return nil
}

func runTypSwitchStmt(pass *analysis.Pass, typSwitchStmt *ast.TypeSwitchStmt, params []*types.Var) *ParamUsage {
	// Guard Definition 3
	if assignExpr, ok := typSwitchStmt.Assign.(*ast.ExprStmt); ok {
		if typAssertExpr, ok := assignExpr.X.(*ast.TypeAssertExpr); ok {
			if v := match(pass, typAssertExpr.X, params); v != nil {
				if _, ok := v.Type().Underlying().(*types.Interface); ok {
					return NewParamUsage(v, typSwitchStmt, nil, v.Pos())
				}
			}
		}
	}
	return nil
}

func isSliceTyp(paramTyp *types.Var) bool {
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

func match(pass *analysis.Pass, expr ast.Expr, params []*types.Var) *types.Var {
	obj := pass.TypesInfo.ObjectOf(cast2Ident(expr))
	for _, param := range params {
		if obj == param {
			return param
		}
	}
	return nil
}
