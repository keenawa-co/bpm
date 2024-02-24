package linker

import (
	"context"
	"fmt"
	"testing"

	"github.com/4rchr4y/bpm/bundleutil/encode"
	"github.com/4rchr4y/bpm/bundleutil/inspect"
	"github.com/4rchr4y/bpm/bundleutil/manifest"
	"github.com/4rchr4y/bpm/fetch"
	"github.com/4rchr4y/bpm/iostream"

	"github.com/4rchr4y/bpm/storage"
	"github.com/4rchr4y/godevkit/v3/env"
	"github.com/4rchr4y/godevkit/v3/syswrap"
)

func TestLink(t *testing.T) {
	dir := env.MustGetString("BPM_PATH")

	io := iostream.NewIOStream()

	osWrap := new(syswrap.OSWrap)
	ioWrap := new(syswrap.IOWrap)
	encoder := &encode.Encoder{
		IO: io,
	}

	inspector := &inspect.Inspector{
		IO: io,
	}

	s := &storage.Storage{
		Dir:     dir,
		IO:      io,
		OSWrap:  osWrap,
		IOWrap:  ioWrap,
		Encoder: encoder,
	}

	fetcher := &fetch.Fetcher{
		IO:        io,
		Storage:   s,
		Inspector: inspector,
		GitHub: &fetch.GithubFetcher{
			IO:      io,
			Client:  nil,
			Encoder: encoder,
		},
	}

	manifester := &manifest.Manifester{
		IO:      io,
		OSWrap:  osWrap,
		Storage: s,
		Encoder: encoder,
		Fetcher: fetcher,
	}

	b, err := s.LoadFromAbs("../../testdata/testbundle", nil)
	if err != nil {
		t.Fatal(err)
	}

	l := Linker{
		Fetcher:    fetcher,
		Manifester: manifester,
		Inspector:  inspector,
	}

	modules, err := l.Link(context.TODO(), b)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println(len(modules))
	t.Fail()
}