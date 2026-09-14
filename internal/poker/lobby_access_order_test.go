package poker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// This is a source-order regression guard, not a runtime timeout test.
func TestLobbyCommitsSnapshotBeforeExternalGrants(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "lobby.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var commit, deadline, grant token.Pos
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "lobby" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if receiver.Name == "tx" && sel.Sel.Name == "Commit" {
				commit = call.Pos()
			}
			if receiver.Name == "s" && sel.Sel.Name == "passwordDeadline" {
				deadline = call.Pos()
			}
			if receiver.Name == "s" && sel.Sel.Name == "accessValid" {
				grant = call.Pos()
			}
			return true
		})
	}
	if commit == 0 || deadline == 0 || grant == 0 || commit >= deadline || commit >= grant {
		t.Fatal("external grant/deadline lookup can precede PG snapshot commit")
	}
}

func TestBuyInHasFinalCommitGate(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "funding.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "BuyIn" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			assignment, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assignment.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "beforeCommit" {
					found = true
				}
			}
			return true
		})
	}
	if !found {
		t.Fatal("BuyIn is missing the actor final-commit gate")
	}
}
