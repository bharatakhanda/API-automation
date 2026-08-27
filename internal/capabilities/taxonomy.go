package capabilities

type Group struct {
	Name         string
	Capabilities []string
}

var Taxonomy = []Group{
	{Name: "Environment", Capabilities: []string{"ServerVersion", "DeviceModel", "ServerStatus"}},
	{Name: "Queues", Capabilities: []string{"Hold", "Print", "Direct", "AvailableQueues", "DefaultQueue"}},
	{Name: "Print", Capabilities: []string{"EFBrightness", "EFResolution", "EFCopies", "EFRotation", "Scaling"}},
	{Name: "Rendering", Capabilities: []string{"EFFineLineRendering", "EFImageSmoothing", "EFEdgeEnhancement"}},
	{Name: "Color", Capabilities: []string{"EFColorMode", "EFColorantDepth", "OutputICCProfile", "RGBSourceProfile", "CMYKSourceProfile", "GrayProfile", "EnabledColorants", "InkDropSizes", "SpotColorCount"}},
	{Name: "Media", Capabilities: []string{"EFMediaType", "EFInputSlot", "EFOutputBin", "SubstrateWidth", "SubstrateHeight"}},
	{Name: "Layout", Capabilities: []string{"EFOrientation", "ImagePositionX", "ImagePositionY"}},
	{Name: "Job", Capabilities: []string{"HasTransparency"}},
}
