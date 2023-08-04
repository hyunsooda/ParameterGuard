package main

import (
	"github.com/hyunsooda/paramguard/checker/passes"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	passes.Init()
	singlechecker.Main(passes.MainAnalyzer)
}
