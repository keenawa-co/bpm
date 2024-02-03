package bundleutil

import (
	"fmt"
	"path/filepath"

	"github.com/4rchr4y/bpm/constant"
	"github.com/4rchr4y/bpm/pkg/bundle"
	"github.com/4rchr4y/bpm/pkg/bundle/bundlefile"
	"github.com/4rchr4y/bpm/pkg/bundle/lockfile"
	"github.com/4rchr4y/bpm/pkg/bundle/regofile"
	"github.com/open-policy-agent/opa/ast"
)

type fileifierEncoder interface {
	DecodeBundleFile(content []byte) (*bundlefile.File, error)
	DecodeLockFile(content []byte) (*lockfile.File, error)
}

type fileifierManifester interface {
	InitLockFile(b *bundle.Bundle) error
}

type Fileifier struct {
	manifester fileifierManifester
	encoder    fileifierEncoder
}

func NewFileifier(encoder fileifierEncoder, manifester fileifierManifester) *Fileifier {
	return &Fileifier{
		manifester: manifester,
		encoder:    encoder,
	}
}

func (bp *Fileifier) Fileify(files map[string][]byte, options ...BundleOptFn) (*bundle.Bundle, error) {
	b := &bundle.Bundle{
		RegoFiles:  make(map[string]*regofile.File),
		OtherFiles: make(map[string][]byte),
	}

	for i := range options {
		options[i](b)
	}

	modules := &lockfile.ModulesDecl{
		List: make([]*lockfile.ModDecl, 0),
	}

	for filePath, content := range files {
		switch {
		case isRegoFile(filePath):
			parsed, err := bp.parseRegoFile(content, filePath)
			if err != nil {
				return nil, err
			}

			file := &regofile.File{Path: filePath, Parsed: parsed}
			b.RegoFiles[filePath] = file

			modules.List = append(modules.List, &lockfile.ModDecl{
				Package: file.Package(),
				Source:  filePath,
				Sum:     file.Sum(),
			})

		case isBPMFile(filePath):
			bundlefile, err := bp.encoder.DecodeBundleFile(content)
			if err != nil {
				return nil, fmt.Errorf("error occurred while decoding %s content: %v", constant.BundleFileName, err)
			}

			b.BundleFile = bundlefile

		case isBPMLockFile(filePath):
			lockfile, err := bp.encoder.DecodeLockFile(content)
			if err != nil {
				return nil, fmt.Errorf("error occurred while decoding %s content: %v", constant.BundleFileName, err)
			}

			b.LockFile = lockfile

		default:
			b.OtherFiles[filePath] = content
		}
	}

	if b.LockFile == nil {
		fmt.Println("WARRING: was no lockfile")
		if err := bp.manifester.InitLockFile(b); err != nil {
			return nil, err
		}
	}

	if len(modules.List) > 0 {
		b.LockFile.Modules = modules.Sort()
	}

	return b, nil
}

func (bp *Fileifier) parseRegoFile(fileContent []byte, filePath string) (*ast.Module, error) {
	parsed, err := ast.ParseModule(filePath, string(fileContent))
	if err != nil {
		return nil, fmt.Errorf("error parsing file contents: %v", err)
	}

	return parsed, nil
}

func isRegoFile(filePath string) bool    { return filepath.Ext(filePath) == constant.RegoFileExt }
func isBPMFile(filePath string) bool     { return filePath == constant.BundleFileName }
func isBPMLockFile(filePath string) bool { return filePath == constant.LockFileName }
func isEmpty(content []byte) bool        { return len(content) < 1 }
