//go:build windows

package appgio

import (
	"errors"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func browsePath(folder bool) (string, error) {
	releaser := win.NewOleReleaser()
	defer releaser.Release()
	var dialog *win.IFileOpenDialog
	if err := win.CoCreateInstance(releaser, &co.CLSID_FileOpenDialog, nil, co.CLSCTX_INPROC_SERVER, &dialog); err != nil {
		return "", err
	}
	options, err := dialog.GetOptions()
	if err != nil {
		return "", err
	}
	options |= co.FOS_FORCEFILESYSTEM | co.FOS_PATHMUSTEXIST
	if folder {
		options |= co.FOS_PICKFOLDERS
	} else {
		options |= co.FOS_FILEMUSTEXIST
	}
	if err := dialog.SetOptions(options); err != nil {
		return "", err
	}
	ok, err := dialog.Show(0)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	item, err := dialog.GetResult(releaser)
	if err != nil {
		return "", err
	}
	path, err := item.GetDisplayName(co.SIGDN_FILESYSPATH)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("no path selected")
	}
	return path, nil
}
