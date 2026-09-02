package capabilities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"api-automation/internal/fiery"
)

func TestFromSnapshotExtractsServerQueuesAndOptions(t *testing.T) {
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{
		{Name: "info", Body: raw(`{"data":{"item":{"name":"SERVER-85","serial_number":"P00014754","version":"1.4","timezone":"Pacific Standard Time (-0700)","locale":"English_United States.1252","uptime":12345,"disk_available":1000,"disk_total":2000,"memory_available":3000,"memory_total":4000}}}`)},
		{Name: "queues", Body: raw(`{"data":{"items":[{"id":1,"name":"hold","available":true,"editable":true},{"id":2,"name":"font","available":false,"editable":true}]}}`)},
		{Name: "jobs", Body: raw(`{"data":{"totalItems":2,"items":[{"id":"JOB-1","status":"done spooling"},{"id":"JOB-2","status":"ripping","state":"processing"}]}}`)},
		{Name: "properties", Body: raw(`{"data":{"items":[{"id":"EFResolution","group":"fpimage","ppdtype":"uimenu","value":"360x360dpi","values":["360x360dpi","360x720dpi"],"scopes":["ps","command","fpimage","uimenu"]},{"id":"EFColorMode","group":"fpcolorwise","ppdtype":"uimenu","value":"CMYK","values":["CMYK","CMYKPLUS"],"scopes":["ps","rerip","fpcolorwise","uimenu"]},{"id":"Ignored","value":"x","values":["x"],"scopes":["ps"]}]}}`)},
	}}

	model := FromSnapshot(snapshot)
	if model.ServerName != "SERVER-85" || model.SerialNumber != "P00014754" || model.Version != "1.4" || model.TimeZone != "Pacific Standard Time (-0700)" || model.Locale != "English_United States.1252" || model.UptimeSeconds != 12345 || model.DiskAvailable != 1000 || model.DiskTotal != 2000 || model.MemoryAvailable != 3000 || model.MemoryTotal != 4000 {
		t.Fatalf("unexpected server metadata: %#v", model)
	}
	if len(model.Queues) != 2 || model.Queues[0].Name != "hold" || !model.Queues[0].Available {
		t.Fatalf("unexpected queues: %#v", model.Queues)
	}
	if model.JobsTotal != 2 || model.ActiveJobs != 1 || model.ActiveJobID != "JOB-2" || model.ActiveJobStatus != "ripping" {
		t.Fatalf("unexpected captured job workload: %#v", model)
	}
	if option, ok := model.OptionByID("EFResolution"); !ok || len(option.Values) != 2 {
		t.Fatalf("missing resolution option: %#v", model.Options)
	}
	if _, ok := model.OptionByID("Ignored"); ok {
		t.Fatal("ungrouped schema metadata was exposed as a writable job property")
	}
	if len(model.ExcludedOptions) != 1 || model.ExcludedOptions[0].ID != "Ignored" {
		t.Fatalf("excluded options = %#v", model.ExcludedOptions)
	}
	copies, ok := model.OptionByID("num copies")
	if !ok || copies.Label != "Copies" || len(copies.Values) == 0 {
		t.Fatalf("expected synthetic copies option, got %#v", copies)
	}
}

func TestPressModelFallsBackToFactoryQueueName(t *testing.T) {
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{{
		Name: "v5_queues",
		Body: raw(`{"data":{"items":[{"id":1,"name":"NZ-1000 hold","available":true,"editable":true}]}}`),
	}}}
	model := FromSnapshot(snapshot)
	if model.PressModel != "NZ-1000" {
		t.Fatalf("press model = %q", model.PressModel)
	}
}

func TestFromSnapshotPopulatesReadOnlyServerPresets(t *testing.T) {
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{{
		Name: "v5_presets",
		Body: raw(`{"data":{"items":[{"id":"PRESET-1","name":"Production","attributes":{"EFColorMode":"CMYK"}}]}}`),
	}}}
	model := FromSnapshot(snapshot)
	if len(model.ServerPresets) != 1 || model.ServerPresets[0].ID != "PRESET-1" || model.ServerPresets[0].Attributes["EFColorMode"] != "CMYK" {
		t.Fatalf("server presets = %#v", model.ServerPresets)
	}
}

func TestFromSnapshotKeepsExistingInfoWhenLaterResponseIsPartial(t *testing.T) {
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{
		{Name: "v5_info", Body: raw(`{"data":{"item":{"name":"SERVER-85","serial_number":"P00014754","version":"1.4"}}}`)},
		{Name: "v4_info", Body: raw(`{"data":{"item":{"name":""}}}`)},
	}}
	model := FromSnapshot(snapshot)
	if model.ServerName != "SERVER-85" || model.SerialNumber != "P00014754" || model.Version != "1.4" {
		t.Fatalf("later partial info erased metadata: %#v", model)
	}
}

func TestParsePropertiesIgnoresNullValues(t *testing.T) {
	options := parseProperties(raw(`{"data":{"items":[{"id":"NullOption","value":null,"values":[null,"", "valid"]}]}}`))
	if len(options) != 1 || options[0].Value != "" || len(options[0].Values) != 1 || options[0].Values[0] != "valid" {
		t.Fatalf("unexpected null normalization: %#v", options)
	}
}

func TestParsePropertiesPreservesFieryOutputProfileWireBOM(t *testing.T) {
	options := parseProperties(raw("{\"data\":{\"items\":[{\"id\":\"EFOutProfile\",\"value\":\"DEFAULT_MEDIA\",\"values\":[\"\\ufeffProfile A\",\"DEFAULT_MEDIA\"]}]}}"))
	if len(options) != 1 || len(options[0].Values) != 2 || options[0].Values[0] != "\ufeffProfile A" {
		t.Fatalf("output-profile values = %#v", options)
	}
	if got := cleanPropertyValue("EFOutProfile", " \ufeffProfile A "); got != " \ufeffProfile A " {
		t.Fatalf("exact output-profile wire value = %q", got)
	}
	if got := cleanPropertyValue("OtherProperty", "\ufeffValue"); got != "Value" {
		t.Fatalf("unrelated property normalization = %q", got)
	}
}

func TestCapturedSnapshotProducesUIPopulatableModel(t *testing.T) {
	path := filepath.Join("..", "..", "server-capabilities-snapshot-20260827-174908.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("captured snapshot not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	var snapshot fiery.CapabilitySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	model := FromSnapshot(snapshot)
	if model.ServerName != "SERVER-85" || model.PressModel != "NZ-1000" {
		t.Fatalf("server identity = name %q press %q", model.ServerName, model.PressModel)
	}
	if len(model.Queues) == 0 {
		t.Fatal("expected queues")
	}
	for _, id := range []string{"PageSize", "EFResolution", "EFColorMode", "EFMediaType", "EFPrintSpeed", "num copies"} {
		option, ok := model.OptionByID(id)
		if !ok || len(option.Values) == 0 {
			t.Fatalf("expected option %s with values, got %#v", id, option)
		}
	}
	outputProfile, ok := model.OptionByID("EFOutProfile")
	if !ok || len(outputProfile.Values) < 2 || !strings.HasPrefix(outputProfile.Values[0], "\ufeff") {
		t.Fatalf("EFOutProfile did not preserve its exact advertised wire prefix: %#v", outputProfile.Values)
	}
	for _, id := range []string{
		"EFCustomerProofing", "EFCustomerProofingNumCopies", "EFPressProofing", "EFPressProofingNumCopies", "EFJobOrchPlugins", "EFScreenBitsPerPixel",
		"EFCourierSubst", "EFDropSizes", "EFInfiniteCopies", "EFIntentDuplex", "EFNozzleOutLUT", "EFSP_Content", "EFUserPageSize",
		"EFCompression", "EFCopierMode", "EFCurveAdjust", "EFOutProfileNonSpot", "EFSizeName", "EFSpotPriority", "InputSlot",
		"EFColorant1Area", "EFColorant1Enable", "EFFineLineRendering",
	} {
		if _, ok := model.OptionByID(id); ok {
			t.Fatalf("NZ-1000 inapplicable/context-only property %s was exposed", id)
		}
		if !excludedOptionExists(model.ExcludedOptions, id) {
			t.Fatalf("NZ-1000 exclusion reason missing for %s", id)
		}
	}
	for _, id := range []string{"EFHTGraphics", "EFHTImages", "EFHTText", "EFMarginZero", "EFPDFPreflightProfile", "EFUseAPPE", "EFUseSPDMediaMapping", "EFRaster"} {
		if _, ok := model.OptionByID(id); ok {
			t.Fatalf("backend-only undocumented property %s was exposed", id)
		}
		if !excludedOptionExists(model.ExcludedOptions, id) {
			t.Fatalf("backend-only taxonomy exclusion missing for %s", id)
		}
	}
	for _, id := range []string{"EFPostFlight", "EFProgressives", "EFCurveAdjSpotBypass", "EFTextGfxQual"} {
		option, ok := model.OptionByID(id)
		if !ok || len(option.Values) < 2 {
			t.Fatalf("%s was incorrectly removed by inverted Fiery constraint handling: %#v", id, option)
		}
	}
	if len(model.Options) != 78 || len(model.ExcludedOptions) != 187 {
		t.Fatalf("NZ-1000 eligibility totals = %d applicable / %d excluded, want 78 / 187", len(model.Options), len(model.ExcludedOptions))
	}
	if len(model.ExcludedValues) != 0 {
		t.Fatalf("installed-configuration value exclusions = %d, want 0: %#v", len(model.ExcludedValues), model.ExcludedValues)
	}
	if count := ConstraintCount(model); count != 24 {
		t.Fatalf("NZ-1000 constrained-property count = %d, want 24", count)
	}
	for _, option := range model.Options {
		if !option.Synthetic && !documentedCWSJobProperty(option.ID) {
			t.Fatalf("undocumented property %s survived into Job Properties", option.ID)
		}
	}
	audit := BuildFilterAudit(model)
	if audit.Summary.RawServerProperties != 263 || audit.Summary.IncludedProperties != 78 || audit.Summary.DisplayedProperties != 78 || len(audit.DisplayedOptions) != 78 {
		t.Fatalf("NZ-1000 filter audit = %#v displayed=%d", audit.Summary, len(audit.DisplayedOptions))
	}
}

func TestCapabilityEligibilityRequiresDocumentedTaxonomyAndServerMetadata(t *testing.T) {
	available := false
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{{
		Name: "v5_properties",
		Body: raw(`{"data":{"items":[
			{"id":"EFResolution","group":"fpimage","ppdtype":"uimenu","value":"A","values":["A","B"],"scopes":["ps","command","fpimage","uimenu"]},
			{"id":"UndocumentedBackendControl","group":"fpimage","ppdtype":"uimenu","value":"A","values":["A","B"],"scopes":["ps","command","summarypane","fpimage","uimenu"]},
			{"id":"GenericContextSetting","group":"fpjobinfo","ppdtype":"uimenu","value":"False","values":["False","True"],"scopes":["ps","appe","impose","childpriority","fpjobinfo","uimenu"]},
			{"id":"ContainerOnlySetting","group":"fpimage","ppdtype":"uimenu","value":"A","values":["A","B"],"scopes":["ps","appe","containeronly","fpimage","uimenu"]},
			{"id":"OneValueSetting","group":"fpimage","ppdtype":"uimenu","value":"A","values":["A"],"scopes":["ps","command","fpimage","uimenu"]},
			{"id":"EFColorant1Opt","group":"efppinstallableoptions","ppdtype":"uimenu","value":"False","values":["False","White"],"scopes":["ps","command","efppinstallableoptions","uimenu"]},
			{"id":"ColorantControl","group":"fpcolorant","ppdtype":"uimenu","value":"False","values":["False","True"],"scopes":["ps","command","fpcolorant","uimenu"]},
			{"id":"EFDuplexOpt","group":"efppinstallableoptions","ppdtype":"uimenu","value":"False","values":["False","True"],"scopes":["ps","command","efppinstallableoptions","uimenu"]},
			{"id":"EFIntentDuplex","group":"fplayout","ppdtype":"uimenu","value":"False","values":["False","True"],"scopes":["ps","command","fplayout","uimenu"]},
			{"id":"ExplicitlyUnavailable","group":"fpimage","ppdtype":"uimenu","available":false,"value":"A","values":["A","B"],"scopes":["ps","command","fpimage","uimenu"]},
			{"id":"ExplicitlyNonEditable","group":"fpimage","ppdtype":"uimenu","editable":false,"value":"A","values":["A","B"],"scopes":["ps","command","fpimage","uimenu"]},
			{"id":"HiddenInternalSetting","group":"fpimage","ppdtype":"uimenu","value":"A","values":["A","B"],"scopes":["ps","command","driveruserdisplayhidden","fpimage","uimenu"]},
			{"id":"InstallableSetting","group":"efppinstallableoptions","ppdtype":"uimenu","value":"True","values":["False","True"],"scopes":["ps","command","efppinstallableoptions","uimenu"]},
			{"id":"SchemaMetadata","value":"x","values":["x"],"scopes":["ps"]},
			{"id":"UnsupportedTextControl","group":"fpjobinfo","ppdtype":"efijobnote","editable":true,"value":"","values":[],"scopes":["ps","command","fpjobinfo","efijobnote"]}
		]}}`),
	}}}
	model := FromSnapshot(snapshot)
	if _, ok := model.OptionByID("EFResolution"); !ok {
		t.Fatal("documented, affirmatively applicable setting was excluded")
	}
	for _, id := range []string{"UndocumentedBackendControl", "GenericContextSetting", "ContainerOnlySetting", "OneValueSetting", "EFColorant1Opt", "ColorantControl", "EFDuplexOpt", "EFIntentDuplex", "ExplicitlyUnavailable", "ExplicitlyNonEditable", "HiddenInternalSetting", "InstallableSetting", "SchemaMetadata", "UnsupportedTextControl"} {
		if _, ok := model.OptionByID(id); ok {
			t.Fatalf("ineligible property %s was exposed", id)
		}
		if !excludedOptionExists(model.ExcludedOptions, id) {
			t.Fatalf("missing diagnostic exclusion for %s", id)
		}
	}
	taxonomyReason := ""
	for _, decision := range model.ExcludedOptions {
		if decision.ID == "UndocumentedBackendControl" {
			taxonomyReason = decision.Reason
			break
		}
	}
	if !strings.Contains(taxonomyReason, "documented CWS Job Properties taxonomy") {
		t.Fatalf("undocumented control exclusion reason = %q", taxonomyReason)
	}
	for _, option := range parseProperties(snapshot.Endpoints[0].Body) {
		if option.ID == "ExplicitlyUnavailable" {
			if option.Available == nil || *option.Available != available {
				t.Fatalf("explicit availability was not retained: %#v", option)
			}
		}
	}
}

func TestInstalledConfigurationPrunesIncompatiblePropertyValues(t *testing.T) {
	options := []Option{
		{ID: "FixedInstallOption", Label: "Fixed install option", Group: "efppinstallableoptions", Value: "B", Values: []string{"A", "B"}},
		{ID: "DependentFeature", Group: "fpimage", PPDType: "uimenu", Value: "Off", Values: []string{"Off", "On"}, Scopes: []string{"ps", "command", "fpimage", "uimenu"}, Constraints: Constraints{"On": {"FixedInstallOption": {"B"}}}},
	}
	filtered, excludedOptions, excludedValues := applyFixedConfigurationConstraints(options)
	if len(filtered) != 1 || filtered[0].ID != "FixedInstallOption" {
		t.Fatalf("configuration-filtered options = %#v", filtered)
	}
	if len(excludedOptions) != 1 || excludedOptions[0].ID != "DependentFeature" {
		t.Fatalf("configuration exclusions = %#v", excludedOptions)
	}
	if len(excludedValues) != 1 || excludedValues[0].OptionID != "DependentFeature" || excludedValues[0].Value != "On" {
		t.Fatalf("configuration value exclusions = %#v", excludedValues)
	}
}

func TestInstalledConfigurationKeepsValuesWhenPublishedConflictDoesNotMatch(t *testing.T) {
	options := []Option{
		{ID: "FixedInstallOption", Group: "efppinstallableoptions", Value: "B", Values: []string{"A", "B"}},
		{ID: "DependentFeature", Group: "fpimage", PPDType: "uimenu", Value: "Off", Values: []string{"Off", "On"}, Constraints: Constraints{"On": {"FixedInstallOption": {"A"}}}},
	}
	filtered, excludedOptions, excludedValues := applyFixedConfigurationConstraints(options)
	if len(filtered) != 2 || len(filtered[1].Values) != 2 || len(excludedOptions) != 0 || len(excludedValues) != 0 {
		t.Fatalf("non-conflicting installed configuration was pruned: filtered=%#v excluded=%#v values=%#v", filtered, excludedOptions, excludedValues)
	}
}

func TestCapabilityEligibilityKeepsHiddenDriverPropertyWithIndependentJobVisibility(t *testing.T) {
	options := []Option{{
		ID: "EFJobExpertRule", Group: "fpimage", PPDType: "uimenu", Values: []string{"False", "True"},
		Scopes: []string{"ps", "rerip", "summarypane", "driveruserdisplayhidden", "fpimage", "uimenu"},
	}}
	eligible, excluded := eligibleJobProperties(options)
	if len(eligible) != 1 || len(excluded) != 0 {
		t.Fatalf("eligible=%#v excluded=%#v", eligible, excluded)
	}
}

func TestCapabilityEligibilityCollapsesAliasesToStrongestVisibleProperty(t *testing.T) {
	options := []Option{
		{ID: "EFFineLineRendering", Group: "fpimage", PPDType: "uimenu", Values: []string{"False", "True"}, Scopes: []string{"ps", "rerip", "driveruserdisplayhidden", "fpimage", "uimenu"}},
		{ID: "EFTextGfxQual", Group: "fpimage", PPDType: "uimenu", Values: []string{"False", "True", "Best"}, Scopes: []string{"ps", "command", "summarypane", "fpimage", "uimenu"}},
	}
	eligible, excluded := eligibleJobProperties(options)
	if len(eligible) != 1 || eligible[0].ID != "EFTextGfxQual" {
		t.Fatalf("eligible aliases = %#v", eligible)
	}
	if len(excluded) != 1 || excluded[0].ID != "EFFineLineRendering" || !strings.Contains(excluded[0].Reason, "alias") {
		t.Fatalf("alias exclusions = %#v", excluded)
	}
}

func TestGroupedOptionsDoesNotDisplayOneAPIOptionTwice(t *testing.T) {
	model := Model{Options: []Option{{ID: "EFEdgeDropSize", Label: "Edge", Values: []string{"1"}}}}
	groups := GroupedOptions(model)
	count := 0
	for _, group := range groups {
		for _, option := range group.Options {
			if option.ID == "EFEdgeDropSize" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("EFEdgeDropSize displayed %d times, want 1", count)
	}
}

func TestParsePropertyMetadataRangeAndConstraints(t *testing.T) {
	options := parseProperties(raw(`{"data":{"items":[{"id":"EFMediaThickness","group":"fppapersource","ppdtype":"efirange","value":2,"value_attributes":{"editable":true,"increment":1,"min":1,"max":50,"precision":0},"constraints":{}},{"id":"EFResolution","group":"fpimage","ppdtype":"uimenu","value":"360x360dpi","values":["360x360dpi","360x720dpi"],"constraints":{"360x720dpi":{"EFEdgeDropSize":["0_1_2_2_2"]}}}]}}`))
	if len(options) != 2 {
		t.Fatalf("options = %#v", options)
	}
	rangeOption := options[0]
	if rangeOption.ID != "EFMediaThickness" || rangeOption.Group != "fppapersource" || rangeOption.Range == nil || rangeOption.Range.Min != 1 || rangeOption.Range.Max != 50 {
		t.Fatalf("range option = %#v", rangeOption)
	}
	resolution := options[1]
	if got := resolution.Constraints["360x720dpi"]["EFEdgeDropSize"]; len(got) != 1 || got[0] != "0_1_2_2_2" {
		t.Fatalf("constraints = %#v", resolution.Constraints)
	}
}

func TestCategorySearchAndExplicitConstraintValidation(t *testing.T) {
	model := Model{Options: []Option{
		{ID: "EFResolution", Label: "Resolution", Group: "fpimage", Values: []string{"360x360dpi", "360x720dpi"}, Constraints: Constraints{"360x720dpi": {"EFEdgeDropSize": {"0_1_2_2_2"}}}},
		{ID: "EFEdgeDropSize", Label: "Edge enhancement", Group: "fpimage", Values: []string{"None", "0_1_2_2_2"}},
		{ID: "EFMediaThickness", Label: "Media thickness", Group: "fppapersource", Range: &NumericRange{Min: 1, Max: 50, Increment: 1}},
	}}
	groups := FilteredGroups(model, "thickness")
	if len(groups) != 1 || groups[0].Name != "Substrate / Media" || len(groups[0].Options) != 1 {
		t.Fatalf("filtered groups = %#v", groups)
	}
	valid := map[string]string{"EFResolution": "360x720dpi", "EFEdgeDropSize": "None"}
	if conflicts := ValidateCombination(model, valid); len(conflicts) != 0 {
		t.Fatalf("valid conflicts = %#v", conflicts)
	}
	invalid := map[string]string{"EFResolution": "360x720dpi", "EFEdgeDropSize": "0_1_2_2_2"}
	if conflicts := ValidateCombination(model, invalid); len(conflicts) != 1 {
		t.Fatalf("invalid conflicts = %#v", conflicts)
	} else if conflicts[0].ConflictingValues[0] != "0_1_2_2_2" {
		t.Fatalf("conflicting values = %#v", conflicts[0])
	}
	if !NeedsConstraintCheck(model, map[string]string{"EFResolution": "360x720dpi"}) || NeedsConstraintCheck(model, map[string]string{"Unrelated": "x"}) {
		t.Fatal("constraint-check requirement detection is incorrect")
	}
}

func TestGroupedOptionsUsesPDFHeadingsNestedSectionsAndOrder(t *testing.T) {
	model := Model{Options: []Option{
		{ID: "EFResolution", Label: "Resolution", Group: "fpimage"},
		{ID: "EFEdgeDropSize", Label: "Edge", Group: "fpimage"},
		{ID: "EFColorMode", Label: "Color", Group: "fpcolorwise"},
		{ID: "EFRGBOverride", Label: "RGB", Group: "fpcolorwise"},
		{ID: "EFPrintDieLine", Label: "Die", Group: "fpjobinfo"},
		{ID: "Instruct", Label: "Instructions", Group: "fpjobinfo"},
	}}
	groups := GroupedOptions(model)
	if names := CategoryNames(model); len(names) != 3 || names[0] != "Job Info" || names[1] != "Color" || names[2] != "Image" {
		t.Fatalf("PDF category order = %v", names)
	}
	for _, name := range CategoryNames(model) {
		if name == "Quick Access" || name == "Color and Image" {
			t.Fatalf("obsolete category remained: %q", name)
		}
	}
	job := groups[0]
	if len(job.Sections) != 2 || job.Sections[0].Name != "Job notes" || job.Sections[1].Name != "Die printing" {
		t.Fatalf("job sections = %#v", job.Sections)
	}
	color := groups[1]
	if len(color.Sections) != 2 || color.Sections[0].Name != "" || color.Sections[1].Name != "Color input" {
		t.Fatalf("color sections = %#v", color.Sections)
	}
	image := groups[2]
	if len(image.Sections) != 2 || image.Sections[0].Name != "" || image.Sections[1].Name != "Edge enhancement" {
		t.Fatalf("image sections = %#v", image.Sections)
	}
}

func TestGroupedOptionsDoesNotDisplayCopiesAliasesTwice(t *testing.T) {
	model := Model{Options: []Option{
		{ID: "EFCopies", Values: []string{"1"}},
		{ID: "num copies", Values: []string{"1"}},
	}}
	count := 0
	for _, group := range GroupedOptions(model) {
		for _, option := range group.Options {
			if option.ID == "EFCopies" || option.ID == "num copies" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("copies options displayed %d times, want 1", count)
	}
}

func excludedOptionExists(options []ExcludedOption, id string) bool {
	for _, option := range options {
		if option.ID == id && option.Reason != "" {
			return true
		}
	}
	return false
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }
