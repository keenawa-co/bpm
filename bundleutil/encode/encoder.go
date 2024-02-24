package encode

import (
	"bufio"
	"bytes"
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
	"github.com/4rchr4y/bpm/iostream/iostreamiface"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsimple"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/open-policy-agent/opa/ast"
)

type Encoder struct {
	IO iostreamiface.IO
}

func (e *Encoder) DecodeIgnoreFile(content []byte) (*bundle.IgnoreFile, error) {
	ignoreFile := bundle.NewIgnoreFile()
	scanner := bufio.NewScanner(bytes.NewReader(content))

	var keys []string
	for scanner.Scan() {
		key := strings.TrimSpace(scanner.Text())
		if key != "" {
			keys = append(keys, key)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading '%s' content: %v", constant.IgnoreFileName, err)
	}

	sort.Strings(keys)
	ignoreFile.IsSorted = true

	for i := range keys {
		ignoreFile.Store(keys[i])
	}

	return ignoreFile, nil
}

func (e *Encoder) DecodeBundleFile(content []byte) (*bundlefile.Schema, error) {
	schema := new(bundlefile.Schema)
	if err := hclsimple.Decode(constant.BundleFileName, content, nil, schema); err != nil {
		return nil, err
	}

	return schema, nil
}

func (e *Encoder) DecodeLockFile(content []byte) (*lockfile.Schema, error) {
	schema := new(lockfile.Schema)
	if err := hclsimple.Decode(constant.LockFileName, content, nil, schema); err != nil {
		return nil, err
	}

	return schema, nil
}

func (e *Encoder) EncodeBundleFile(bundlefile *bundlefile.Schema) []byte {
	f := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(bundlefile, f.Body())

	return bundleutil.FormatBundleFile(f.Bytes())
}

const lockfileComment = "// This file has been auto-generated by `bpm`.\n// It is not meant to be edited manually."

func (e *Encoder) EncodeLockFile(lockfile *lockfile.Schema) (result []byte) {
	tempFile := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(lockfile, tempFile.Body())

	f := hclwrite.NewEmptyFile()

	f.Body().AppendUnstructuredTokens([]*hclwrite.Token{
		{Type: hclsyntax.TokenComment, Bytes: []byte(lockfileComment)},
		{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")},
		{Type: hclsyntax.TokenOBrace, Bytes: tempFile.Bytes()},
	})

	return bundleutil.FormatLockFile(f.Bytes())
}

func (e *Encoder) EncodeIgnoreFile(ignorefile *bundle.IgnoreFile) []byte {
	var builder strings.Builder

	keys := make([]string, 0, len(ignorefile.List))
	for key := range ignorefile.List {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteRune('\n')
	}

	return []byte(builder.String())
}

type FileifyOutput struct {
	RegoFiles  map[string]*regofile.File
	OtherFiles map[string][]byte
}

func (e *Encoder) Fileify(files map[string][]byte) (*FileifyOutput, error) {
	output := &FileifyOutput{
		RegoFiles:  make(map[string]*regofile.File, len(files)),
		OtherFiles: make(map[string][]byte),
	}

	for filePath, content := range files {
		switch {
		case isRegoFile(filePath):
			parsed, err := ast.ParseModule(filePath, string(content))
			if err != nil {
				return nil, fmt.Errorf("error parsing file contents: %v", err)
			}

			output.RegoFiles[filePath] = &regofile.File{
				Path:   filePath,
				Parsed: parsed,
				Raw:    content,
			}

		// case isLockFile(filePath):
		// 	lockfile, err := e.DecodeLockFile(content)
		// 	if err != nil {
		// 		return nil, fmt.Errorf("error occurred while decoding %s content: %v", constant.BundleFileName, err)
		// 	}

		// 	output.LockFile = lockfile

		case isIgnoreFile(filePath) || isBundleFile(filePath) || isLockFile(filePath):
			// Ignoring this file is due to the fact that it must be read
			// and processed BEFORE this function is executed in order to
			// avoid loading files into memory that are not required at
			// runtime(.bpmignore) or to prepare(bundle.hcl) the rego environment
			// before parsing rego files.
			//
			// In addition, explicit ignoring of this file is necessary so
			// that this file does not end up in the list of OtherFiles.
			// Otherwise, this will cause a difference in the checksums,
			// since when the bundle was created, this file was not in
			// the list of OtherFiles, and if it is not explicitly ignored,
			// it will end up there.
			continue

		default:
			output.OtherFiles[filePath] = content
		}
	}

	return output, nil
}

func isRegoFile(filePath string) bool   { return filepath.Ext(filePath) == constant.RegoFileExt }
func isBundleFile(filePath string) bool { return filePath == constant.BundleFileName }
func isLockFile(filePath string) bool   { return filePath == constant.LockFileName }
func isIgnoreFile(filePath string) bool { return filePath == constant.IgnoreFileName }
