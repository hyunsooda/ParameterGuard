package pass

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

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
func getAllInnerTyps(pass *analysis.Pass, collected NamedTypes, params []types.Object, namedTyps map[types.Object]*ast.StructType) NamedTypes {
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
