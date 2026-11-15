//go:build ignore

// One-shot renamer helpers for varnamelen cleanup.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == ".git" || base == "scripts" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		return rewriteFile(path)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rewriteFile(path string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	changed := false

	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}
		if renameModelLocals(fn) {
			changed = true
		}
		return false
	})

	// Also walk function literals inside decls
	ast.Inspect(file, func(n ast.Node) bool {
		fl, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		if renameModelLocalsInBody(fl.Body) {
			changed = true
		}
		return true
	})

	if !changed {
		return nil
	}

	var buf strings.Builder
	if err := format.Node(&buf, fset, file); err != nil {
		return fmt.Errorf("%s format: %w", path, err)
	}
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

func renameModelLocals(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}
	return renameModelLocalsInBody(fn.Body)
}

func renameModelLocalsInBody(body *ast.BlockStmt) bool {
	changed := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if stmt.Tok != token.DEFINE {
				return true
			}
			for i, lhs := range stmt.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name != "m" || id.Obj == nil {
					continue
				}
				if i >= len(stmt.Rhs) {
					continue
				}
				if looksLikeModelValue(stmt.Rhs[i]) {
					obj := id.Obj
					id.Name = "model"
					renameIdentUses(body, obj, "model")
					changed = true
				}
			}
		case *ast.DeclStmt:
			gen, ok := stmt.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				return true
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if !isModelType(vs.Type) && !(len(vs.Values) > 0 && looksLikeModelValue(vs.Values[0])) {
					continue
				}
				for _, name := range vs.Names {
					if name.Name != "m" || name.Obj == nil {
						continue
					}
					obj := name.Obj
					name.Name = "model"
					renameIdentUses(body, obj, "model")
					changed = true
				}
			}
		}
		return true
	})
	return changed
}

func looksLikeModelValue(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return isModelType(e.Type)
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "New" {
			return true
		}
	case *ast.Ident:
		// m := base where base is Model-typed — treat short assign from ident as candidate
		// only when name suggests model copy in tests (base).
		return e.Name == "base"
	case *ast.TypeAssertExpr:
		return isModelType(e.Type)
	}
	return false
}

func renameIdentUses(root ast.Node, obj *ast.Object, newName string) {
	if obj == nil {
		return
	}
	ast.Inspect(root, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if id.Obj == obj {
			id.Name = newName
		}
		return true
	})
}

func isModelType(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "Model"
	case *ast.StarExpr:
		id, ok := t.X.(*ast.Ident)
		return ok && id.Name == "Model"
	}
	return false
}
