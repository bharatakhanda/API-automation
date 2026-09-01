//go:build windows

package appgio

import (
	"errors"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func confirmDestructiveAction(title, message string) (bool, error) {
	result, err := win.HWND(0).MessageBox(message, title, co.MB_YESNO|co.MB_ICONWARNING|co.MB_DEFBUTTON2)
	if err != nil {
		return false, err
	}
	return result == co.ID_YES, nil
}

func saveExcelPath(defaultName string) (string, error) {
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()
	releaser := win.NewOleReleaser()
	defer releaser.Release()
	var dialog *win.IFileSaveDialog
	if err := win.CoCreateInstance(releaser, &co.CLSID_FileSaveDialog, nil, co.CLSCTX_INPROC_SERVER, &dialog); err != nil {
		return "", err
	}
	options, err := dialog.GetOptions()
	if err != nil {
		return "", err
	}
	options |= co.FOS_FORCEFILESYSTEM | co.FOS_PATHMUSTEXIST | co.FOS_OVERWRITEPROMPT | co.FOS_NOREADONLYRETURN
	if err := dialog.SetOptions(options); err != nil {
		return "", err
	}
	if err := dialog.SetFileTypes([]win.COMDLG_FILTERSPEC{{Name: "Excel workbook", Spec: "*.xlsx"}}); err != nil {
		return "", err
	}
	if err := dialog.SetFileTypeIndex(1); err != nil {
		return "", err
	}
	if err := dialog.SetDefaultExtension("xlsx"); err != nil {
		return "", err
	}
	if err := dialog.SetFileName(defaultName); err != nil {
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
		return "", errors.New("no export path selected")
	}
	return path, nil
}

func browsePath(folder bool) (string, error) {
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()
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
