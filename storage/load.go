package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/4rchr4y/bpm/bundle"
	"github.com/4rchr4y/bpm/bundle/bundlefile"
	"github.com/4rchr4y/bpm/bundle/lockfile"
	"github.com/4rchr4y/bpm/constant"
	"github.com/4rchr4y/bpm/regoutil"
)

type ErrNotExist struct{}

func (ErrNotExist) Error() string { return "bundle does not exist" }

func (s *Storage) Load(source string, version *bundle.VersionSpec) (*bundle.Bundle, error) {
	if strings.TrimSpace(source) == "" || version == nil {
		return nil, ErrNotExist{}
	}

	return s.LoadFromAbs(
		s.MakeBundleSourcePath(source, version.String()),
		version,
	)
}

func (s *Storage) LoadFromAbs(dir string, v *bundle.VersionSpec) (*bundle.Bundle, error) {
	ok, err := s.OSWrap.Exists(dir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotExist{}
	}

	absDirPath, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path for %s: %v", dir, err)
	}

	s.IO.PrintfInfo("loading from %s", absDirPath)

	ignoreFile, err := s.readIgnoreFile(absDirPath)
	if err != nil {
		return nil, err
	}

	bundleFile, err := s.readBundleFile(absDirPath)
	if err != nil {
		return nil, err
	}

	if err := regoutil.PrepareDocumentParser(bundleFile); err != nil {
		return nil, err
	}

	lockFile, err := s.readLockFile(absDirPath)
	if err != nil {
		return nil, err
	}

	files, err := s.readBundleDir(absDirPath, ignoreFile)
	if err != nil {
		return nil, err
	}

	fileifyOutput, err := s.Encoder.Fileify(files)
	if err != nil {
		return nil, err
	}

	return &bundle.Bundle{
		Version:    v,
		BundleFile: bundlefile.PrepareSchema(bundleFile),
		LockFile:   lockfile.PrepareSchema(lockFile),
		RegoFiles:  fileifyOutput.RegoFiles,
		IgnoreFile: ignoreFile,
		OtherFiles: fileifyOutput.OtherFiles,
	}, nil
}

func (fetcher *Storage) readBundleDir(abs string, ignoreFile *bundle.IgnoreFile) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := fetcher.OSWrap.Walk(abs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error occurred while accessing a path %s: %v", path, err)
		}

		relativePath, err := filepath.Rel(abs, path)
		if err != nil {
			return fmt.Errorf("error getting relative path for %s from %s: %v", path, abs, err)
		}

		if ignoreFile != nil && ignoreFile.Some(relativePath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			content, err := fetcher.readFileContent(path)
			if err != nil {
				return err
			}
			files[relativePath] = content
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking the path %s: %v", abs, err)
	}

	return files, nil
}

func (fetcher *Storage) readFileContent(path string) ([]byte, error) {
	file, err := fetcher.OSWrap.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// TODO: use bufio buffer instead of ReadAll
	return fetcher.IOWrap.ReadAll(file)
}

func (fetcher *Storage) readIgnoreFile(dir string) (*bundle.IgnoreFile, error) {
	ignoreFilePath := filepath.Join(dir, constant.IgnoreFileName)
	content, err := fetcher.readFileContent(ignoreFilePath)
	if err != nil {
		return nil, err
	}

	ignoreFile, err := fetcher.Encoder.DecodeIgnoreFile(content)
	if err != nil {
		return nil, fmt.Errorf("error occurred while decoding %s content: %v", constant.IgnoreFileName, err)
	}

	return ignoreFile, nil
}

func (fetcher *Storage) readLockFile(dir string) (*lockfile.Schema, error) {
	lockFilePath := filepath.Join(dir, constant.LockFileName)
	content, err := fetcher.readFileContent(lockFilePath)
	if err != nil {
		return nil, err
	}

	lockFile, err := fetcher.Encoder.DecodeLockFile(content)
	if err != nil {
		return nil, fmt.Errorf("error occurred while decoding %s content: %v", constant.LockFileName, err)
	}

	return lockFile, nil
}

func (fetcher *Storage) readBundleFile(dir string) (*bundlefile.Schema, error) {
	bundleFilePath := filepath.Join(dir, constant.BundleFileName)
	content, err := fetcher.readFileContent(bundleFilePath)
	if err != nil {
		return nil, err
	}

	bundleFile, err := fetcher.Encoder.DecodeBundleFile(content)
	if err != nil {
		return nil, fmt.Errorf("error occurred while decoding %s content: %v", constant.BundleFileName, err)
	}

	return bundleFile, nil
}
