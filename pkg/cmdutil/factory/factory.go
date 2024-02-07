package factory

import (
	"github.com/4rchr4y/bpm/core"
	"github.com/4rchr4y/bpm/internal/gitfacade"
	"github.com/4rchr4y/bpm/pkg/bundleutil"
	"github.com/4rchr4y/bpm/pkg/bundleutil/encode"
	"github.com/4rchr4y/bpm/pkg/fetch"
	"github.com/4rchr4y/godevkit/syswrap/ioiface"
	"github.com/4rchr4y/godevkit/syswrap/osiface"
)

type Factory struct {
	Name    string // executable name
	Version string // app version

	IOStream core.IO
	Verifier *bundleutil.Verifier
	Encoder  *encode.Encoder // decoder of bundle component files
	// OsLoader   *osload.OsLoader       // bundle file loader from file system
	GitFacade *gitfacade.GitFacade // facade for interaction with both the CLI and the API
	// GitLoader  *gitload.GitLoader     // bundle file loader from the git repo
	Fetcher    *fetch.Fetcher
	Saver      *bundleutil.Saver      // bundle saver files into the file system
	Downloader *bundleutil.Downloader // downloader of a bundle and its dependencies
	Manifester *bundleutil.Manifester // bundle manifest file control operator
	OS         osiface.OSWrapper      // set of functions for working with the OS
	IO         ioiface.IOWrapper      // set of functions for working with input/output
}
