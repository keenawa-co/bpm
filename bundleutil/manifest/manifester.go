package manifest

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/4rchr4y/bpm/bundle"
	"github.com/4rchr4y/bpm/bundle/bundlefile"
	"github.com/4rchr4y/bpm/bundle/lockfile"
	"github.com/4rchr4y/bpm/bundle/regofile"
	"github.com/4rchr4y/bpm/bundleutil"
	"github.com/4rchr4y/bpm/constant"
	"github.com/4rchr4y/bpm/fetch"
	"github.com/4rchr4y/bpm/iostream/iostreamiface"
	"github.com/4rchr4y/godevkit/v3/syswrap/osiface"
)

var compOp = map[bool]string{true: "=>", false: "<="}

func NewBundlefileRequirementDecl(b *bundle.Bundle) *bundlefile.RequirementDecl {
	return &bundlefile.RequirementDecl{
		Repository: b.Repository(),
		Name:       b.Name(),
		Version:    b.Version.String(),
	}
}

func NewLockfileRequirementDecl(b *bundle.Bundle, direction lockfile.DirectionType) *lockfile.RequirementDecl {
	return &lockfile.RequirementDecl{
		Repository: b.Repository(),
		Direction:  direction.String(),
		Name:       b.Name(),
		Version:    b.Version.String(),
		H1:         b.BundleFile.Sum(),
		H2:         b.Sum(),
	}
}

type manifesterEncoder interface {
	EncodeBundleFile(bundlefile *bundlefile.Schema) []byte
	EncodeLockFile(lockfile *lockfile.Schema) []byte
}

type manifesterStorage interface {
	Store(b *bundle.Bundle) error
	Some(repo string, version string) bool
	StoreSome(b *bundle.Bundle) error
	Load(source string, version *bundle.VersionSpec) (*bundle.Bundle, error)
}

type manifesterFetcher interface {
	Fetch(ctx context.Context, source string, version *bundle.VersionSpec) (*fetch.FetchResult, error)
}

type Manifester struct {
	IO      iostreamiface.IO
	OSWrap  osiface.OSWrapper
	Storage manifesterStorage
	Encoder manifesterEncoder
	Fetcher manifesterFetcher
}

type InsertRequirementInput struct {
	Parent  *bundle.Bundle
	Source  string
	Version *bundle.VersionSpec
}

func (m *Manifester) InsertRequirement(ctx context.Context, input *InsertRequirementInput) error {
	if input.Parent.Repository() == input.Source {
		return errors.New("installing a bundle into itself is not allowed")
	}

	existingRequirement, idx, ok := input.Parent.BundleFile.FindIndexOfRequirement(
		bundlefile.FilterBySource(input.Source),
	)

	if ok && existingRequirement.Version == input.Version.String() {
		m.IO.PrintfOk(
			"bundle %s is already installed",
			bundleutil.FormatSourceWithVersion(input.Source, input.Version.String()),
		)
		return m.SyncLockfile(ctx, input.Parent) // such requirement is already installed, then just synchronize
	}

	result, err := m.Fetcher.Fetch(ctx, input.Source, input.Version)
	if err != nil {
		return err
	}

	if ok {
		existingVersion, err := bundle.ParseVersionExpr(existingRequirement.Version)
		if err != nil {
			return err
		}

		if result.Target.Version.String() == existingVersion.String() {
			m.IO.PrintfOk(
				"bundle %s is already installed",
				bundleutil.FormatSourceWithVersion(result.Target.Repository(), result.Target.Version.String()),
			)
			return m.SyncLockfile(ctx, input.Parent)
		}

		isGreater := result.Target.Version.GreaterThan(existingVersion)
		if !isGreater {
			m.IO.PrintfWarn(
				"installing an older bundle %s version",
				bundleutil.FormatSourceWithVersion(result.Target.Repository(), result.Target.Version.String()),
			)
		}

		m.IO.PrintfInfo("upgrading %s %s %s",
			bundleutil.FormatSourceWithVersion(input.Source, input.Version.String()),
			compOp[isGreater],
			bundleutil.FormatSourceWithVersion(result.Target.Repository(), result.Target.Version.String()),
		)

		input.Parent.BundleFile.Require.List[idx] = NewBundlefileRequirementDecl(result.Target)

		return m.SyncLockfile(ctx, input.Parent)
	}

	input.Parent.BundleFile.Require.List = append(
		input.Parent.BundleFile.Require.List,
		NewBundlefileRequirementDecl(result.Target),
	)

	return m.SyncLockfile(ctx, input.Parent)
}

func (m *Manifester) SyncLockfile(ctx context.Context, parent *bundle.Bundle) error {
	// creating a cache for faster matching with bundle file
	requireCache := make(map[string]struct{})

	for i, req := range parent.LockFile.Require.List {
		// while going through the lock requirements, simultaneously
		// remove requirements that no longer exist in bundle file
		if exists := parent.BundleFile.SomeRequirement(
			bundlefile.FilterBySource(req.Repository),
			bundlefile.FilterByVersion(req.Version),
		); !exists {
			parent.LockFile.Require.List[i] = nil
			continue
		}

		// creating a cache of lock requirements
		formattedVersion := bundleutil.FormatSourceWithVersion(req.Repository, req.Version)
		requireCache[formattedVersion] = struct{}{}

	}

	// list of all bundles required for the bath bundle,
	// it is necessary for in-depth comparison of imports
	requireList := make(map[string]*bundle.Bundle, 0)

	// go through all direct requirements to ensure that the
	// lock file is up to date
	for _, r := range parent.BundleFile.Require.List {
		v, err := bundle.ParseVersionExpr(r.Version)
		if err != nil {
			return err
		}

		result, err := m.Fetcher.Fetch(ctx, r.Repository, v)
		if err != nil {
			return err
		}

		requireList[result.Target.Name()] = result.Target

		for _, b := range result.Merge() {
			if err := m.Storage.StoreSome(b); err != nil {
				return err
			}

			key := bundleutil.FormatSourceWithVersion(b.Repository(), b.Version.String())
			if _, exists := requireCache[key]; !exists {
				parent.LockFile.Require.List = append(
					parent.LockFile.Require.List,
					NewLockfileRequirementDecl(b, defineDirection(result.Target, b)),
				)
			}
		}
	}

	modules, err := m.parseModuleList(parent, requireList)
	if err != nil {
		return err
	}

	parent.LockFile.Sum = parent.Sum()
	parent.LockFile.Modules = &lockfile.ModulesBlock{List: modules}
	return nil
}

func defineDirection(target, actual *bundle.Bundle) lockfile.DirectionType {
	if actual.Repository() == target.Repository() {
		return lockfile.Direct
	}

	return lockfile.Indirect
}

func (m *Manifester) parseModuleList(b *bundle.Bundle, requireList map[string]*bundle.Bundle) ([]*lockfile.ModDecl, error) {
	result := make([]*lockfile.ModDecl, 0, len(b.RegoFiles))

	for filePath, f := range b.RegoFiles {
		requireList, err := m.parseRequireList(requireList, f)
		if err != nil {
			return nil, err
		}

		result = append(result, &lockfile.ModDecl{
			Package: b.BundleFile.Package.Name + "." + f.Package(),
			Source:  filePath,
			Sum:     f.Sum(),
			Require: requireList,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Package < result[j].Package
	})

	return result, nil
}

func (m *Manifester) parseRequireList(requireList map[string]*bundle.Bundle, f *regofile.File) ([]string, error) {
	if len(f.Parsed.Imports) == 0 {
		return nil, nil
	}

	// list of known imports, it is required to detect duplicate imports
	knownImports := make(map[string]struct{}, 0)
	result := make([]string, 0, len(f.Parsed.Imports))
	for _, v := range f.Parsed.Imports {
		pathStr := v.Path.String()
		importPath := strings.TrimPrefix(pathStr, regofile.ImportPathPrefix)

		if _, exists := knownImports[importPath]; exists {
			m.IO.PrintfWarn("duplicated import '%s' detected in %s:%d", pathStr, f.Path, v.Location.Row)
			continue
		}

		packageName := importPath
		dotIndex := strings.Index(importPath, ".")
		if dotIndex != -1 {
			packageName = importPath[:dotIndex]
		}

		// check that the package used really exists for this bundle
		required, exists := requireList[packageName]
		if !exists {
			return nil, fmt.Errorf("undefined import '%s' in %s", pathStr, f.Path)
		}

		// check that the module used exists in the specified package
		if exists := required.LockFile.SomeModule(
			lockfile.ModulesFilterByPackage(importPath),
		); !exists {
			return nil, fmt.Errorf("undefined import '%s' in %s", pathStr, f.Path)
		}

		// save information that this file requires a bundle of a specific version
		source := bundleutil.FormatSourceWithVersion(required.Repository(), required.Version.String())
		result = append(
			result,
			lockfile.NewModRequireSpec(v.Location.Row, source, importPath).String(),
		)

		// mark this import as already identified
		knownImports[importPath] = struct{}{}
	}

	return result, nil
}

func (m *Manifester) Upgrade(workDir string, b *bundle.Bundle) error {
	if err := m.upgradeBundleFile(workDir, b); err != nil {
		return err
	}

	if err := m.upgradeLockFile(workDir, b); err != nil {
		return err
	}

	m.IO.PrintfDebug("bundle %s has been successfully upgraded", b.Repository())
	return nil
}

func (m *Manifester) upgradeBundleFile(workDir string, b *bundle.Bundle) error {
	bundlefilePath := filepath.Join(workDir, constant.BundleFileName)
	bytes := m.Encoder.EncodeBundleFile(b.BundleFile)

	if err := m.OSWrap.WriteFile(bundlefilePath, bytes, 0644); err != nil {
		return fmt.Errorf("error occurred while '%s' file updating: %v", constant.BundleFileName, err)
	}

	return nil
}

func (m *Manifester) upgradeLockFile(workDir string, b *bundle.Bundle) error {
	bundlefilePath := filepath.Join(workDir, constant.LockFileName)
	bytes := m.Encoder.EncodeLockFile(b.LockFile)

	if err := m.OSWrap.WriteFile(bundlefilePath, bytes, 0644); err != nil {
		return fmt.Errorf("error occurred while '%s' file updating: %v", constant.LockFileName, err)
	}

	return nil
}
