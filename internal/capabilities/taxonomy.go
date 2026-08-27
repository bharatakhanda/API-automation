package capabilities

type Group struct {
	Name         string
	Capabilities []string
}

type CanonicalOption struct {
	CanonicalID string
	Label       string
	APIKeys     []string
}

type OptionGroup struct {
	Name    string
	Options []Option
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

var CanonicalTaxonomy = []struct {
	Name    string
	Options []CanonicalOption
}{
	{Name: "Print", Options: []CanonicalOption{
		{CanonicalID: "EFBrightness", Label: "Brightness", APIKeys: []string{"EFBrightness"}},
		{CanonicalID: "EFResolution", Label: "Resolution", APIKeys: []string{"EFResolution"}},
		{CanonicalID: "EFCopies", Label: "Copies", APIKeys: []string{"EFCopies", "num copies"}},
		{CanonicalID: "EFPrintCover", Label: "Cover page", APIKeys: []string{"EFPrintCover"}},
		{CanonicalID: "EFRotation", Label: "Rotation", APIKeys: []string{"EFRotateDocument", "EFRotation"}},
		{CanonicalID: "Scaling", Label: "Scaling", APIKeys: []string{"Scaling", "EFScale"}},
	}},
	{Name: "Rendering", Options: []CanonicalOption{
		{CanonicalID: "EFFineLineRendering", Label: "Fine line rendering", APIKeys: []string{"EFTextGfxQual", "EFFineLineRendering"}},
		{CanonicalID: "EFImageSmoothing", Label: "Image smoothing", APIKeys: []string{"EFImageSmooth", "EFImageSmoothing"}},
		{CanonicalID: "EFEdgeEnhancement", Label: "Edge enhancement", APIKeys: []string{"EFEdgeDropSize", "EFEdgeEnhancement"}},
	}},
	{Name: "Color", Options: []CanonicalOption{
		{CanonicalID: "EFColorMode", Label: "Color mode", APIKeys: []string{"EFColorMode"}},
		{CanonicalID: "EFColorantDepth", Label: "Colorant depth", APIKeys: []string{"EFColorantDepth"}},
		{CanonicalID: "OutputICCProfile", Label: "Output ICC profile", APIKeys: []string{"EFOutProfile", "OutputICCProfile"}},
		{CanonicalID: "RGBSourceProfile", Label: "RGB source profile", APIKeys: []string{"EFRGBOverride", "RGBSourceProfile"}},
		{CanonicalID: "CMYKSourceProfile", Label: "CMYK source profile", APIKeys: []string{"EFSimulation", "CMYKSourceProfile"}},
		{CanonicalID: "GrayProfile", Label: "Gray profile", APIKeys: []string{"EFGrayOverride", "GrayProfile"}},
		{CanonicalID: "EnabledColorants", Label: "Enabled colorants", APIKeys: []string{"EFProcessNColorants", "EFColorant1Enable", "EnabledColorants"}},
		{CanonicalID: "InkDropSizes", Label: "Ink/drop size", APIKeys: []string{"EFEdgeDropSize", "InkDropSizes"}},
		{CanonicalID: "SpotColorCount", Label: "Spot color matching", APIKeys: []string{"EFSpotColors", "SpotColorCount"}},
	}},
	{Name: "Media", Options: []CanonicalOption{
		{CanonicalID: "EFMediaType", Label: "Media type", APIKeys: []string{"EFMediaType"}},
		{CanonicalID: "EFInputSlot", Label: "Input slot", APIKeys: []string{"InputSlot", "EFInputSlot"}},
		{CanonicalID: "EFOutputBin", Label: "Output tray", APIKeys: []string{"EFOutputBin"}},
		{CanonicalID: "SubstrateWidth", Label: "Substrate width", APIKeys: []string{"SubstrateWidth", "EFMediaWidth"}},
		{CanonicalID: "SubstrateHeight", Label: "Substrate height", APIKeys: []string{"SubstrateHeight", "EFMediaLength"}},
	}},
	{Name: "Layout", Options: []CanonicalOption{
		{CanonicalID: "EFOrientation", Label: "Orientation", APIKeys: []string{"EFOrientation", "Orientation"}},
		{CanonicalID: "ImagePositionX", Label: "Image position X", APIKeys: []string{"EFImageFrontXOutput", "ImagePositionX"}},
		{CanonicalID: "ImagePositionY", Label: "Image position Y", APIKeys: []string{"EFImageFrontYOutput", "ImagePositionY"}},
	}},
	{Name: "Job", Options: []CanonicalOption{
		{CanonicalID: "HasTransparency", Label: "PDF transparency", APIKeys: []string{"EFPDF_PS_RGB_Transparency", "HasTransparency"}},
	}},
}

func GroupedOptions(model Model) []OptionGroup {
	groups := make([]OptionGroup, 0, len(CanonicalTaxonomy))
	used := map[string]struct{}{}
	for _, group := range CanonicalTaxonomy {
		out := OptionGroup{Name: group.Name}
		for _, canonical := range group.Options {
			if opt, ok := firstOptionByKeys(model, canonical.APIKeys); ok {
				opt.Label = canonical.Label
				out.Options = append(out.Options, opt)
				used[opt.ID] = struct{}{}
			}
		}
		if len(out.Options) > 0 {
			groups = append(groups, out)
		}
	}
	misc := OptionGroup{Name: "Other discovered options"}
	for _, opt := range model.Options {
		if _, ok := used[opt.ID]; !ok {
			misc.Options = append(misc.Options, opt)
		}
	}
	if len(misc.Options) > 0 {
		groups = append(groups, misc)
	}
	return groups
}

func firstOptionByKeys(model Model, keys []string) (Option, bool) {
	for _, key := range keys {
		if opt, ok := model.OptionByID(key); ok {
			return opt, true
		}
	}
	return Option{}, false
}
