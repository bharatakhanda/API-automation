package main

import wails "github.com/wailsapp/wails/v3/pkg/application"

type nativeDialogs struct {
	app *wails.App
}

func (dialogs nativeDialogs) SelectFolder() (string, error) {
	return dialogs.app.Dialog.OpenFile().
		SetTitle("Select test folder").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(false).
		PromptForSingleSelection()
}

func (dialogs nativeDialogs) SelectFile() (string, error) {
	return dialogs.app.Dialog.OpenFile().
		SetTitle("Select Fiery test file").
		CanChooseDirectories(false).
		CanChooseFiles(true).
		AllowsOtherFileTypes(false).
		AddFilter("Fiery job files", "*.pdf;*.ps;*.eps;*.prn;*.tif;*.tiff;*.jpg;*.jpeg;*.png").
		PromptForSingleSelection()
}

func (dialogs nativeDialogs) SelectExcelPath() (string, error) {
	return dialogs.app.Dialog.SaveFile().
		SetMessage("Export automation results").
		SetFilename("api-automation-results.xlsx").
		AddFilter("Excel workbook", "*.xlsx").
		PromptForSingleSelection()
}

func (dialogs nativeDialogs) Confirm(title, message string) (bool, error) {
	confirmed := false
	dialog := dialogs.app.Dialog.Question().SetTitle(title).SetMessage(message)
	confirm := dialog.AddButton("Confirm").OnClick(func() { confirmed = true })
	cancel := dialog.AddButton("Cancel")
	dialog.SetDefaultButton(cancel)
	dialog.SetCancelButton(cancel)
	dialog.Show()
	_ = confirm
	return confirmed, nil
}
