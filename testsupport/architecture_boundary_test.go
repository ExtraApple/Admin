package testsupport

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
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
	"messaging":      {},
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
func TestArchitectureIdentityConsumersAreComposedBeforeFullIdentity(t *testing.T) {
	root := architectureRepositoryRoot(t)
	path := filepath.Join(root, "internal", "app", "app.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse internal/app/app.go: %v", err)
	}
	wanted := map[string]struct{}{
		"newIdentityCore":                       {},
		"newAuthorizationService":               {},
		"newNavigationComposition":              {},
		"organizationDescriptorsWithVisibility": {},
		"newIdentityComposition":                {},
	}
	positions := make(map[string]token.Pos, len(wanted))
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		function, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		if _, tracked := wanted[function.Name]; tracked {
			positions[function.Name] = call.Pos()
		}
		return true
	})
	for name := range wanted {
		if positions[name] == token.NoPos {
			t.Fatalf("internal/app/app.go does not call %s", name)
		}
	}
	core := positions["newIdentityCore"]
	fullIdentity := positions["newIdentityComposition"]
	for _, consumer := range []string{"newAuthorizationService", "newNavigationComposition", "organizationDescriptorsWithVisibility"} {
		if positions[consumer] <= core || positions[consumer] >= fullIdentity {
			t.Errorf("%s must be composed after Identity Core and before full Identity", consumer)
		}
	}
}
func TestArchitectureFullIdentityCompositionDoesNotRetainDirectory(t *testing.T) {
	root := architectureRepositoryRoot(t)
	path := filepath.Join(root, "internal", "app", "identity.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse internal/app/identity.go: %v", err)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "identityComposition" {
			return true
		}
		fields, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			t.Fatal("identityComposition must remain a struct")
		}
		for _, field := range fields.Fields.List {
			for _, name := range field.Names {
				if name.Name == "directory" {
					t.Error("identityComposition must not retain Identity Directory; identityCore owns the shared instance")
				}
			}
		}
		return false
	})
}
func TestArchitectureIdentityImportClassification(t *testing.T) {
	tests := []struct {
		name         string
		relativePath string
		importPath   string
		wantAllowed  bool
	}{
		{name: "Identity composition may construct GORM Adapter", relativePath: "internal/app/identity.go", importPath: "admin/internal/identity/adapters/gorm", wantAllowed: true},
		{name: "Identity avatar composition may construct Object Storage Adapter", relativePath: "internal/app/identity_avatar.go", importPath: "admin/internal/identity/adapters/objectstorage", wantAllowed: true},
		{name: "App root may expose Identity HTTP Adapter", relativePath: "internal/app/app.go", importPath: "admin/internal/identity/adapters/http", wantAllowed: true},
		{name: "migration may collect Identity model", relativePath: "internal/app/migrate.go", importPath: "admin/internal/identity", wantAllowed: true},
		{name: "Seed orchestration may invoke Identity Seed Adapter", relativePath: "internal/app/seeddata/seed.go", importPath: "admin/internal/identity/adapters/gorm", wantAllowed: true},
		{name: "cross-module composition may use Identity Application", relativePath: "internal/app/authorization.go", importPath: "admin/internal/identity/application", wantAllowed: true},
		{name: "cross-module composition may use Identity public value", relativePath: "internal/app/organization.go", importPath: "admin/internal/identity/domain", wantAllowed: true},
		{name: "Authorization composition may not import Identity model", relativePath: "internal/app/authorization.go", importPath: "admin/internal/identity", wantAllowed: false},
		{name: "Navigation composition may not import Identity GORM Adapter", relativePath: "internal/app/navigation.go", importPath: "admin/internal/identity/adapters/gorm", wantAllowed: false},
		{name: "App root may not import Identity persistence Adapter", relativePath: "internal/app/app.go", importPath: "admin/internal/identity/adapters/gorm", wantAllowed: false},
		{name: "path prefix cannot bypass Adapter guard", relativePath: "internal/app/organization.go", importPath: "admin/internal/identity/adapters/gorm/internalquery", wantAllowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := architectureIdentityImportAllowed(test.relativePath, test.importPath); got != test.wantAllowed {
				t.Fatalf("architectureIdentityImportAllowed(%q, %q) = %v, want %v", test.relativePath, test.importPath, got, test.wantAllowed)
			}
		})
	}
}

func TestArchitectureIdentityPolicyLiteralClassification(t *testing.T) {
	tests := []struct {
		relativePath string
		literal      string
		wantAllowed  bool
	}{
		{relativePath: "internal/app/migrate.go", literal: "avatar_object_name", wantAllowed: true},
		{relativePath: "internal/app/migrate.go", literal: "avatar_validation_status", wantAllowed: true},
		{relativePath: "internal/app/authorization.go", literal: "avatar_validation_status", wantAllowed: false},
		{relativePath: "internal/app/organization.go", literal: "TrustedAvatarObjectName", wantAllowed: false},
		{relativePath: "internal/app/navigation.go", literal: "/api/avatars/", wantAllowed: false},
	}
	for _, test := range tests {
		if got := architectureIdentityPolicyLiteralAllowed(test.relativePath, test.literal); got != test.wantAllowed {
			t.Errorf("architectureIdentityPolicyLiteralAllowed(%q, %q) = %v, want %v", test.relativePath, test.literal, got, test.wantAllowed)
		}
	}
}

func TestArchitectureAppUserConsumersUseIdentityDirectory(t *testing.T) {
	root := architectureRepositoryRoot(t)
	appRoot := filepath.Join(root, "internal", "app")
	for _, path := range architectureProductionGoFiles(t, appRoot) {
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("resolve relative path for %s: %v", path, err)
		}
		relativePath = filepath.ToSlash(relativePath)
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", relativePath, err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, source, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", relativePath, err)
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if !architectureIdentityImportAllowed(relativePath, importPath) {
				t.Errorf("%s imports forbidden Identity dependency %q; inject an Application Contract instead", relativePath, importPath)
			}
		}
		text := string(source)
		for _, forbidden := range []string{"avatar_object_name", "avatar_validation_status", "TrustedAvatarObjectName", "/api/avatars/"} {
			if strings.Contains(text, forbidden) && !architectureIdentityPolicyLiteralAllowed(relativePath, forbidden) {
				t.Errorf("%s duplicates Identity avatar policy via %q", relativePath, forbidden)
			}
		}
	}
}

func TestArchitectureIdentityHTTPDoesNotQueryUserDirectory(t *testing.T) {
	root := architectureRepositoryRoot(t)
	httpRoot := filepath.Join(root, "internal", "identity", "adapters", "http")
	for _, path := range architectureProductionGoFiles(t, httpRoot) {
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
			if selector.Sel.Name == "ListUsersByIDs" || selector.Sel.Name == "ListUserIDs" {
				t.Errorf("%s queries UserDirectory through %s; Identity HTTP must use existing Application results", path, selector.Sel.Name)
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

// architectureWiringAnnotation is the only accepted dependency wiring
// annotation form: an exact marker plus a required explanation of what breaks
// when the field is missing. Typos and bare markers must fail loudly.
var architectureWiringAnnotation = regexp.MustCompile(`^// wiring: (required|optional) —— .+$`)

type architectureDependencyField struct {
	Owner string
	Name  string
	Level string
}

func TestArchitectureDeclaredDependenciesAreWired(t *testing.T) {
	root := architectureRepositoryRoot(t)
	fields := architectureDependencyFields(t, root)
	if len(fields) == 0 {
		t.Fatal("no Dependencies struct with wiring annotations found under internal/")
	}
	assigned := architectureAssignedDependencyFields(t, root)
	if len(assigned) == 0 {
		t.Fatal("no Dependencies composite literal found in internal/app")
	}
	for _, field := range fields {
		if field.Level != "required" {
			continue
		}
		if _, ok := assigned[field.Owner+"."+field.Name]; !ok {
			t.Errorf("%s.%s is declared required but never assigned in internal/app", field.Owner, field.Name)
		}
	}
}

func architectureDependencyFields(t *testing.T, root string) []architectureDependencyField {
	t.Helper()
	var fields []architectureDependencyField
	for _, path := range architectureProductionGoFiles(t, filepath.Join(root, "internal")) {
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("resolve relative path for %s: %v", path, err)
		}
		relativePath = filepath.ToSlash(relativePath)
		owner := architectureDependencyOwner(relativePath)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			generic, ok := declaration.(*ast.GenDecl)
			if !ok || generic.Tok != token.TYPE {
				continue
			}
			for _, specification := range generic.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if typeSpec.Assign.IsValid() && architectureDependencyTypeName(typeSpec.Type) == "Dependencies" {
					t.Errorf("%s declares the type alias %s = Dependencies, which bypasses the wiring guardrail", relativePath, typeSpec.Name.Name)
					continue
				}
				if typeSpec.Name.Name != "Dependencies" {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					t.Errorf("%s declares Dependencies as a non-struct type", relativePath)
					continue
				}
				for _, structField := range structType.Fields.List {
					names := architectureDependencyFieldNames(structField)
					level, ok := architectureWiringLevel(structField)
					if !ok {
						t.Errorf("%s: Dependencies field %s needs an exact `// wiring: required —— <consequence>` or `// wiring: optional —— <fallback>` annotation",
							relativePath, strings.Join(names, ", "))
						continue
					}
					for _, name := range names {
						fields = append(fields, architectureDependencyField{Owner: owner, Name: name, Level: level})
					}
				}
			}
		}
	}
	return fields
}

func architectureAssignedDependencyFields(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	assigned := map[string]struct{}{}
	for _, path := range architectureProductionGoFiles(t, filepath.Join(root, "internal", "app")) {
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("resolve relative path for %s: %v", path, err)
		}
		relativePath = filepath.ToSlash(relativePath)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		importPaths := architectureImportPaths(file)
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			selector, ok := literal.Type.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Dependencies" {
				return true
			}
			alias, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			importPath, ok := importPaths[alias.Name]
			if !ok {
				t.Errorf("%s: cannot resolve the package of the %s.Dependencies literal", relativePath, alias.Name)
				return true
			}
			for _, element := range literal.Elts {
				key, ok := element.(*ast.KeyValueExpr)
				if !ok {
					t.Errorf("%s: Dependencies literal uses positional values, so the wiring guardrail cannot match field names", relativePath)
					return false
				}
				name, ok := key.Key.(*ast.Ident)
				if !ok {
					continue
				}
				assigned[importPath+"."+name.Name] = struct{}{}
			}
			return true
		})
	}
	return assigned
}

func architectureWiringLevel(field *ast.Field) (string, bool) {
	if field.Doc == nil {
		return "", false
	}
	level := ""
	for _, comment := range field.Doc.List {
		text := strings.TrimRight(comment.Text, " \t")
		if !strings.Contains(text, "wiring") {
			continue
		}
		match := architectureWiringAnnotation.FindStringSubmatch(text)
		if match == nil || level != "" {
			return "", false
		}
		level = match[1]
	}
	return level, level != ""
}

func architectureDependencyFieldNames(field *ast.Field) []string {
	if len(field.Names) == 0 {
		return []string{"<embedded>"}
	}
	names := make([]string, 0, len(field.Names))
	for _, name := range field.Names {
		names = append(names, name.Name)
	}
	return names
}

func architectureDependencyTypeName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	}
	return ""
}

func architectureDependencyOwner(relativePath string) string {
	directory := filepath.ToSlash(filepath.Dir(relativePath))
	if directory == "." {
		return "admin"
	}
	return "admin/" + directory
}

func architectureImportPaths(file *ast.File) map[string]string {
	importPaths := map[string]string{}
	for _, specification := range file.Imports {
		path, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		name := path
		if index := strings.LastIndex(path, "/"); index >= 0 {
			name = path[index+1:]
		}
		if specification.Name != nil {
			if specification.Name.Name == "_" || specification.Name.Name == "." {
				continue
			}
			name = specification.Name.Name
		}
		importPaths[name] = path
	}
	return importPaths
}

func architectureIdentityPolicyLiteralAllowed(relativePath, literal string) bool {
	if filepath.ToSlash(relativePath) != "internal/app/migrate.go" {
		return false
	}
	return literal == "avatar_object_name" || literal == "avatar_validation_status"
}

func architectureIdentityImportAllowed(relativePath, importPath string) bool {
	const identityRoot = "admin/internal/identity"
	if importPath != identityRoot && !strings.HasPrefix(importPath, identityRoot+"/") {
		return true
	}
	if importPath == identityRoot+"/application" || importPath == identityRoot+"/domain" {
		return true
	}
	relativePath = filepath.ToSlash(relativePath)
	switch relativePath {
	case "internal/app/identity.go", "internal/app/identity_avatar.go":
		return true
	case "internal/app/app.go":
		return importPath == identityRoot+"/adapters/http"
	case "internal/app/migrate.go":
		return importPath == identityRoot
	}
	if strings.HasPrefix(relativePath, "internal/app/seeddata/") {
		return importPath == identityRoot+"/adapters/gorm"
	}
	return false
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
