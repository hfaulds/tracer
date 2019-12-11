package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"io"
	"sort"
	"strings"

	"github.com/hfaulds/tracer/parse/types"
)

type Builder interface {
	WriteTo(io.Writer) (int, error)

	WriteStruct(types.Struct)
	WriteMethod(*types.Struct, types.Method, func(b Builder))
	WriteLine(string, ...interface{})
	Write(string, ...interface{})
}

type builder struct {
	buf       *bytes.Buffer
	importMap map[string]string
}

func NewBuilder(pkg *types.Package) Builder {
	b := builder{
		buf:       &bytes.Buffer{},
		importMap: buildImportMap(pkg),
	}
	b.WriteLine("// Code generated by tracer v0.0.1. DO NOT EDIT.")
	b.WriteLine("package %s", pkg.Name)
	b.writeImports()
	return b
}

func (b builder) WriteTo(w io.Writer) (int, error) {
	formatted, err := format.Source(b.buf.Bytes())
	if err != nil {
		return 0, err
	}
	return w.Write([]byte(formatted))
}

func (b builder) WriteStruct(strct types.Struct) {
	b.WriteLine("\ntype %s struct {", strct.Name)
	for _, attr := range strct.Attrs {
		b.WriteLine("%s %s", attr.Name, b.resolveParam(attr.Type))
	}
	b.WriteLine("}")
}

func (b builder) WriteMethod(strct *types.Struct, method types.Method, callback func(b Builder)) {
	b.Write("\nfunc ")
	if strct != nil {
		b.Write("(t %s) ", strct.Name)
	}
	generateMethodSig(b.buf, "", method.Name, b.resolveParams(method.Params), b.resolveParams(method.Returns))
	b.WriteLine(" {")
	callback(b)
	b.WriteLine("}")
}

func (b builder) WriteLine(str string, a ...interface{}) {
	b.Write(str+"\n", a...)
}

func (b builder) Write(str string, a ...interface{}) {
	fmt.Fprintf(b.buf, str, a...)
}

func (b builder) resolveParams(params []types.Param) []string {
	resolved := make([]string, 0, len(params))
	for _, p := range params {
		resolved = append(resolved, b.resolveParam(p))
	}
	return resolved
}

func (b builder) resolveParam(p types.Param) string {
	switch tp := p.(type) {
	case types.BasicParam:
		return tp.Typ
	case types.NamedParam:
		if tp.Pkg != "" {
			if alias, ok := b.importMap[tp.Pkg]; ok {
				return fmt.Sprintf("%s.%s", alias, tp.Typ)
			} else {
				return tp.Typ
			}
		}
		return tp.Typ
	case types.ArrayParam:
		return fmt.Sprintf("[%d]%s", tp.Length, b.resolveParam(tp.Typ))
	case types.SliceParam:
		return fmt.Sprintf("[]%s", b.resolveParam(tp.Typ))
	case types.PointerParam:
		return fmt.Sprintf("*%s", b.resolveParam(tp.Typ))
	case types.MapParam:
		return fmt.Sprintf("map[%s]%s", b.resolveParam(tp.Key), b.resolveParam(tp.Elem))
	case types.InterfaceParam:
		var buf strings.Builder
		if len(tp.Methods) == 0 {
			fmt.Fprint(&buf, "interface{}")
		} else if len(tp.Methods) == 1 {
			fmt.Fprint(&buf, "interface{ ")
			m := tp.Methods[0]
			params := b.resolveParams(m.Params)
			returns := b.resolveParams(m.Returns)
			generateMethodSig(&buf, "", m.Name, params, returns)
			fmt.Fprint(&buf, " }")
		} else {
			fmt.Fprint(&buf, "interface {")
			for _, m := range tp.Methods {
				fmt.Fprint(&buf, "\n")
				params := b.resolveParams(m.Params)
				returns := b.resolveParams(m.Returns)
				generateMethodSig(&buf, "", m.Name, params, returns)
			}
			fmt.Fprint(&buf, "\n},\n")
		}
		return buf.String()
	default:
		return "<unsupported>"
	}
}

func generateMethodSig(b io.Writer, implementor, methodName string, params, returns []string) {
	if implementor != "" {
		fmt.Fprintf(b, "(t %s) ", implementor)
	}
	fmt.Fprintf(b, "%s(", methodName)
	for i, param := range params {
		fmt.Fprintf(b, "p%d %s", i, param)
		if i < len(params)-1 {
			fmt.Fprint(b, ", ")
		}
	}
	fmt.Fprint(b, ")")
	if len(returns) > 0 {
		fmt.Fprint(b, " ")
	}
	if len(returns) > 1 {
		fmt.Fprint(b, "(")
	}
	for i, r := range returns {
		if i > 0 {
			fmt.Fprint(b, ", ")
		}
		fmt.Fprint(b, r)
	}
	if len(returns) > 1 {
		fmt.Fprint(b, ")")
	}
}

func (b builder) writeImports() {
	var imports []string
	for imp, alias := range b.importMap {
		imports = append(imports, fmt.Sprintf("import %s \"%s\"", alias, imp))
	}
	sort.Strings(imports)
	b.Write(strings.Join(imports, "\n"))
	if len(imports) > 0 {
		b.WriteLine("")
	}
}

func buildImportMap(pkg *types.Package) map[string]string {
	importMap := map[string]string{}
	for _, i := range pkg.Interfaces {
		for _, p := range resolveMethodPackages(i.Methods) {
			if p == pkg.PkgPath {
				continue
			}
			if _, ok := importMap[p]; !ok {
				importMap[p] = fmt.Sprintf("i%d", len(importMap))
			}
		}
	}
	return importMap
}

func resolveMethodPackages(methods []types.Method) []string {
	var pkgs []string
	for _, m := range methods {
		for _, p := range m.Params {
			pkgs = append(pkgs, resolvePackages(p)...)
		}
		for _, p := range m.Returns {
			pkgs = append(pkgs, resolvePackages(p)...)
		}
	}
	return pkgs
}

func resolvePackages(p types.Param) []string {
	switch tp := p.(type) {
	case types.NamedParam:
		if tp.Pkg == "" {
			return []string{}
		}
		return []string{tp.Pkg}
	case types.ArrayParam:
		return resolvePackages(tp.Typ)
	case types.SliceParam:
		return resolvePackages(tp.Typ)
	case types.PointerParam:
		return resolvePackages(tp.Typ)
	case types.MapParam:
		return append(resolvePackages(tp.Key), resolvePackages(tp.Elem)...)
	case types.InterfaceParam:
		return resolveMethodPackages(tp.Methods)
	default:
		return []string{}
	}
}
