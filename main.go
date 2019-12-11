package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/hfaulds/tracer/gen"
	"github.com/hfaulds/tracer/gen/constructor"
	"github.com/hfaulds/tracer/gen/timing"
	"github.com/hfaulds/tracer/gen/tracing"
	"github.com/hfaulds/tracer/parse"
	"github.com/hfaulds/tracer/parse/types"
)

//go:generate code-gen ./ -interface=Client -struct=client
//go:generate code-gen ./ -interface=Client -tracing=pkg
//go:generate code-gen ./ -interface=Client -struct=client -tracing=pkg
//go:generate code-gen ./ -interface=Client -struct=client -tracing=pkg -o client_gen.go
//go:generate code-gen ./ -interface=Client -struct=client -timing

type flags struct {
	interfaceName string
	structName    string
	tracingPkg    string
	timingAttr    string
	output        string
}

func main() {
	f := new(flags)
	flag.StringVar(&f.interfaceName, "interface", "", "Interface to generate wrappers for")
	flag.StringVar(&f.structName, "struct", "", "Toggles constructor generation and the struct to return. When used in combination with other flags it will construct the generated wrappers.")
	flag.StringVar(&f.tracingPkg, "tracing", "", "Toggles tracing wrapper generation")
	flag.StringVar(&f.timingAttr, "timing", "", "Toggles timing wrapper generation")

	flag.StringVar(&f.output, "o", "", "Output file; defaults to stdout.")

	flag.Parse()
	if flag.NArg() < 1 {
		usage()
		log.Fatalf("Expected at least one arguments, received %d", flag.NArg())
	}
	if len(f.interfaceName) < 1 {
		log.Fatal("required flag -interface missing")
	}
	if len(f.structName) < 1 {
		log.Fatal("required flag -struct missing")
	}

	pkg, err := parse.ParseDir(flag.Arg(0))
	if err != nil {
		log.Fatalf("Failed to parse %s", err)
	}

	importMap := gen.BuildImportMap(pkg)

	var b gen.Buffer
	fmt.Fprint(&b, "// Code generated by tracer v0.0.1. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n\n", pkg.Name)
	gen.GenerateImports(&b, importMap)

	iface, ok := findInterface(pkg, f.interfaceName)
	if !ok {
		log.Fatalf("Could not find interface: %s", f.interfaceName)
	}
	strct, ok := findStruct(pkg, f.structName)
	if !ok {
		log.Fatalf("Could not find struct: %s", f.structName)
	}

	var wrappers []string
	if len(f.tracingPkg) > 0 {
		if tracing.ShouldSkipInterface(iface) {
			log.Fatal("Could not find any methods taking context")
		}
		tracingWrapper := tracing.Gen(&b, iface, importMap, f.tracingPkg)
		wrappers = append(wrappers, tracingWrapper)
	}
	if len(f.timingAttr) > 0 {
		if !timing.StructHasTimingAttr(strct, f.timingAttr) {
			log.Fatal("Struct does not have specific timing attribute")
		}
		timingWrapper := timing.Gen(&b, iface, importMap, f.timingAttr)
		wrappers = append(wrappers, timingWrapper)
	}
	constructor.Gen(&b, importMap, iface, strct, wrappers)

	dst := os.Stdout
	if len(f.output) > 0 {
		if err := os.MkdirAll(filepath.Dir(f.output), os.ModePerm); err != nil {
			log.Fatalf("Unable to create directory: %v", err)
		}
		f, err := os.Create(f.output)
		if err != nil {
			log.Fatalf("Failed opening destination file: %v", err)
		}
		defer f.Close()
		dst = f
	}

	if _, err := b.WriteTo(dst); err != nil {
		log.Fatalf("Failed writing to destination: %v", err)
	}
}

func usage() {
	io.WriteString(os.Stderr, usageText)
	flag.PrintDefaults()
}

const usageText = `
grep [-trace=Interface] [-o=dest.go] [file]
`

func findInterface(pkg *types.Package, name string) (types.Interface, bool) {
	for _, iface := range pkg.Interfaces {
		if iface.Name == name {
			return iface, true
		}
	}
	return types.Interface{}, false
}

func findStruct(pkg *types.Package, name string) (types.Struct, bool) {
	for _, strct := range pkg.Structs {
		if strct.Name == name {
			return strct, true
		}
	}
	return types.Struct{}, false
}
