package testsupport

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

var architectureAllowedInternalRoots = map[string]struct{}{
	"app":            {},
	"platform":       {},
	"identity":       {},
	"authorization":  {},
	"navigation":     {},
	"apimetadata":    {},
	"organization":   {},
	"dictionary":     {},
	"files":          {},
	"audit":          {},
	"routecatalog":   {},
	"apidoc":         {},
	"uploadsecurity": {},
}

var ginRouteRegistrationMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "PATCH": {}, "DELETE": {},
	"HEAD": {}, "OPTIONS": {}, "Any": {}, "Match": {}, "Handle": {},
	"Static": {}, "StaticFile": {}, "StaticFS": {},
}

var architectureCoreForbiddenImports = []string{
	"github.com/gin-gonic/gin",
	"gorm.io/gorm",
	"github.com/redis/go-redis",
	"github.com/minio/minio-go",
	"/adapter",
	"/adapters",
	"/global",
}

func TestArchitectureInternalModuleInventorySkeleton(t *testing.T) {
	root := architectureRepositoryRoot(t)
	internalRoot := filepath.Join(root, "internal")
	entries, err := os.ReadDir(internalRoot)
	if os.IsNotExist(err) {
		t.Log("internal module tree is not created yet; inventory gate is armed")
		return
	}
	if err != nil {
		t.Fatalf("read internal module root: %v", err)
	}

	var unexpected []string
	for _, entry := range entries {
		if !entry.IsDir() {
			unexpected = append(unexpected, entry.Name())
			continue
		}
		if _, ok := architectureAllowedInternalRoots[entry.Name()]; !ok {
			unexpected = append(unexpected, entry.Name())
		}
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		t.Fatalf("unexpected internal module roots: %v", unexpected)
	}
}

func TestArchitectureAppIsOnlyGinRouteRegistrationPointSkeleton(t *testing.T) {
	root := architectureRepositoryRoot(t)
	internalRoot := filepath.Join(root, "internal")
	_, err := os.Stat(internalRoot)
	if os.IsNotExist(err) {
		t.Log("internal module tree is not created yet; Gin registration gate is armed")
		return
	}
	if err != nil {
		t.Fatalf("stat internal module root: %v", err)
	}

	for _, path := range architectureProductionGoFiles(t, internalRoot) {
		if filepath.Base(filepath.Dir(path)) == "app" || filepath.Dir(path) == internalRoot {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, isRouteRegistration := ginRouteRegistrationMethods[selector.Sel.Name]; isRouteRegistration {
				t.Errorf("%s calls Gin route registration method %s outside internal/app", path, selector.Sel.Name)
			}
			return true
		})
	}
}

func TestArchitectureCoreLayersAvoidFrameworkAndAdapterImportsSkeleton(t *testing.T) {
	root := architectureRepositoryRoot(t)
	internalRoot := filepath.Join(root, "internal")
	_, err := os.Stat(internalRoot)
	if os.IsNotExist(err) {
		t.Log("internal module tree is not created yet; core dependency gate is armed")
		return
	}
	if err != nil {
		t.Fatalf("stat internal module root: %v", err)
	}

	for _, path := range architectureProductionGoFiles(t, internalRoot) {
		slashPath := filepath.ToSlash(path)
		if !strings.Contains(slashPath, "/domain/") && !strings.Contains(slashPath, "/application/") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, importSpec := range file.Imports {
			importPath := strings.Trim(importSpec.Path.Value, "\"")
			for _, forbidden := range architectureCoreForbiddenImports {
				if strings.Contains(importPath, forbidden) {
					t.Errorf("%s imports forbidden core dependency %q", path, importPath)
				}
			}
		}
	}
}

func TestArchitectureProductionDoesNotUseLegacyGlobal(t *testing.T) {
	root := architectureRepositoryRoot(t)
	internalRoot := filepath.Join(root, "internal")
	if _, err := os.Stat(filepath.Join(internalRoot, "app", "legacyglobal")); err == nil {
		t.Fatal("internal/app/legacyglobal must be deleted after the module cutover")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat legacyglobal Adapter: %v", err)
	}
	for _, path := range architectureProductionGoFiles(t, internalRoot) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, importSpec := range file.Imports {
			importPath := strings.Trim(importSpec.Path.Value, "\"")
			if importPath == "admin/global" || importPath == "admin/internal/app/legacyglobal" {
				t.Errorf("%s imports retired global dependency %q", path, importPath)
			}
		}
	}
}

func TestArchitectureModulesDoNotImportForeignAdapters(t *testing.T) {
	root := architectureRepositoryRoot(t)
	internalRoot := filepath.Join(root, "internal")
	for _, path := range architectureProductionGoFiles(t, internalRoot) {
		relativePath, err := filepath.Rel(internalRoot, path)
		if err != nil {
			t.Fatalf("resolve relative path for %s: %v", path, err)
		}
		module := strings.Split(filepath.ToSlash(relativePath), "/")[0]
		if module == "app" {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports in %s: %v", path, err)
		}
		for _, importSpec := range file.Imports {
			importPath := strings.Trim(importSpec.Path.Value, "\"")
			if importPath == "admin/internal/app" || strings.HasPrefix(importPath, "admin/internal/app/") {
				t.Errorf("%s imports App composition root", path)
			}
			const prefix = "admin/internal/"
			if !strings.HasPrefix(importPath, prefix) {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(importPath, prefix), "/")
			if len(parts) > 2 && parts[0] != module && parts[1] == "adapters" {
				t.Errorf("%s imports foreign Adapter %q", path, importPath)
			}
		}
	}
}

func TestArchitectureAppOwnsCompositionMigrationAndSeed(t *testing.T) {
	root := architectureRepositoryRoot(t)
	buildCalls, ginConstructors, migrations, foundationCalls, finalizeCalls := 0, 0, 0, 0, 0
	for _, path := range architectureProductionGoFiles(t, root) {
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("resolve relative path for %s: %v", path, err)
		}
		relativePath = filepath.ToSlash(relativePath)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			owner, _ := selector.X.(*ast.Ident)
			if relativePath == "main.go" && owner != nil && owner.Name == "app" && selector.Sel.Name == "BuildFromPath" {
				buildCalls++
			}
			if owner != nil && owner.Name == "gin" && selector.Sel.Name == "New" {
				ginConstructors++
				if relativePath != "internal/app/app.go" {
					t.Errorf("%s constructs a Gin engine outside App", relativePath)
				}
			}
			if selector.Sel.Name == "AutoMigrate" {
				migrations++
				if relativePath != "internal/app/migrate.go" {
					t.Errorf("%s invokes AutoMigrate outside App migration orchestration", relativePath)
				}
			}
			if owner != nil && owner.Name == "seeddata" && (selector.Sel.Name == "Foundation" || selector.Sel.Name == "Finalize") {
				if selector.Sel.Name == "Foundation" {
					foundationCalls++
				} else {
					finalizeCalls++
				}
				if relativePath != "internal/app/seed.go" {
					t.Errorf("%s invokes Seed phase outside App", relativePath)
				}
			}
			return true
		})
	}
	if buildCalls != 1 || ginConstructors != 1 || migrations != 1 || foundationCalls != 1 || finalizeCalls != 1 {
		t.Fatalf("entrypoint counts BuildFromPath/Gin/AutoMigrate/Foundation/Finalize = %d/%d/%d/%d/%d, want 1/1/1/1/1", buildCalls, ginConstructors, migrations, foundationCalls, finalizeCalls)
	}
}

func TestArchitectureSeedOrchestrationDoesNotWriteOwnedModels(t *testing.T) {
	root := architectureRepositoryRoot(t)
	seedRoot := filepath.Join(root, "internal", "app", "seeddata")
	entries, err := os.ReadDir(seedRoot)
	if os.IsNotExist(err) {
		t.Fatal("App Seed orchestration package is required")
	}
	if err != nil {
		t.Fatalf("read App Seed orchestration package: %v", err)
	}
	writeMethods := map[string]struct{}{"Create": {}, "Updates": {}, "Delete": {}, "Save": {}, "Model": {}, "First": {}, "Where": {}, "Pluck": {}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(seedRoot, entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			owner, ok := selector.X.(*ast.Ident)
			if ok {
				if _, writes := writeMethods[selector.Sel.Name]; writes && owner.Name == "tx" {
					t.Errorf("%s writes owned persistence model through tx.%s; delegate to module Seed capability", path, selector.Sel.Name)
				}
			}
			return true
		})
	}
}

func TestArchitectureProductionDoesNotDiscoverGinRoutes(t *testing.T) {
	root := architectureRepositoryRoot(t)
	for _, path := range architectureProductionGoFiles(t, root) {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Routes" {
				return true
			}
			owner, _ := selector.X.(*ast.Ident)
			if owner != nil && (owner.Name == "engine" || owner.Name == "router" || owner.Name == "ginEngine" || owner.Name == "ginRouter") {
				t.Errorf("%s discovers business routes from Gin Engine; use Route Catalog Snapshot", path)
			}
			return true
		})
	}
}

func TestArchitectureProductionDoesNotAddTopLevelBusinessPackagesSkeleton(t *testing.T) {
	root := architectureRepositoryRoot(t)
	for _, name := range []string{"roles", "permissions", "menus", "buttons"} {
		if _, err := os.Stat(filepath.Join(root, "internal", name)); err == nil {
			t.Fatalf("internal/%s must not become a first-level business module", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat internal/%s: %v", name, err)
		}
	}
}

func TestArchitectureRetiredTopLevelPackagesAreAbsent(t *testing.T) {
	root := architectureRepositoryRoot(t)
	for _, name := range []string{"handler", "service", "dto", "model", "router", "middleware", "utils", "global"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			t.Errorf("retired top-level package %s still exists", name)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat retired package %s: %v", name, err)
		}
	}
}

func architectureRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	return filepath.Dir(filepath.Dir(currentFile))
}

func architectureProductionGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".go" && !strings.HasSuffix(path, "_test.go") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk architecture production files: %v", err)
	}
	sort.Strings(paths)
	return paths
}
