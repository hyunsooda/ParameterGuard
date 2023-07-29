### How to run

`./unsafeuse ./...`

### Flag
`--config=<configuration file path>` Set the configuration file path (default=none)

- Configuration file format
```yaml
files: ["*_test.go", "safe.go"] # Checker ignores all the test files and `safe.go`
pkgs: ["mypackage1", "mypackage2"] # Checker ignores entire functions under the `mypackage1` and `mypackage2`
funcs:
  [
    {
      pkg: "mypackage3",
      funcs: ["func1", "func2"] # Checker ignores `func1` and `func2` under the package `mypackage3`
    },
    {
      pkg: "mypackage4",
      funcs: ["func1", "func2"]
    }
  ]
log: false
```

### Interesting types
interface, map, pointer, slice, struct, function pointer
