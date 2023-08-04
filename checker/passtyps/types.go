package passtyps

import (
	"go/ast"
	"go/token"
	"go/types"
	"sync"

	"golang.org/x/tools/go/analysis"
)

type ParamUsage struct {
	Fn         *types.Func
	Param      types.Object
	Context    types.Object
	GuardAt    ast.Node
	UseAt      ast.Node
	DeclaredAt token.Pos
}

type CallGraph = map[string][]string

type (
	NamedTypes map[types.Type]map[*ast.Ident]types.Object
	FuncParams = map[types.Object]ParamWithTypCollection
	StructTyps = map[types.Object]*ast.StructType
)

type ParamWithTypCollection struct {
	Params        []types.Object
	TypCollection NamedTypes
}

type Context struct {
	Pass          *analysis.Pass
	Params        []types.Object
	TypCollection NamedTypes
}

type Test struct {
	On           bool
	lock         *sync.Mutex
	ReportedMsgs map[string]bool
}

func (t Test) Lock() {
	t.lock.Lock()
}

func (t Test) Unlock() {
	t.lock.Unlock()
}

func NewContext(pass *analysis.Pass, params []types.Object, typCollection NamedTypes) Context {
	return Context{
		Pass:          pass,
		Params:        params,
		TypCollection: typCollection,
	}
}

func NewParamUsage(param types.Object, guardAt, useAt ast.Node, declaredAt token.Pos) *ParamUsage {
	return &ParamUsage{
		Param:      param,
		GuardAt:    guardAt,
		UseAt:      useAt,
		DeclaredAt: declaredAt,
	}
}

func NewParamWithTypCollection(params []types.Object, typCollection NamedTypes) ParamWithTypCollection {
	return ParamWithTypCollection{
		Params:        params,
		TypCollection: typCollection,
	}
}

func InitTest() {
	Testing.On = true
	Testing.lock = &sync.Mutex{}
	Testing.ReportedMsgs = make(map[string]bool)
}
