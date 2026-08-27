package app

const (
	windowWidth  = 1280
	windowHeight = 1120
)

type uiRect struct {
	Name       string
	X, Y, W, H int
	Layer      string
}

func enterpriseLayoutRects() []uiRect {
	return []uiRect{
		{Name: "captureButton", X: 778, Y: 24, W: 170, H: 28, Layer: "action"},
		{Name: "runButton", X: 960, Y: 24, W: 132, H: 28, Layer: "action"},
		{Name: "cancelButton", X: 1104, Y: 24, W: 92, H: 28, Layer: "action"},
		{Name: "serverIP", X: 220, Y: 142, W: 250, H: 26, Layer: "input"},
		{Name: "secretKey", X: 492, Y: 142, W: 300, H: 26, Layer: "input"},
		{Name: "password", X: 814, Y: 142, W: 170, H: 26, Layer: "input"},
		{Name: "folderPath", X: 220, Y: 248, W: 650, H: 26, Layer: "input"},
		{Name: "browseFolder", X: 884, Y: 247, W: 96, H: 28, Layer: "action"},
		{Name: "selectionMode", X: 220, Y: 316, W: 150, H: 26, Layer: "input"},
		{Name: "filePath", X: 396, Y: 316, W: 474, H: 26, Layer: "input"},
		{Name: "browseFile", X: 884, Y: 315, W: 96, H: 28, Layer: "action"},
		{Name: "method", X: 220, Y: 432, W: 92, H: 26, Layer: "input"},
		{Name: "url", X: 336, Y: 432, W: 610, H: 26, Layer: "input"},
		{Name: "concurrency", X: 970, Y: 432, W: 76, H: 26, Layer: "input"},
		{Name: "runMode", X: 1064, Y: 432, W: 132, H: 26, Layer: "input"},
		{Name: "queue", X: 220, Y: 502, W: 190, H: 26, Layer: "input"},
		{Name: "pageSize", X: 432, Y: 502, W: 190, H: 26, Layer: "input"},
		{Name: "resolution", X: 644, Y: 502, W: 150, H: 26, Layer: "input"},
		{Name: "colorMode", X: 816, Y: 502, W: 150, H: 26, Layer: "input"},
		{Name: "mediaType", X: 220, Y: 570, W: 260, H: 26, Layer: "input"},
		{Name: "printSpeed", X: 502, Y: 570, W: 150, H: 26, Layer: "input"},
		{Name: "status", X: 220, Y: 768, W: 940, H: 22, Layer: "status"},
		{Name: "results", X: 220, Y: 832, W: 940, H: 184, Layer: "surface"},
		{Name: "log", X: 220, Y: 1060, W: 940, H: 48, Layer: "surface"},
	}
}
