package pass

import (
	"go/ast"
	"reflect"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var TypCollector = &analysis.Analyzer{
	Doc:        "ss",
	Name:       "aaaa",
	Run:        runTypCollector,
	Requires:   []*analysis.Analyzer{inspect.Analyzer},
	ResultType: reflect.TypeOf(new(StructTyps)),
}

func runTypCollector(pass *analysis.Pass) (interface{}, error) {
	namedTyps := aggregateNamedTyps(pass)
	return &namedTyps, nil
}

func aggregateNamedTyps(pass *analysis.Pass) StructTyps {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filterNodes := []ast.Node{
		(*ast.GenDecl)(nil),
	}
	typObjs := make(StructTyps)
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
