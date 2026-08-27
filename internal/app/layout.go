package app

const (
	windowWidth  = 1180
	windowHeight = 760
)

type uiRect struct {
	Name       string
	X, Y, W, H int
	Layer      string
}

func enterpriseLayoutRects() []uiRect {
	return []uiRect{
		{Name: "settingsButton", X: 620, Y: 24, W: 110, H: 30, Layer: "action"},
		{Name: "captureButton", X: 742, Y: 24, W: 240, H: 30, Layer: "action"},
		{Name: "runButton", X: 994, Y: 24, W: 142, H: 30, Layer: "action"},
		{Name: "cancelButton", X: 994, Y: 60, W: 142, H: 28, Layer: "action"},
		{Name: "folderPath", X: 220, Y: 126, W: 650, H: 26, Layer: "input"},
		{Name: "browseFolder", X: 884, Y: 125, W: 96, H: 28, Layer: "action"},
		{Name: "selectionMode", X: 220, Y: 184, W: 150, H: 26, Layer: "input"},
		{Name: "filePath", X: 396, Y: 184, W: 474, H: 26, Layer: "input"},
		{Name: "browseFile", X: 884, Y: 183, W: 96, H: 28, Layer: "action"},
		{Name: "method", X: 220, Y: 278, W: 92, H: 26, Layer: "input"},
		{Name: "url", X: 336, Y: 278, W: 510, H: 26, Layer: "input"},
		{Name: "concurrency", X: 860, Y: 278, W: 76, H: 26, Layer: "input"},
		{Name: "runMode", X: 952, Y: 278, W: 170, H: 26, Layer: "input"},
		{Name: "queue", X: 220, Y: 354, W: 170, H: 26, Layer: "input"},
		{Name: "pageSize", X: 410, Y: 354, W: 170, H: 26, Layer: "input"},
		{Name: "resolution", X: 600, Y: 354, W: 140, H: 26, Layer: "input"},
		{Name: "colorMode", X: 760, Y: 354, W: 140, H: 26, Layer: "input"},
		{Name: "mediaType", X: 220, Y: 420, W: 240, H: 26, Layer: "input"},
		{Name: "printSpeed", X: 480, Y: 420, W: 150, H: 26, Layer: "input"},
		{Name: "status", X: 220, Y: 560, W: 940, H: 22, Layer: "status"},
		{Name: "results", X: 220, Y: 612, W: 940, H: 86, Layer: "surface"},
		{Name: "log", X: 220, Y: 730, W: 940, H: 24, Layer: "surface"},
	}
}
