package protocol

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRemotePackets_NoOpMethodsOnlyForZeroFieldStructs(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	target := filepath.Join(filepath.Dir(thisFile), "remote_packets.go")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, target, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", target, err)
	}

	fieldCount := map[string]int{}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			name := ts.Name.Name
			if !strings.HasPrefix(name, "Remote_") {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			n := 0
			if st.Fields != nil {
				for _, fld := range st.Fields.List {
					if len(fld.Names) == 0 {
						n++
					} else {
						n += len(fld.Names)
					}
				}
			}
			fieldCount[name] = n
		}
	}

	var noOps int
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || fd.Name == nil {
			continue
		}
		method := fd.Name.Name
		if method != "Read" && method != "Write" {
			continue
		}

		if !isNoOpReturnNil(fd) {
			continue
		}

		recvType := recvName(fd)
		if !strings.HasPrefix(recvType, "Remote_") {
			continue
		}

		noOps++
		if fieldCount[recvType] != 0 {
			t.Fatalf("%s.%s is no-op but struct has %d fields", recvType, method, fieldCount[recvType])
		}
	}

	if noOps == 0 {
		t.Fatalf("expected at least one no-op remote packet method")
	}
}

func recvName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	t := fd.Recv.List[0].Type
	switch tt := t.(type) {
	case *ast.Ident:
		return tt.Name
	case *ast.StarExpr:
		if id, ok := tt.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func isNoOpReturnNil(fd *ast.FuncDecl) bool {
	if fd.Body == nil || len(fd.Body.List) != 1 {
		return false
	}
	ret, ok := fd.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	id, ok := ret.Results[0].(*ast.Ident)
	return ok && id.Name == "nil"
}
