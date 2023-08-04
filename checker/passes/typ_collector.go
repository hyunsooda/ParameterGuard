package passes

import (
	"go/ast"
	"reflect"

	"github.com/hyunsooda/paramguard/checker/passtyps"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var TypCollector = &analysis.Analyzer{
	Doc:        "Assistant pass for ParamGuard analyzer",
	Name:       "typcollector",
	Run:        runTypCollector,
	Requires:   []*analysis.Analyzer{inspect.Analyzer},
	ResultType: reflect.TypeOf(new(passtyps.StructTyps)),
}

func runTypCollector(pass *analysis.Pass) (interface{}, error) {
	namedTyps := aggregateNamedTyps(pass)
	return &namedTyps, nil
}

func aggregateNamedTyps(pass *analysis.Pass) passtyps.StructTyps {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	filterNodes := []ast.Node{
		(*ast.GenDecl)(nil),
	}
	typObjs := make(passtyps.StructTyps)
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
