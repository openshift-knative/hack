package imports

import (
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	ErrCompileError = fmt.Errorf("could't compile")
	ErrIO           = fmt.Errorf("io fail")
)

// ScanForMains will scan a root dir, in specified packages to collect the
// packages that have a main() function. It works for vendorless and vendorful
// projects.
func ScanForMains(rootDir string, packages []string, tags []string) (sets.Set[string], error) {
	pkgs := sets.New[string]()
	collctr, err := collector(rootDir, pkgs)
	if err != nil {
		return nil, err
	}
	for _, subpkg := range packages {
		if err := filepath.WalkDir(path.Join(rootDir, subpkg), scanner(collctr, tags)); err != nil {
			return nil, err
		}
	}

	return pkgs, nil
}

type collectOnlyMainFn func(imprts []string) error

func collector(rootDir string, pkgs sets.Set[string]) (collectOnlyMainFn, error) {
	gomodPath := path.Join(rootDir, "go.mod")
	contents, err := os.ReadFile(gomodPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIO, errors.WithStack(err))
	}
	gm, err := modfile.ParseLax(gomodPath, contents, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCompileError, errors.WithStack(err))
	}
	return func(imprts []string) error {
		for _, imprt := range imprts {
			ok, err := isDepMainPackage(rootDir, gm, imprt)
			if err != nil {
				return err
			}
			if ok {
				pkgs.Insert(imprt)
			}
		}
		return nil
	}, nil
}

func isDepMainPackage(rootDir string, gm *modfile.File, imprt string) (bool, error) {
	// within repo
	if strings.HasPrefix(imprt, gm.Module.Mod.Path) {
		subimprt := strings.TrimPrefix(imprt, gm.Module.Mod.Path)
		pkgPath := path.Join(rootDir, subimprt)
		return isMainPkg(pkgPath)
	}
	// try vendor
	pkgPath := path.Join(rootDir, "vendor", imprt)
	fi, err := os.Stat(pkgPath)
	if err == nil && fi.IsDir() {
		return isMainPkg(pkgPath)
	}
	// modules
	var mod module.Version
	for _, req := range gm.Require {
		if strings.HasPrefix(imprt, req.Mod.Path) {
			mod = req.Mod
			break
		}
	}
	for _, repl := range gm.Replace {
		if repl.Old.Path == mod.Path {
			mod = repl.New
			break
		}
	}
	subIprt := strings.TrimPrefix(imprt, mod.Path)
	pkgPath = path.Join(goModCache(), mod.Path+"@"+mod.Version, subIprt)
	return isMainPkg(pkgPath)
}

func isMainPkg(pkgPath string) (bool, error) {
	response := false
	if err := filepath.WalkDir(pkgPath, func(path string, info fs.DirEntry, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".go") {
			if path != pkgPath && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%w: ReadFile %s failed: %w",
				ErrIO, path, errors.WithStack(err))
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, string(content), parser.PackageClauseOnly)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrCompileError, errors.WithStack(err))
		}
		if f.Name.Name == "main" {
			response = true
			return filepath.SkipAll
		}
		return nil
	}); err != nil {
		return false, err
	}
	return response, nil
}

func scanner(collectOnlyMainFn collectOnlyMainFn, tags []string) fs.WalkDirFunc {
	// TODO: limit the Go files to the tags provided
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%w: ReadFile %s failed: %w",
				ErrIO, path, errors.WithStack(err))
		}
		imprts, err := collectImports(path, content)
		if err != nil {
			return err
		}
		return collectOnlyMainFn(imprts)
	}
}

func collectImports(pth string, content []byte) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, pth, string(content), parser.ImportsOnly)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCompileError, errors.WithStack(err))
	}
	imprts := make([]string, 0, len(f.Imports))
	for _, spec := range f.Imports {
		imprt := strings.TrimSuffix(strings.TrimPrefix(spec.Path.Value, "\""), "\"")
		imprts = append(imprts, imprt)
	}
	return imprts, nil
}

func goModCache() string {
	return envOr("GOMODCACHE", gopathDir("pkg/mod"))
}

func envOr(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		val = def
	}
	return val
}

func gopathDir(rel string) string {
	list := filepath.SplitList(gopath())
	if len(list) == 0 || list[0] == "" {
		return ""
	}
	return filepath.Join(list[0], rel)
}

func gopath() string {
	gp := os.Getenv("GOPATH")
	if gp == "" {
		gp = build.Default.GOPATH
	}
	return gp
}
