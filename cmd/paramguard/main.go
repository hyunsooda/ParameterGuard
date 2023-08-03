package main

import (
	"github.com/hyunsooda/paramguard/pass"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	pass.Init()
	singlechecker.Main(pass.MainAnalyzer)
}
