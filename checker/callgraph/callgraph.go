package callgraph

import (
	"log"
	"strings"
	"sync"

	"github.com/hyunsooda/paramguard/checker/passtyps"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/callgraph/vta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type FromTo struct {
	from string
	to   string
}

const DefaultNPaths = 30

var (
	CGP  passtyps.CallGraph
	once = new(sync.Once)
)

func getProjPkg(pkg *ssa.Package) string {
	proj := strings.Split(pkg.String(), "/")
	return strings.Join(proj[:3], "/")
}

func InitCallGraph(p *analysis.Pass) {
	if passtyps.Testing.On {
		return
	}
	config := passtyps.ParseConfig(p)
	if config.CallGraph {
		nPaths := config.Maxpath
		if nPaths == 0 {
			nPaths = DefaultNPaths
		}
		once.Do(func() {
			CGP = genCallGraph(nPaths)
		})
	}
}

/// genCallGraph returns callpaths for all of the callees.
/// Return format is (callee, [caller of callee (P), caller of P (PP), caller of PP, ...])
/// For N of different callers, they are represented with the array
func genCallGraph(max int) passtyps.CallGraph {
	cfg := &packages.Config{
		Mode:  packages.LoadAllSyntax,
		Tests: false,
		Dir:   "",
	}
	initial, err := packages.Load(cfg, "./...")
	if err != nil {
		log.Fatalln(err)
	}
	if packages.PrintErrors(initial) > 0 {
		log.Fatalln("packages contain errors")
	}
	// Create and build SSA-form program representation
	mode := ssa.InstantiateGenerics // instantiate generics by default for soundness
	prog, pkgs := ssautil.AllPackages(initial, mode)
	prog.Build()

	proj := getProjPkg(pkgs[0])
	cg := vta.CallGraph(ssautil.AllFunctions(prog), cha.CallGraph(prog))

	callPaths := make(passtyps.CallGraph)
	callgraph.GraphVisitEdges(cg, func(e *callgraph.Edge) error {
		callerFunc := e.Caller.Func
		callee, calleeFunc := e.Callee, e.Callee.Func
		if callerFunc.Pkg != nil &&
			calleeFunc.Pkg != nil &&
			strings.Contains(callerFunc.Pkg.String(), proj) &&
			strings.Contains(calleeFunc.Pkg.String(), proj) {
			var visited []string
			visitIn(callee, &visited, max-1)
			callPaths[calleeFunc.String()] = visited
		}
		return nil
	})
	return callPaths
}

func visitIn(n *callgraph.Node, visited *[]string, max int) {
	if len(*visited) > max {
		return
	}
	vs := make([]string, len(n.In))
	for idx, in := range n.In {
		vs[idx] = in.Caller.Func.String()
	}
	switch len(n.In) {
	case 0:
		break
	case 1:
		*visited = append(*visited, vs[0])
	default:
		*visited = append(*visited, "["+strings.Join(vs, ", ")+"]")
	}
	for _, in := range n.In {
		visitIn(in.Caller, visited, max)
	}
}
