package app

const (
	windowWidth  = 1240
	windowHeight = 900
)

type uiRect struct {
	Name       string
	X, Y, W, H int
	Layer      string
}

func settingsLayoutRects() []uiRect {
	return []uiRect{
		{Name: "settingsTitle", X: 220, Y: 80, W: 320, H: 18, Layer: "settings"},
		{Name: "serverIPLabel", X: 220, Y: 100, W: 140, H: 20, Layer: "settings"},
		{Name: "serverIP", X: 220, Y: 124, W: 220, H: 26, Layer: "settings"},
		{Name: "secretLabel", X: 456, Y: 100, W: 120, H: 20, Layer: "settings"},
		{Name: "secretKey", X: 456, Y: 124, W: 310, H: 26, Layer: "settings"},
		{Name: "passwordLabel", X: 782, Y: 100, W: 130, H: 20, Layer: "settings"},
		{Name: "password", X: 782, Y: 124, W: 160, H: 26, Layer: "settings"},
	}
}

func enterpriseLayoutRects() []uiRect {
	return []uiRect{
		{Name: "settingsButton", X: 620, Y: 24, W: 110, H: 30, Layer: "action"},
		{Name: "captureButton", X: 742, Y: 24, W: 240, H: 30, Layer: "action"},
		{Name: "runButton", X: 1010, Y: 24, W: 160, H: 30, Layer: "action"},
		{Name: "cancelButton", X: 1010, Y: 60, W: 160, H: 28, Layer: "action"},
		{Name: "folderPath", X: 220, Y: 126, W: 650, H: 26, Layer: "input"},
		{Name: "browseFolder", X: 884, Y: 125, W: 96, H: 28, Layer: "action"},
		{Name: "selectionMode", X: 220, Y: 184, W: 150, H: 26, Layer: "input"},
		{Name: "filePath", X: 396, Y: 184, W: 474, H: 26, Layer: "input"},
		{Name: "browseFile", X: 884, Y: 183, W: 96, H: 28, Layer: "action"},
		{Name: "method", X: 220, Y: 278, W: 92, H: 26, Layer: "input"},
		{Name: "url", X: 336, Y: 278, W: 510, H: 26, Layer: "input"},
		{Name: "concurrency", X: 860, Y: 278, W: 76, H: 26, Layer: "input"},
		{Name: "runMode", X: 952, Y: 278, W: 170, H: 26, Layer: "input"},
		{Name: "queue", X: 220, Y: 372, W: 190, H: 26, Layer: "input"},
		{Name: "pageSize", X: 430, Y: 372, W: 180, H: 26, Layer: "input"},
		{Name: "resolution", X: 630, Y: 372, W: 170, H: 26, Layer: "input"},
		{Name: "colorMode", X: 820, Y: 372, W: 170, H: 26, Layer: "input"},
		{Name: "mediaType", X: 220, Y: 446, W: 280, H: 26, Layer: "input"},
		{Name: "printSpeed", X: 520, Y: 446, W: 170, H: 26, Layer: "input"},
		{Name: "strategy", X: 720, Y: 446, W: 170, H: 26, Layer: "input"},
		{Name: "maxCases", X: 920, Y: 446, W: 90, H: 26, Layer: "input"},
		{Name: "status", X: 220, Y: 670, W: 980, H: 24, Layer: "status"},
		{Name: "results", X: 220, Y: 724, W: 980, H: 82, Layer: "surface"},
		{Name: "log", X: 220, Y: 838, W: 980, H: 48, Layer: "surface"},
	}
}
