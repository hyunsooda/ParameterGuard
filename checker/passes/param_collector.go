package passes

import (
	"go/ast"
	"go/types"
	"reflect"

	"github.com/hyunsooda/paramguard/checker/common"
	"github.com/hyunsooda/paramguard/checker/passtyps"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var ParamCollector = &analysis.Analyzer{
	Doc:        "Assistant pass for ParamGuard analyzer",
	Name:       "paramcollector",
	Run:        runParamCollector,
	Requires:   []*analysis.Analyzer{inspect.Analyzer, TypCollector},
	ResultType: reflect.TypeOf(new(passtyps.FuncParams)),
}

func runParamCollector(pass *analysis.Pass) (interface{}, error) {
	funcParams := aggregateFuncParams(pass)
	return &funcParams, nil
}

func aggregateFuncParams(pass *analysis.Pass) passtyps.FuncParams {
	config := passtyps.ParseConfig(pass)
	namedTyps := pass.ResultOf[TypCollector].(*passtyps.StructTyps)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filterNodes := []ast.Node{
		(*ast.FuncDecl)(nil),
	}
	funcParams := make(passtyps.FuncParams)
	insp.Preorder(filterNodes, func(n ast.Node) {
		if fnDecl, ok := n.(*ast.FuncDecl); ok {
			if !passtyps.IsInExcludes(pass, fnDecl, config) {
				if fn, ok := pass.TypesInfo.Defs[fnDecl.Name]; ok {
					sig := fn.Type().(*types.Signature)
					interestingParams := getNilableParams(sig.Params())
					typCollection := getAllInnerTyps(pass, nil, interestingParams, *namedTyps)
					funcParams[fn] = passtyps.NewParamWithTypCollection(interestingParams, typCollection)
				}
			}
		}
	})
	return funcParams
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

/// getAllInnerTyps recursively collect struct inner types
func getAllInnerTyps(pass *analysis.Pass, collected passtyps.NamedTypes, params []types.Object, namedTyps map[types.Object]*ast.StructType) passtyps.NamedTypes {
	m := make(passtyps.NamedTypes)

	for _, param := range params {
		for declTyp, structTyp := range namedTyps {
			objTyp := declTyp.Type()
			_, ok := objTyp.(*types.Named)
			if !ok { // Not a struct type
				continue
			}
			if common.UnwrapPtrTyp(param.Type()) != objTyp {
				continue
			}
			m[objTyp] = make(map[*ast.Ident]types.Object)
			for _, targetField := range structTyp.Fields.List {
				for _, targetFieldNm := range targetField.Names {
					fieldObj := pass.TypesInfo.Defs[targetFieldNm]
					m[objTyp][targetFieldNm] = fieldObj

					// self-reference structure create infinite loop
					if common.IsCollected(collected, fieldObj) {
						continue
					}

					for _, innerTyps := range getAllInnerTyps(pass, common.MapMerge(m, collected), []types.Object{fieldObj}, namedTyps) {
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
