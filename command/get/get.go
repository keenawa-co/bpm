package get

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/4rchr4y/bpm/cli/require"
	"github.com/4rchr4y/bpm/command/factory"
	"github.com/4rchr4y/bpm/constant"
	"github.com/4rchr4y/bpm/pkg/encode"
	"github.com/4rchr4y/bpm/pkg/install"
	"github.com/4rchr4y/bpm/pkg/load/gitload"
	"github.com/4rchr4y/bpm/pkg/load/osload"
	"github.com/spf13/cobra"
)

func NewCmdGet(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get a new dependency",
		Args:  require.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			version, err := cmd.Flags().GetString("version")
			if err != nil {
				return err
			}

			wd, err := os.Getwd()
			if err != nil {
				return err
			}

			return getRun(&getOptions{
				WorkDir:   wd,
				URL:       args[0],
				Version:   version,
				GitLoader: f.GitLoader,
				OsLoader:  f.OsLoader,
				Installer: f.Installer,
				Encoder:   f.Encoder,
				WriteFile: f.OS.WriteFile,
			})
		},
	}

	cmd.Flags().StringP("version", "v", "", "Bundle version")
	return cmd
}

type getOptions struct {
	WorkDir   string                                                 // bundle working directory
	URL       string                                                 // bundle repository that needs to be installed
	Version   string                                                 // specified bundle version
	GitLoader *gitload.GitLoader                                     // bundle file loader from the git repo
	OsLoader  *osload.OsLoader                                       // bundle file loader from file system
	Installer *install.BundleInstaller                               // bundle installer into the file system
	Encoder   *encode.BundleEncoder                                  // decoder of bundle component files
	WriteFile func(name string, data []byte, perm fs.FileMode) error // func of saving a file to disk
}

func getRun(opts *getOptions) error {
	workingBundle, err := opts.OsLoader.LoadBundle(opts.WorkDir)
	if err != nil {
		return err
	}

	b, err := opts.GitLoader.DownloadBundle(opts.URL, opts.Version)
	if err != nil {
		return err
	}

	if err := opts.Installer.Install(b); err != nil {
		return err
	}

	if err := workingBundle.Require(b); err != nil {
		return err
	}

	bundlefilePath := filepath.Join(opts.WorkDir, constant.BundleFileName)
	bytes := opts.Encoder.EncodeBundleFile(workingBundle.BundleFile)

	if err := opts.WriteFile(bundlefilePath, bytes, 0644); err != nil {
		return err
	}

	return nil
}