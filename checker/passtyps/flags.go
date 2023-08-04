package passtyps

import (
	"go/ast"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Files []string
	Pkgs  []string
	Funcs []struct {
		Pkg   string
		Funcs []string
	}
	Log       bool
	CallGraph bool
	Maxpath   int
}

const (
	FLAG_CONFIG_FILE_PATH = "config"
)

var Testing Test

func ParseConfig(pass *analysis.Pass) *Config {
	if Testing.On {
		return nil
	}
	filePath := pass.Analyzer.Flags.Lookup(FLAG_CONFIG_FILE_PATH).Value.String()
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalln(err)
	}
	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		log.Fatalf("Error unmarshaling YAML: %v", err)
	}
	return &config
}

func IsInExcludes(pass *analysis.Pass, fnDecl *ast.FuncDecl, config *Config) bool {
	if Testing.On {
		return false
	}
	// 1. Exclude files
	curFileName := pass.Fset.File(fnDecl.Pos()).Name()
	for _, file := range config.Files {
		trimmed := filepath.Base(curFileName)
		if strCmp(file, curFileName) || strCmp(file, trimmed) {
			skipLog(pass, config, curFileName+" file skipped")
			return true
		}
	}

	// 2. Exclude packages
	curPkg := pass.Pkg.Name()
	for _, pkg := range config.Pkgs {
		if strCmp(pkg, curPkg) {
			skipLog(pass, config, curPkg+" package skipped")
			return true
		}
	}

	// 3. Exclude specific functions
	curFunc := fnDecl.Name.Name
	for _, funcs := range config.Funcs {
		if strCmp(funcs.Pkg, curPkg) {
			for _, funcName := range funcs.Funcs {
				if curFunc == funcName {
					skipLog(pass, config, filepath.Join(curPkg, curFunc)+" function skipped")
					return true
				}
			}
		}
	}
	return false
}

func skipLog(pass *analysis.Pass, config *Config, logStr string) {
	if config.Log {
		pass.Reportf(0, logStr)
	}
}

func isWildcardPattrn(str string) bool {
	return strings.Contains(str, "*")
}

func getWildcardPattern(str string) string {
	return "." + str + "$"
}

func wildcardPattern(patternStr, targetStr string) bool {
	pattern := getWildcardPattern(patternStr)

	regExp, err := regexp.Compile(pattern)
	if err != nil {
		log.Fatalf("Error compiling regular expression: %v", err)
	}

	if regExp.MatchString(targetStr) {
		return true
	}
	return false
}

func strCmp(listedInConfig, curContext string) bool {
	if isWildcardPattrn(listedInConfig) {
		return wildcardPattern(listedInConfig, curContext)
	} else {
		return listedInConfig == curContext
	}
}
