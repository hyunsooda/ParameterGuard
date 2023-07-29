package pass

import (
	"go/ast"
	"go/token"
	"go/types"
	"sync"

	"golang.org/x/tools/go/analysis"
)

type ParamUsage struct {
	param      types.Object
	context    types.Object
	guardAt    ast.Node
	useAt      ast.Node
	declaredAt token.Pos
}

type NamedTypes map[types.Type]map[*ast.Ident]types.Object

type Context struct {
	pass          *analysis.Pass
	params        []types.Object
	typCollection NamedTypes
}

type Test struct {
	on           bool
	lock         *sync.Mutex
	reportedMsgs map[string]bool
}

func NewContext(pass *analysis.Pass, params []types.Object, typCollection NamedTypes) Context {
	return Context{
		pass:          pass,
		params:        params,
		typCollection: typCollection,
	}
}

func NewParamUsage(param types.Object, guardAt, useAt ast.Node, declaredAt token.Pos) *ParamUsage {
	return &ParamUsage{
		param:      param,
		guardAt:    guardAt,
		useAt:      useAt,
		declaredAt: declaredAt,
	}
}

func InitTest() {
	Testing.on = true
	Testing.lock = &sync.Mutex{}
	Testing.reportedMsgs = make(map[string]bool)
}
