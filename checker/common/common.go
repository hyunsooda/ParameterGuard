package common

import (
	"go/ast"
	"go/types"

	"github.com/hyunsooda/paramguard/checker/passtyps"
)

func UnwrapPtrTyp(typ types.Type) types.Type {
	if t, ok := typ.(*types.Pointer); ok {
		return UnwrapPtrTyp(t.Elem())
	}
	return typ
}

func MapMerge(m1, m2 passtyps.NamedTypes) passtyps.NamedTypes {
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

func IsCollected(collected passtyps.NamedTypes, target types.Object) bool {
	for _, structTyp := range collected {
		for _, obj := range structTyp {
			if obj == target {
				return true
			}
		}
	}
	return false
}

func IsTargetedParam(ctx passtyps.Context, expr ast.Expr) types.Object {
	obj := ctx.Pass.TypesInfo.ObjectOf(Cast2Ident(expr))
	if obj == nil {
		return nil
	}
	if v := findParam(obj.Type(), ctx.Params); v != nil && v.Type().String() != "*testing.T" {
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

func GetSelectorExprChildren(expr ast.Expr) []*ast.SelectorExpr {
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

func IsSliceTyp(paramTyp types.Object) bool {
	_, ok := paramTyp.Type().Underlying().(*types.Slice)
	return ok
}

func Cast2Ident(expr ast.Expr) *ast.Ident {
	switch typ := expr.(type) {
	case *ast.StarExpr:
		return Cast2Ident(typ.X)
	case *ast.CallExpr:
		return Cast2Ident(typ.Fun)
	case *ast.Ident:
		return typ
	default:
		return nil
	}
}
