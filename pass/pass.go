package pass

import (
	"flag"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
	"strings"

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

const FLAG_EXCLUDE_TEST = "excludetest"

var (
	TestOn   = false
	Analyzer = &analysis.Analyzer{
		Doc:      "Perform static analysis on Go source files to identify unsafe practices, such as nil dereferences, using a heuristic-based approach.",
		Name:     "unsafeuse",
		Run:      run,
		Requires: []*analysis.Analyzer{inspect.Analyzer},
	}
)

func Init() {
	customFlags := flag.NewFlagSet("unsafeuse-flags", flag.ExitOnError)
	customFlags.Bool(FLAG_EXCLUDE_TEST, false, "Set true to exclude test files. (default=false)")
	Analyzer.Flags = *customFlags
}

func NewParamUsage(param *types.Var, GuardAt, UseAt ast.Node, DeclaredAt token.Pos) *ParamUsage {
	return &ParamUsage{
		Param:      param,
		GuardAt:    GuardAt,
		UseAt:      UseAt,
		DeclaredAt: DeclaredAt,
	}
}

func isExcludeTestFiles(pass *analysis.Pass) bool {
	v := pass.Analyzer.Flags.Lookup(FLAG_EXCLUDE_TEST).Value.String()
	if isOn, err := strconv.ParseBool(v); err == nil {
		return isOn
	} else {
		panic(err)
	}
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filterNodes := []ast.Node{
		(*ast.FuncDecl)(nil),
	}
	insp.Preorder(filterNodes, func(n ast.Node) {
		if fnDecl, ok := n.(*ast.FuncDecl); ok {
			if isExcludeTestFiles(pass) {
				fileName := strings.TrimSuffix(filepath.Base(pass.Fset.File(fnDecl.Pos()).Name()), ".go")
				if len(fileName) > 4 && fileName[len(fileName)-4:] == "test" {
					pass.Reportf(fnDecl.Pos(), "testfile skipped")
					return
				}
			}

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

	// Guard Definition 1
	case *ast.BinaryExpr:
		lhs, op, rhs := expr.X, expr.Op, expr.Y
		lhsV := match(pass, lhs, params)
		rhsV := match(pass, rhs, params)
		if op == token.EQL || op == token.NEQ {
			if lhsV != nil {
				if rhsIdent := cast2Ident(rhs); rhsIdent != nil && rhsIdent.Name == "nil" {
					return NewParamUsage(lhsV, expr, nil, lhsV.Pos())
				}
			}
			if rhsV != nil {
				if lhsIdent := cast2Ident(lhs); lhsIdent != nil && lhsIdent.Name == "nil" {
					return NewParamUsage(rhsV, expr, nil, rhsV.Pos())
				}
			}
		}
	// Guard Definition 2
	case *ast.TypeSwitchStmt:
		if assignExpr, ok := expr.Assign.(*ast.ExprStmt); ok {
			if typAssertExpr, ok := assignExpr.X.(*ast.TypeAssertExpr); ok {
				if v := match(pass, typAssertExpr.X, params); v != nil {
					if _, ok := v.Type().Underlying().(*types.Interface); ok {
						return NewParamUsage(v, expr, nil, v.Pos())
					}
				}
			}
		}

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
		// default:
		// 	fmt.Println(reflect.TypeOf(expr))
	}
	return nil
}

func cast2Ident(expr ast.Expr) *ast.Ident {
	switch typ := expr.(type) {
	// case *ast.StarExpr:
	case *ast.StarExpr:
		return cast2Ident(typ.X)
	case *ast.CallExpr:
		return cast2Ident(typ.Fun)
	case *ast.Ident:
		return typ
	// case *ast.BasicLit:
	// TODO: is it correct?
	default:
		// panic(fmt.Sprintf("not considered :%v", reflect.TypeOf(expr)))
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
