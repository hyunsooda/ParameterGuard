package pass

import (
	"go/ast"
	"go/types"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var ParamCollector = &analysis.Analyzer{
	Doc:        "Assistant pass for ParamGuard analyzer",
	Name:       "paramcollector",
	Run:        runParamCollector,
	Requires:   []*analysis.Analyzer{inspect.Analyzer, TypCollector},
	ResultType: reflect.TypeOf(new(FuncParams)),
}

func runParamCollector(pass *analysis.Pass) (interface{}, error) {
	funcParams := aggregateFuncParams(pass)
	return &funcParams, nil
}

func aggregateFuncParams(pass *analysis.Pass) FuncParams {
	config := parseConfig(pass)
	namedTyps := pass.ResultOf[TypCollector].(*StructTyps)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filterNodes := []ast.Node{
		(*ast.FuncDecl)(nil),
	}
	funcParams := make(FuncParams)
	insp.Preorder(filterNodes, func(n ast.Node) {
		if fnDecl, ok := n.(*ast.FuncDecl); ok {
			if !isInExcludes(pass, fnDecl, config) {
				if fn, ok := pass.TypesInfo.Defs[fnDecl.Name]; ok {
					sig := fn.Type().(*types.Signature)
					interestingParams := getNilableParams(sig.Params())
					typCollection := getAllInnerTyps(pass, nil, interestingParams, *namedTyps)
					funcParams[fn] = ParamWithTypCollection{params: interestingParams, typCollection: typCollection}
				}
			}
		}
	})
	return funcParams
}
