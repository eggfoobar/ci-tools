package validation

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"k8s.io/utils/diff"
	"k8s.io/utils/ptr"

	"github.com/openshift/ci-tools/pkg/api"
	"github.com/openshift/ci-tools/pkg/testhelper"
)

func TestValidateBuildRoot(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		buildRootImageConfig *api.BuildRootImageConfiguration
		hasImages            bool
		ref                  string
		expectedValid        bool
	}{
		{
			name: "both project_image and image_stream_tag in build_root defined causes error",
			buildRootImageConfig: &api.BuildRootImageConfiguration{
				ImageStreamTagReference: &api.ImageStreamTagReference{
					Namespace: "test_namespace",
					Name:      "test_name",
					Tag:       "test",
				},
				ProjectImageBuild: &api.ProjectDirectoryImageBuildInputs{
					ContextDir:     "/",
					DockerfilePath: "Dockerfile.test",
				},
			},
			expectedValid: false,
		},
		{
			name: "Both project_image and from_repository causes error",
			buildRootImageConfig: &api.BuildRootImageConfiguration{
				ProjectImageBuild: &api.ProjectDirectoryImageBuildInputs{
					ContextDir:     "/",
					DockerfilePath: "Dockerfile.test",
				},
				FromRepository: true,
			},
			expectedValid: false,
		},
		{
			name: "Both image_stream_tag and from_repository causes error",
			buildRootImageConfig: &api.BuildRootImageConfiguration{
				ImageStreamTagReference: &api.ImageStreamTagReference{
					Namespace: "test_namespace",
					Name:      "test_name",
					Tag:       "test",
				},
				FromRepository: true,
			},
			expectedValid: false,
		},
		{
			name:                 "build root without any content causes an error",
			buildRootImageConfig: &api.BuildRootImageConfiguration{},
			expectedValid:        false,
		},
		{
			name:                 "nil build root is allowed when no images",
			buildRootImageConfig: nil,
			hasImages:            false,
			expectedValid:        true,
		},
		{
			name:                 "nil build root is not allowed when images defined",
			buildRootImageConfig: nil,
			hasImages:            true,
			expectedValid:        false,
		},
		{
			name: "image_stream_tag in build_root valid with ref",
			buildRootImageConfig: &api.BuildRootImageConfiguration{
				ImageStreamTagReference: &api.ImageStreamTagReference{
					Namespace: "test_namespace",
					Name:      "test_name",
					Tag:       "test",
				},
			},
			ref:           "org.repo",
			expectedValid: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateBuildRootImageConfiguration(NewConfigContext().AddField("build_root"), tc.buildRootImageConfig, tc.hasImages, tc.ref); (err != nil) && tc.expectedValid {
				t.Errorf("expected to be valid, got: %v", err)
			} else if !tc.expectedValid && err == nil {
				t.Error("expected to be invalid, but returned valid")
			}
		})
	}
}

func TestValidateExternalConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name           string
		externalConfig map[string]api.ExternalImage
		expectedValid  bool
	}{
		{
			name: "valid external configuration",
			externalConfig: map[string]api.ExternalImage{
				"foo":                {Registry: "quay.io", ImageStreamTagReference: api.ImageStreamTagReference{Namespace: "test", Name: "test", Tag: "test"}, PullSecret: "test-pull"},
				"bar":                {Registry: "quay.io/my", ImageStreamTagReference: api.ImageStreamTagReference{Namespace: "bar", Name: "test", Tag: "latest"}, PullSecret: "test-pull-2"},
				"without-pullsecret": {Registry: "quay.io/openshift/ci", ImageStreamTagReference: api.ImageStreamTagReference{Namespace: "bar", Name: "test", Tag: "latest"}},
			},
			expectedValid: true,
		},
		{
			name: "invalid external configuration, registry missing",
			externalConfig: map[string]api.ExternalImage{
				"foo": {ImageStreamTagReference: api.ImageStreamTagReference{Namespace: "test", Name: "test", Tag: "test"}, PullSecret: "test-pull"},
			},
			expectedValid: false,
		},
		{
			name: "invalid external configuration, namespace missing",
			externalConfig: map[string]api.ExternalImage{
				"foo": {Registry: "quay.io", ImageStreamTagReference: api.ImageStreamTagReference{Name: "test", Tag: "test"}},
			},
			expectedValid: false,
		},
		{
			name: "invalid external configuration, names missing",
			externalConfig: map[string]api.ExternalImage{
				"foo": {Registry: "quay.io", ImageStreamTagReference: api.ImageStreamTagReference{Namespace: "test", Tag: "test"}},
			},
			expectedValid: false,
		},
		{
			name: "invalid external configuration, tag missing",
			externalConfig: map[string]api.ExternalImage{
				"foo": {Registry: "quay.io", ImageStreamTagReference: api.ImageStreamTagReference{Namespace: "test", Name: "test"}},
			},
			expectedValid: false,
		},
		{
			name: "invalid external configuration, pull_spec set",
			externalConfig: map[string]api.ExternalImage{
				"foo": {PullSpec: "quay.io/some-pull-spec/name:tag"},
			},
			expectedValid: false,
		},
		{
			name:           "nil external configuration",
			externalConfig: nil,
			expectedValid:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateExternalConfiguration(NewConfigContext().AddField("external"), tc.externalConfig); (err != nil) && tc.expectedValid {
				t.Errorf("expected to be valid, got: %v", err)
			} else if !tc.expectedValid && err == nil {
				t.Error("expected to be invalid, but returned valid")
			}
		})
	}
}

func TestValidateImageStreamTagReferenceMap(t *testing.T) {
	for _, tc := range []struct {
		id            string
		baseImages    map[string]api.ImageStreamTagReference
		expectedValid bool
	}{
		{
			id: "valid",
			baseImages: map[string]api.ImageStreamTagReference{
				"test": {Tag: "test"}, "test2": {Tag: "test2"},
			},
			expectedValid: true,
		},
		{
			id: "missing tag",
			baseImages: map[string]api.ImageStreamTagReference{
				"test": {Tag: "test"}, "test2": {},
			},
			expectedValid: false,
		},
		{
			id: "cannot be bundle source",
			baseImages: map[string]api.ImageStreamTagReference{
				string(api.PipelineImageStreamTagReferenceBundleSource): {Tag: "bundle-src"},
			},
			expectedValid: false,
		},
	} {
		t.Run(tc.id, func(t *testing.T) {
			if errs := validateImageStreamTagReferenceMap("base_images", tc.baseImages); len(errs) > 0 && tc.expectedValid {
				t.Errorf("expected to be valid, got: %v", errs)
			} else if !tc.expectedValid && len(errs) == 0 {
				t.Error("expected to be invalid, but returned valid")
			}
		})
	}
}

func TestValidateResources(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		input       api.ResourceConfiguration
		expectedErr bool
	}{
		{
			name: "valid configuration makes no error",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Limits: api.ResourceList{
						"cpu": "100m",
					},
					Requests: api.ResourceList{
						"cpu": "100m",
					},
				},
			},
			expectedErr: false,
		},
		{
			name:        "configuration without any entry fails",
			input:       api.ResourceConfiguration{},
			expectedErr: true,
		},
		{
			name: "configuration without a blanket entry fails",
			input: api.ResourceConfiguration{
				"something": api.ResourceRequirements{
					Limits: api.ResourceList{
						"cpu": "100m",
					},
					Requests: api.ResourceList{
						"cpu": "100m",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "invalid key makes an error",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Limits: api.ResourceList{
						"cpu":    "100m",
						"boogie": "value",
					},
					Requests: api.ResourceList{
						"cpu": "100m",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "not having either cpu or memory makes an error",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Limits: api.ResourceList{
						"boogie": "100m",
					},
					Requests: api.ResourceList{
						"cpu": "100m",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "invalid value makes an error",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Limits: api.ResourceList{
						"cpu": "donkeys",
					},
					Requests: api.ResourceList{
						"cpu": "100m",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "negative value makes an error",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Limits: api.ResourceList{
						"cpu": "-110m",
					},
					Requests: api.ResourceList{
						"cpu": "100m",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "zero value makes an error",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Limits: api.ResourceList{
						"cpu": "0m",
					},
					Requests: api.ResourceList{
						"cpu": "100m",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "valid ephemeral-storage value passes",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Requests: api.ResourceList{
						"ephemeral-storage": "60Gi",
					},
					Limits: api.ResourceList{
						"ephemeral-storage": "100Gi",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "valid shm value passes",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Requests: api.ResourceList{
						api.ShmResource: "2G",
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "too large of shm value makes an error",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Requests: api.ResourceList{
						api.ShmResource: "3G",
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "valid nvidia gpu value passes",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Requests: api.ResourceList{
						api.NvidiaGPUResource: "1",
					},
					Limits: api.ResourceList{
						api.NvidiaGPUResource: "1",
					},
				},
			},
		},
		{
			name: "invalid nvidia gpu value passes",
			input: api.ResourceConfiguration{
				"*": api.ResourceRequirements{
					Requests: api.ResourceList{
						api.NvidiaGPUResource: "foo",
					},
					Limits: api.ResourceList{
						api.NvidiaGPUResource: "bar",
					},
				},
			},
			expectedErr: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateResources("", testCase.input)
			if err == nil && testCase.expectedErr {
				t.Errorf("%s: expected an error, but got none", testCase.name)
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("%s: expected no error, but got one: %v", testCase.name, err)
			}
		})
	}
}

func TestValidatePromotion(t *testing.T) {
	var testCases = []struct {
		name                    string
		input                   api.PromotionConfiguration
		promotesOfficialImages  bool
		imageTargets            bool
		releaseTagConfiguration *api.ReleaseTagConfiguration
		releases                map[string]api.UnresolvedRelease
		expected                []error
	}{
		{
			name:         "normal config by name is valid",
			input:        api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Name: "bar"}}},
			imageTargets: true,
			expected:     nil,
		},
		{
			name:         "normal config by tag is valid",
			input:        api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Tag: "bar"}}},
			imageTargets: true,
			expected:     nil,
		},
		{
			name:         "config missing fields yields errors",
			input:        api.PromotionConfiguration{Targets: []api.PromotionTarget{{}}},
			imageTargets: true,
			expected:     []error{errors.New("promotion.to[0]: no namespace defined"), errors.New("promotion.to[0]: no name or tag defined")},
		},
		{
			name:         "config with extra fields yields errors",
			input:        api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Name: "bar", Tag: "baz"}}},
			imageTargets: true,
			expected:     []error{errors.New("promotion.to[0]: both name and tag defined")},
		},
		{
			name:         "cannot promote to namespace openshift-some",
			input:        api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "openshift-some", Tag: "bar"}}},
			imageTargets: true,
			expected:     []error{errors.New("promotion.to[0]: cannot promote to namespace openshift-some matching this regular expression: (^kube.*|^openshift.*|^default$|^redhat.*)")},
		},
		{
			name:         "cannot have overlapping targets by tag",
			input:        api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Tag: "bar"}, {Namespace: "foo", Tag: "bar"}}},
			imageTargets: true,
			expected:     []error{errors.New("promotion.to[0]: promotes to the same target as promotion.to[1]"), errors.New("promotion.to[1]: promotes to the same target as promotion.to[0]")},
		},
		{
			name:         "cannot have overlapping targets by name",
			input:        api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Name: "bar"}, {Namespace: "foo", Name: "bar"}}},
			imageTargets: true,
			expected:     []error{errors.New("promotion.to[0]: promotes to the same target as promotion.to[1]"), errors.New("promotion.to[1]: promotes to the same target as promotion.to[0]")},
		},
		{
			name:                   "[release:latest] is not fulfilled",
			input:                  api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Tag: "bar"}}},
			promotesOfficialImages: true,
			imageTargets:           true,
			expected:               []error{fmt.Errorf("importing the release stream is required to ensure the promoted images to the namespace foo can be integrated properly. Although it can be achieved by tag_specification or releases[\"latest\"], adding an e2e test is strongly suggested")},
		},
		{
			name:                   "[release:latest] is not fulfilled because the release name is not correct",
			input:                  api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Tag: "bar"}}},
			promotesOfficialImages: true,
			imageTargets:           true,
			releases: map[string]api.UnresolvedRelease{
				"initial": {},
			},
			expected: []error{fmt.Errorf("importing the release stream is required to ensure the promoted images to the namespace foo can be integrated properly. Although it can be achieved by tag_specification or releases[\"latest\"], adding an e2e test is strongly suggested")},
		},
		{
			name:  "[release:latest] is fulfilled by release[latest]",
			input: api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Tag: "bar"}}},
			releases: map[string]api.UnresolvedRelease{
				"latest": {},
			},
			promotesOfficialImages: true,
			imageTargets:           true,
		},
		{
			name:                    "[release:latest] is fulfilled by tag_specification",
			input:                   api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Tag: "bar"}}},
			releaseTagConfiguration: &api.ReleaseTagConfiguration{},
			promotesOfficialImages:  true,
			imageTargets:            true,
		},
		{
			name:                   "[release:latest] is not fulfilled but there are no image targets",
			input:                  api.PromotionConfiguration{Targets: []api.PromotionTarget{{Namespace: "foo", Tag: "bar"}}},
			promotesOfficialImages: true,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			actual, expected := validatePromotionConfiguration("promotion", test.input, test.promotesOfficialImages, test.imageTargets, test.releaseTagConfiguration, test.releases), test.expected
			if diff := cmp.Diff(actual, expected, testhelper.EquateErrorMessage); diff != "" {
				t.Errorf("%s: got incorrect errors: %v", test.name, diff)
			}
		})
	}
}

func TestValidateReleaseTagConfiguration(t *testing.T) {
	var testCases = []struct {
		name     string
		input    api.ReleaseTagConfiguration
		expected []error
	}{
		{
			name:     "valid tag_specification",
			input:    api.ReleaseTagConfiguration{Name: "test", Namespace: "test"},
			expected: nil,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := validateReleaseTagConfiguration("tag_specification", testCase.input), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect errors: %v", testCase.name, diff.ObjectDiff(actual, expected))
			}
		})
	}
}

func TestValidateImages(t *testing.T) {
	var testCases = []struct {
		name   string
		input  []api.ProjectDirectoryImageBuildStepConfiguration
		output []error
	}{{
		name:  "`to` must be set",
		input: []api.ProjectDirectoryImageBuildStepConfiguration{{}},
		output: []error{
			errors.New("images[0]: `to` must be set"),
		},
	},
		{
			name: "two items cannot have identical `to`",
			input: []api.ProjectDirectoryImageBuildStepConfiguration{
				{To: "same-thing"},
				{To: "same-thing"},
			},
			output: []error{
				errors.New("images[1]: duplicate image name 'same-thing' (previously defined by field 'images[0]')"),
			},
		},
		{
			name: "Dockerfile literal is mutually exclusive with context_dir",
			input: []api.ProjectDirectoryImageBuildStepConfiguration{{
				ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
					DockerfileLiteral: ptr.To("FROM foo"),
					ContextDir:        "foo",
				},
				To: "amsterdam",
			}},
			output: []error{
				errors.New("images[0]: dockerfile_literal is mutually exclusive with context_dir and dockerfile_path"),
			},
		},
		{
			name: "Dockerfile literal is mutually exclusive with dockerfile_path",
			input: []api.ProjectDirectoryImageBuildStepConfiguration{{
				ProjectDirectoryImageBuildInputs: api.ProjectDirectoryImageBuildInputs{
					DockerfileLiteral: ptr.To("FROM foo"),
					DockerfilePath:    "foo",
				},
				To: "amsterdam",
			}},
			output: []error{
				errors.New("images[0]: dockerfile_literal is mutually exclusive with context_dir and dockerfile_path"),
			},
		},
		{
			name: "non valid image architecture",
			input: []api.ProjectDirectoryImageBuildStepConfiguration{{
				AdditionalArchitectures: []string{"foo", "amd64"},
				To:                      "amsterdam",
			}},
			output: []error{
				errors.New("images[0]: invalid architecture: foo. Use one of amd64, arm64, ppc64le, s390x"),
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			config := &api.ReleaseBuildConfiguration{
				Images: testCase.input,
			}
			if actual, expected := ValidateImages(NewConfigContext().AddField("images"), config.Images), testCase.output; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect errors: %s", testCase.name, cmp.Diff(actual, expected, cmp.Comparer(func(x, y error) bool {
					return x.Error() == y.Error()
				})))
			}
		})
	}
}

func TestValidateOperator(t *testing.T) {
	var goodStepLink = api.AllStepsLink()
	var badStepLink api.StepLink
	var testCases = []struct {
		name           string
		input          *api.OperatorStepConfiguration
		withResolvesTo api.StepLink
		output         []error
	}{
		{
			name: "everything is good",
			input: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{
					As:             "my-bundle",
					DockerfilePath: "./dockerfile",
					ContextDir:     ".",
					BaseIndex:      "an-index",
					UpdateGraph:    "replaces",
				}},
				Substitutions: []api.PullSpecSubstitution{
					{
						PullSpec: "original",
						With:     "substitute",
					},
				},
			},
			withResolvesTo: goodStepLink,
		},
		{
			name: "missing a substitution.pullspec and a substitution.with",
			input: &api.OperatorStepConfiguration{
				Substitutions: []api.PullSpecSubstitution{{
					PullSpec: "original",
					With:     "substitute",
				}, {
					PullSpec: "original2",
				}, {
					With: "substitute2",
				}},
			},
			withResolvesTo: goodStepLink,
			output: []error{
				errors.New("operator.substitute[1].with: must be set"),
				errors.New("operator.substitute[2].pullspec: must be set"),
			},
		},
		{
			name: "bad step link",
			input: &api.OperatorStepConfiguration{
				Substitutions: []api.PullSpecSubstitution{
					{
						PullSpec: "original",
						With:     "substitute",
					},
				},
			},
			withResolvesTo: badStepLink,
			output: []error{
				errors.New("operator.substitute[0].with: could not resolve 'substitute' to an image involved in the config"),
			},
		},
		{
			name: "bundle set without conflict",
			input: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{
					As: "no conflict",
				}},
			},
			withResolvesTo: goodStepLink,
		},
		{
			name: "bundle set with update_graph but not base_index set",
			input: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{
					As:          "valid bundle",
					UpdateGraph: "replaces",
				}},
			},
			withResolvesTo: goodStepLink,
			output: []error{
				errors.New("operator.bundles[0].update_graph: update_graph requires base_index to be set"),
			},
		},
		{
			name: "bundle set with base_index but not as set",
			input: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{
					BaseIndex: "an-index",
				}},
			},
			withResolvesTo: goodStepLink,
			output: []error{
				errors.New("operator.bundles[0].base_index: base_index requires 'as' to be set"),
			},
		},
		{
			name: "invalid update_graph",
			input: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{
					As:          "valid bundle",
					BaseIndex:   "an-index",
					UpdateGraph: "hello",
				}},
			},
			withResolvesTo: goodStepLink,
			output: []error{
				errors.New("operator.bundles[0].update_graph: update_graph must be semver, semver-skippatch, or replaces"),
			},
		},
		{
			name: "SkipBuildingIndex can be set",
			input: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{
					As:                "my-bundle",
					DockerfilePath:    "./dockerfile",
					ContextDir:        ".",
					BaseIndex:         "an-index",
					UpdateGraph:       "replaces",
					SkipBuildingIndex: true,
				}},
				Substitutions: []api.PullSpecSubstitution{
					{
						PullSpec: "original",
						With:     "substitute",
					},
				},
			},
			withResolvesTo: goodStepLink,
		},
		{
			name: "SkipBuildingIndex cannot be set on an unnamed bundle",
			input: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{
					DockerfilePath:    "./dockerfile",
					ContextDir:        ".",
					SkipBuildingIndex: true,
				}},
				Substitutions: []api.PullSpecSubstitution{
					{
						PullSpec: "original",
						With:     "substitute",
					},
				},
			},
			withResolvesTo: goodStepLink,
			output: []error{
				errors.New("operator.bundles[0].skip_building_index: skip_building_index requires 'as' to be set"),
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			linkFunc := func(string) api.StepLink {
				return testCase.withResolvesTo
			}
			if actual, expected := validateOperator(NewConfigContext().AddField("operator"), testCase.input, linkFunc), testCase.output; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect errors: %s", testCase.name, cmp.Diff(actual, expected, cmp.Comparer(func(x, y error) bool {
					return x.Error() == y.Error()
				})))
			}
		})
	}
}

func errListMessagesEqual(a, b []error) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if (a[idx] == nil) != (b[idx] == nil) {
			return false
		}
		if a[idx].Error() != b[idx].Error() {
			return false
		}
	}
	return true
}

func TestReleaseBuildConfiguration_validateImages(t *testing.T) {
	root := api.BuildRootImageConfiguration{FromRepository: true}
	input := api.InputConfiguration{BuildRootImage: &root}
	resources := api.ResourceConfiguration{
		"*": api.ResourceRequirements{
			Requests: api.ResourceList{"cpu": "1"},
		},
	}
	for _, tc := range []struct {
		name         string
		config       api.ReleaseBuildConfiguration
		mergedConfig bool
		expected     error
	}{{
		name: "valid",
		config: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Images: []api.ProjectDirectoryImageBuildStepConfiguration{
				{To: "image"},
			},
			Tests: []api.TestStepConfiguration{{
				As:       "test",
				Commands: "commands",
				ContainerTestConfiguration: &api.ContainerTestConfiguration{
					From: "from",
				},
			}},
			Resources: resources,
		},
	}, {
		name: "image and test cannot have the same name",
		config: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Images: []api.ProjectDirectoryImageBuildStepConfiguration{
				{To: "duplicated"},
			},
			Tests: []api.TestStepConfiguration{{
				As:       "duplicated",
				Commands: "commands",
				ContainerTestConfiguration: &api.ContainerTestConfiguration{
					From: "from",
				},
			}},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: tests[0].as: duplicated name "duplicated" already declared in 'images'`),
	}, {
		name: "valid mergedConfig",
		config: api.ReleaseBuildConfiguration{
			InputConfiguration: api.InputConfiguration{BuildRootImages: map[string]api.BuildRootImageConfiguration{
				"":         root,
				"org.repo": root,
			},
			},
			Images: []api.ProjectDirectoryImageBuildStepConfiguration{
				{To: "image"},
			},
			Tests: []api.TestStepConfiguration{{
				As:       "test",
				Commands: "commands",
				ContainerTestConfiguration: &api.ContainerTestConfiguration{
					From: "from",
				},
			}},
			Resources: resources,
		},
		mergedConfig: true,
	}, {
		name: "non-mergeConfig uses merged fields invalid",
		config: api.ReleaseBuildConfiguration{
			InputConfiguration: api.InputConfiguration{BuildRootImages: map[string]api.BuildRootImageConfiguration{
				"":         root,
				"org.repo": root,
			},
			},
			Images: []api.ProjectDirectoryImageBuildStepConfiguration{
				{To: "image"},
			},
			Tests: []api.TestStepConfiguration{{
				As:       "test",
				Commands: "commands",
				ContainerTestConfiguration: &api.ContainerTestConfiguration{
					From: "from",
				},
			}},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: it is not permissible to directly set: ‘build_roots’ directly in the config`),
	}} {
		t.Run(tc.name, func(t *testing.T) {
			err := IsValidResolvedConfiguration(&tc.config, tc.mergedConfig)
			testhelper.Diff(t, "error", err, tc.expected, testhelper.EquateErrorMessage)
		})
	}
}

func TestReleaseBuildConfiguration_validateTestStepDependencies(t *testing.T) {
	var testCases = []struct {
		name     string
		config   api.ReleaseBuildConfiguration
		expected []error
	}{
		{
			name: "no tests",
		},
		{
			name: "valid dependencies",
			config: api.ReleaseBuildConfiguration{
				InputConfiguration: api.InputConfiguration{
					// tag_spec provides stable, initial
					ReleaseTagConfiguration: &api.ReleaseTagConfiguration{Namespace: "ocp", Name: "4.5"},
					// releases provides custom
					Releases: map[string]api.UnresolvedRelease{
						"custom": {Release: &api.Release{Version: "4.7", Channel: api.ReleaseChannelStable}},
					},
				},
				BinaryBuildCommands: "whoa",
				Images:              []api.ProjectDirectoryImageBuildStepConfiguration{{To: "image"}},
				Operator: &api.OperatorStepConfiguration{
					Bundles: []api.Bundle{{
						DockerfilePath: "bundle.Dockerfile",
						ContextDir:     "manifests",
					}, {
						As:             "my-bundle",
						DockerfilePath: "bundle.Dockerfile",
						ContextDir:     "manifests",
					}},
				},
				Tests: []api.TestStepConfiguration{
					{MultiStageTestConfiguration: &api.MultiStageTestConfiguration{
						Pre: []api.TestStep{
							{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "src"}, {Name: "bin"}, {Name: "installer"}, {Name: "pipeline:ci-index"}}}},
							{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "pipeline:my-bundle"}}}},
							{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "stable:installer"}, {Name: "stable-initial:installer"}}}},
						},
						Test: []api.TestStep{{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "pipeline:bin"}}}}},
						Post: []api.TestStep{{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "image"}}}}},
					}},
					{MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
						Pre:  []api.LiteralTestStep{{Dependencies: []api.StepDependency{{Name: "stable-custom:cli"}, {Name: "ci-index-my-bundle"}}}},
						Test: []api.LiteralTestStep{{Dependencies: []api.StepDependency{{Name: "release:custom"}, {Name: "release:initial"}}}},
						Post: []api.LiteralTestStep{{Dependencies: []api.StepDependency{{Name: "pipeline:image"}}}},
					}},
				},
			},
		},
		{
			name: "overridden dependencies",
			config: api.ReleaseBuildConfiguration{
				Tests: []api.TestStepConfiguration{
					{MultiStageTestConfiguration: &api.MultiStageTestConfiguration{
						DependencyOverrides: map[string]string{
							"OH_SNAP": "nice",
						},
						Test: []api.TestStep{{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "pipeline:bin", Env: "OH_SNAP"}}}}},
					}},
					{MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
						DependencyOverrides: map[string]string{
							"OO_INDEX":   "coolstuff",
							"SOME_THING": "awwwyeah",
						},
						Test: []api.LiteralTestStep{{Dependencies: []api.StepDependency{{Name: "ci-index-my-bundle", Env: "OO_INDEX"}, {Name: string(api.PipelineImageStreamTagReferenceRPMs), Env: "SOME_THING"}}}},
					}},
				},
			},
		},
		{
			name: "invalid dependencies",
			config: api.ReleaseBuildConfiguration{
				Tests: []api.TestStepConfiguration{
					{MultiStageTestConfiguration: &api.MultiStageTestConfiguration{
						Pre: []api.TestStep{
							{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "stable:installer"}, {Name: "stable:grafana"}}}},
							{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "stable-custom:cli"}, {Name: "totally-invalid:cli"}}}},
						},
						Test: []api.TestStep{
							{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "pipeline:bin"}}}},
							{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "pipeline:test-bin"}}}},
						},
						Post: []api.TestStep{{LiteralTestStep: &api.LiteralTestStep{Dependencies: []api.StepDependency{{Name: "pipeline:image"}}}}},
					}},
					{MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
						Pre: []api.LiteralTestStep{
							{Dependencies: []api.StepDependency{{Name: "release:custom"}, {Name: "pipeline:ci-index"}}},
							{Dependencies: []api.StepDependency{{Name: "pipeline:ci-index-my-bundle"}}}},
						Test: []api.LiteralTestStep{{Dependencies: []api.StepDependency{{Name: "pipeline:root"}}}},
						Post: []api.LiteralTestStep{{Dependencies: []api.StepDependency{{Name: "pipeline:rpms"}}}},
					}},
				},
			},
			expected: []error{
				errors.New(`tests[0].steps.pre[0].dependencies[0]: cannot determine source for dependency "stable:installer" - this dependency requires a "latest" release, which is not configured`),
				errors.New(`tests[0].steps.pre[0].dependencies[1]: cannot determine source for dependency "stable:grafana" - this dependency requires a "latest" release, which is not configured`),
				errors.New(`tests[0].steps.pre[1].dependencies[0]: cannot determine source for dependency "stable-custom:cli" - this dependency requires a "custom" release, which is not configured`),
				errors.New(`tests[0].steps.pre[1].dependencies[1]: cannot determine source for dependency "totally-invalid:cli" - ensure the correct ImageStream name was provided`),
				errors.New(`tests[0].steps.test[0].dependencies[0]: cannot determine source for dependency "pipeline:bin" - this dependency requires built binaries, which are not configured`),
				errors.New(`tests[0].steps.test[1].dependencies[0]: cannot determine source for dependency "pipeline:test-bin" - this dependency requires built test binaries, which are not configured`),
				errors.New(`tests[0].steps.post[0].dependencies[0]: cannot determine source for dependency "pipeline:image" - no base image import, project image build, or bundle image build is configured to provide this dependency`),
				errors.New(`tests[1].literal_steps.pre[0].dependencies[0]: cannot determine source for dependency "release:custom" - this dependency requires a "custom" release, which is not configured`),
				errors.New(`tests[1].literal_steps.pre[0].dependencies[1]: cannot determine source for dependency "pipeline:ci-index" - this dependency requires an operator bundle configuration, which is not configured`),
				errors.New(`tests[1].literal_steps.pre[1].dependencies[0]: cannot determine source for dependency "pipeline:ci-index-my-bundle" - this dependency requires an operator bundle configuration, which is not configured`),
				errors.New(`tests[1].literal_steps.test[0].dependencies[0]: cannot determine source for dependency "pipeline:root" - this dependency requires a build root, which is not configured`),
				errors.New(`tests[1].literal_steps.post[0].dependencies[0]: cannot determine source for dependency "pipeline:rpms" - this dependency requires built RPMs, which are not configured`),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if actual, expected := validateTestStepDependencies(&testCase.config), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: got incorrect errors: %s", testCase.name, cmp.Diff(actual, expected, cmp.Comparer(func(x, y error) bool {
					return x.Error() == y.Error()
				})))
			}
		})
	}
}

func TestReleaseBuildConfiguration_ImageStreamFor(t *testing.T) {
	var testCases = []struct {
		name     string
		config   *api.ReleaseBuildConfiguration
		image    string
		expected string
		explicit bool
	}{
		{
			name: "explicit, is a base image",
			config: &api.ReleaseBuildConfiguration{InputConfiguration: api.InputConfiguration{
				BaseImages: map[string]api.ImageStreamTagReference{"thebase": {}},
			}},
			image:    "thebase",
			expected: api.PipelineImageStream,
			explicit: true,
		},
		{
			name: "explicit, is an RPM base image",
			config: &api.ReleaseBuildConfiguration{InputConfiguration: api.InputConfiguration{
				BaseRPMImages: map[string]api.ImageStreamTagReference{"thebase": {}},
			}},
			image:    "thebase",
			expected: api.PipelineImageStream,
			explicit: true,
		},
		{
			name:     "explicit, is a known pipeline image",
			config:   &api.ReleaseBuildConfiguration{},
			image:    "src",
			expected: api.PipelineImageStream,
			explicit: true,
		},
		{
			name:     "explicit, is a known built image",
			config:   &api.ReleaseBuildConfiguration{Images: []api.ProjectDirectoryImageBuildStepConfiguration{{To: "myimage"}}},
			image:    "myimage",
			expected: api.PipelineImageStream,
			explicit: true,
		},
		{
			name:     "implicit, is random",
			config:   &api.ReleaseBuildConfiguration{},
			image:    "something",
			expected: api.StableImageStream,
			explicit: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, explicit := testCase.config.ImageStreamFor(testCase.image)
			if explicit != testCase.explicit {
				t.Errorf("%s: did not correctly determine if ImageStream was explicit (should be %v)", testCase.name, testCase.explicit)
			}
			if actual != testCase.expected {
				t.Errorf("%s: did not correctly determine ImageStream wanted %s, got %s", testCase.name, testCase.expected, actual)
			}
		})
	}
}

func TestReleaseBuildConfiguration_DependencyParts(t *testing.T) {
	var testCases = []struct {
		name           string
		config         *api.ReleaseBuildConfiguration
		claimRelease   *api.ClaimRelease
		dependency     api.StepDependency
		expectedStream string
		expectedTag    string
		explicit       bool
	}{
		{
			name: "explicit, short-hand for base image",
			config: &api.ReleaseBuildConfiguration{InputConfiguration: api.InputConfiguration{
				BaseImages: map[string]api.ImageStreamTagReference{"thebase": {}},
			}},
			dependency:     api.StepDependency{Name: "thebase"},
			expectedStream: api.PipelineImageStream,
			expectedTag:    "thebase",
			explicit:       true,
		},
		{
			name:           "implicit, short-hand for random",
			config:         &api.ReleaseBuildConfiguration{},
			dependency:     api.StepDependency{Name: "whatever"},
			expectedStream: api.StableImageStream,
			expectedTag:    "whatever",
			explicit:       false,
		},
		{
			name:           "explicit, long-form for stable",
			config:         &api.ReleaseBuildConfiguration{},
			dependency:     api.StepDependency{Name: "stable:installer"},
			expectedStream: api.StableImageStream,
			expectedTag:    "installer",
			explicit:       true,
		},
		{
			name:           "explicit, long-form for stable, overridden by cluster claim",
			config:         &api.ReleaseBuildConfiguration{},
			claimRelease:   &api.ClaimRelease{ReleaseName: "latest-e2e", OverrideName: "latest"},
			dependency:     api.StepDependency{Name: "stable:installer"},
			expectedStream: "stable-latest-e2e",
			expectedTag:    "installer",
			explicit:       true,
		},
		{
			name:           "explicit, long-form for something crazy",
			config:         &api.ReleaseBuildConfiguration{},
			dependency:     api.StepDependency{Name: "whoa:really"},
			expectedStream: "whoa",
			expectedTag:    "really",
			explicit:       true,
		},
		{
			name:           "explicit, long-form for custom release, overridden by cluster claim",
			config:         &api.ReleaseBuildConfiguration{},
			claimRelease:   &api.ClaimRelease{ReleaseName: "whoa-e2e", OverrideName: "whoa"},
			dependency:     api.StepDependency{Name: "stable-whoa:really"},
			expectedStream: "stable-whoa-e2e",
			expectedTag:    "really",
			explicit:       true,
		},
		{
			name:           "explicit, long-form for custom release, with cluster claim that does not override imagestream",
			config:         &api.ReleaseBuildConfiguration{},
			claimRelease:   &api.ClaimRelease{ReleaseName: "latest-e2e", OverrideName: "latest"},
			dependency:     api.StepDependency{Name: "stable-whoa:really"},
			expectedStream: "stable-whoa",
			expectedTag:    "really",
			explicit:       true,
		},
		{
			name:           "release payload image gets overridden by cluster claim",
			config:         &api.ReleaseBuildConfiguration{},
			claimRelease:   &api.ClaimRelease{ReleaseName: "latest-e2e-claim", OverrideName: "latest"},
			dependency:     api.StepDependency{Name: "release:latest"},
			expectedStream: "release",
			expectedTag:    "latest-e2e-claim",
			explicit:       true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actualStream, actualTag, explicit := testCase.config.DependencyParts(testCase.dependency, testCase.claimRelease)
			if explicit != testCase.explicit {
				t.Errorf("%s: did not correctly determine if ImageStream was explicit (should be %v)", testCase.name, testCase.explicit)
			}
			if actualStream != testCase.expectedStream {
				t.Errorf("%s: did not correctly determine ImageStream wanted %s, got %s", testCase.name, testCase.expectedStream, actualStream)
			}
			if actualTag != testCase.expectedTag {
				t.Errorf("%s: did not correctly determine ImageTag wanted %s, got %s", testCase.name, testCase.expectedTag, actualTag)
			}
		})
	}
}

func TestPipelineImages(t *testing.T) {
	root := api.BuildRootImageConfiguration{FromRepository: true}
	input := api.InputConfiguration{BuildRootImage: &root}
	resources := api.ResourceConfiguration{
		"*": api.ResourceRequirements{
			Requests: api.ResourceList{"cpu": "1"},
		},
	}
	makeImages := func(names ...api.PipelineImageStreamTagReference) (ret []api.ProjectDirectoryImageBuildStepConfiguration) {
		for _, x := range names {
			ret = append(ret, api.ProjectDirectoryImageBuildStepConfiguration{
				To: x,
			})
		}
		return
	}
	for _, tc := range []struct {
		name     string
		conf     api.ReleaseBuildConfiguration
		expected error
	}{{
		name: "all pipeline images are unique",
		conf: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Images:             makeImages("to0", "to1"),
			Resources:          resources,
		},
	}, {
		name: "binary_build_commands",
		conf: api.ReleaseBuildConfiguration{
			BinaryBuildCommands: "binary build commands",
			InputConfiguration:  input,
			Images:              makeImages("bin"),
			Resources:           resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'bin' (previously defined by field 'binary_build_commands')`),
	}, {
		name: "test_binary_build_commands",
		conf: api.ReleaseBuildConfiguration{
			TestBinaryBuildCommands: "test_binary build commands",
			InputConfiguration:      input,
			Images:                  makeImages("test-bin"),
			Resources:               resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'test-bin' (previously defined by field 'test_binary_build_commands')`),
	}, {
		name: "rpm_build_commands",
		conf: api.ReleaseBuildConfiguration{
			RpmBuildCommands:   "rpm build commands",
			InputConfiguration: input,
			Images:             makeImages("rpms"),
			Resources:          resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'rpms' (previously defined by field 'rpm_build_commands')`),
	}, {
		name: "bundle",
		conf: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Images:             makeImages("bundle"),
			Operator: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{As: "bundle"}},
			},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'bundle' (previously defined by field 'operator.bundles[0].as')`),
	}, {
		name: "unnamed bundle",
		conf: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Images:             makeImages("ci-bundle0"),
			Operator: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{}},
			},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'ci-bundle0' (previously defined by field 'operator.bundles[0]')`),
	}, {
		name: "bundle index",
		conf: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Images:             makeImages("ci-index-bundle"),
			Operator: &api.OperatorStepConfiguration{
				Bundles: []api.Bundle{{As: "bundle"}},
			},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'ci-index-bundle' (previously defined by field 'operator.bundles[0].as')`),
	}, {
		name: "bundle source",
		conf: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Images:             makeImages("src-bundle"),
			Operator:           &api.OperatorStepConfiguration{},
			Resources:          resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'src-bundle' (previously defined by field 'operator')`),
	}, {
		name: "base_rpm_images",
		conf: api.ReleaseBuildConfiguration{
			Images: makeImages("base-rpm-image"),
			InputConfiguration: api.InputConfiguration{
				BuildRootImage: &root,
				BaseRPMImages: map[string]api.ImageStreamTagReference{
					"base-rpm-image": {Tag: "tag"},
				},
			},
			RpmBuildCommands: "rpm build commands",
			Resources:        resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'base-rpm-image' (previously defined by field 'base_rpm_images[base-rpm-image]')`),
	}, {
		name: "base_rpm_images without-rpms",
		conf: api.ReleaseBuildConfiguration{
			Images: makeImages("base-rpm-image-without-rpms"),
			InputConfiguration: api.InputConfiguration{
				BuildRootImage: &root,
				BaseRPMImages: map[string]api.ImageStreamTagReference{
					"base-rpm-image": {Tag: "tag"},
				},
			},
			RpmBuildCommands: "rpm build commands",
			Resources:        resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'base-rpm-image-without-rpms' (previously defined by field 'base_rpm_images[base-rpm-image]')`),
	}, {
		name: "base_images",
		conf: api.ReleaseBuildConfiguration{
			Images: makeImages("base-image"),
			InputConfiguration: api.InputConfiguration{
				BuildRootImage: &root,
				BaseImages: map[string]api.ImageStreamTagReference{
					"base-image": {Tag: "tag"},
				},
			},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'base-image' (previously defined by field 'base_images[base-image]')`),
	}, {
		name: "images",
		conf: api.ReleaseBuildConfiguration{
			Images:             makeImages("duplicated", "duplicated"),
			InputConfiguration: input,
			Resources:          resources,
		},
		expected: errors.New(`invalid configuration: images[1]: duplicate image name 'duplicated' (previously defined by field 'images[0]')`),
	}, {
		name: "build_root.project_image_build",
		conf: api.ReleaseBuildConfiguration{
			Images: makeImages("root"),
			InputConfiguration: api.InputConfiguration{
				BuildRootImage: &api.BuildRootImageConfiguration{
					ProjectImageBuild: &api.ProjectDirectoryImageBuildInputs{},
				},
			},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: images[0]: duplicate image name 'root' (previously defined by field 'build_root')`),
	}, {
		name: "multi-stage from_image",
		conf: api.ReleaseBuildConfiguration{
			Images:             makeImages("ns-name-from_image"),
			InputConfiguration: input,
			Tests: []api.TestStepConfiguration{{
				As: "test0",
				MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
					Test: []api.LiteralTestStep{{
						As:       "step-name",
						Commands: "commands",
						FromImage: &api.ImageStreamTagReference{
							Namespace: "ns",
							Name:      "name",
							Tag:       "from_image",
						},
						Resources: resources["*"],
					}},
				},
			}, {
				As: "test1",
				MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
					Test: []api.LiteralTestStep{{
						As:       "step-name",
						Commands: "commands",
						FromImage: &api.ImageStreamTagReference{
							Namespace: "ns",
							Name:      "name",
							Tag:       "from_image",
						},
						Resources: resources["*"],
					}},
				},
			}},
			Resources: resources,
		},
		expected: errors.New(`invalid configuration: tests[0].steps.test[0].from_image: duplicate image name 'ns-name-from_image' (previously defined by field 'images[0]')`),
	}, {
		name: "multi-stage from_image aliased across tests",
		conf: api.ReleaseBuildConfiguration{
			InputConfiguration: input,
			Tests: []api.TestStepConfiguration{{
				As: "test0",
				MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
					Test: []api.LiteralTestStep{{
						As:       "step-name",
						Commands: "commands",
						FromImage: &api.ImageStreamTagReference{
							Namespace: "ns",
							Name:      "name",
							Tag:       "from_image",
						},
						Resources: resources["*"],
					}},
				},
			}, {
				As: "test1",
				MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
					Test: []api.LiteralTestStep{{
						As:       "step-name",
						Commands: "commands",
						FromImage: &api.ImageStreamTagReference{
							Namespace: "ns",
							Name:      "name",
							Tag:       "from_image",
						},
						Resources: resources["*"],
					}},
				},
			}, {
				As: "test2",
				MultiStageTestConfigurationLiteral: &api.MultiStageTestConfigurationLiteral{
					Test: []api.LiteralTestStep{{
						As:       "step-name",
						Commands: "commands",
						FromImage: &api.ImageStreamTagReference{
							Namespace: "ns",
							Name:      "name",
							Tag:       "from_image",
						},
						Resources: resources["*"],
					}},
				},
			}},
			Resources: resources,
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			err := IsValidConfiguration(&tc.conf, "org", "repo")
			testhelper.Diff(t, "error", err, tc.expected, testhelper.EquateErrorMessage)
		})
	}
}

func TestValidateReleaseBuildConfiguration(t *testing.T) {
	testCases := []struct {
		name         string
		input        *api.ReleaseBuildConfiguration
		mergedConfig bool
		expected     []error
	}{
		{
			name:     "empty images and tests -> error",
			input:    &api.ReleaseBuildConfiguration{},
			expected: []error{errors.New("you must define at least one test or image build in 'tests' or 'images'")},
		},
		{
			name: "empty images and tests -> not error if additional images are promoted",
			input: &api.ReleaseBuildConfiguration{
				PromotionConfiguration: &api.PromotionConfiguration{Targets: []api.PromotionTarget{{AdditionalImages: map[string]string{"name": "src"}}}},
			},
		},
		{
			name: "merged config allows plural fields to be set",
			input: &api.ReleaseBuildConfiguration{
				PromotionConfiguration: &api.PromotionConfiguration{Targets: []api.PromotionTarget{{AdditionalImages: map[string]string{"name": "src"}}}},
				BinaryBuildCommandsList: []api.RefCommands{
					{
						Ref:      "org.repo",
						Commands: "build this",
					},
					{
						Ref:      "org.other-repo",
						Commands: "build that",
					},
				},
				TestBinaryBuildCommandsList: []api.RefCommands{
					{
						Ref:      "org.repo",
						Commands: "build test this",
					},
					{
						Ref:      "org.other-repo",
						Commands: "build test that",
					},
				},
				RpmBuildCommandsList: []api.RefCommands{
					{
						Ref:      "org.repo",
						Commands: "rpms",
					},
				},
				RpmBuildLocationList: []api.RefLocation{
					{
						Ref:      "org.repo",
						Location: "here",
					},
				},
			},
			mergedConfig: true,
		},
		{
			name: "non-merged config doesn't allow plural fields to be set",
			input: &api.ReleaseBuildConfiguration{
				PromotionConfiguration: &api.PromotionConfiguration{Targets: []api.PromotionTarget{{AdditionalImages: map[string]string{"name": "src"}}}},
				BinaryBuildCommandsList: []api.RefCommands{
					{
						Ref:      "org.repo",
						Commands: "build this",
					},
					{
						Ref:      "org.other-repo",
						Commands: "build that",
					},
				},
				TestBinaryBuildCommandsList: []api.RefCommands{
					{
						Ref:      "org.repo",
						Commands: "build test this",
					},
					{
						Ref:      "org.other-repo",
						Commands: "build test that",
					},
				},
				RpmBuildCommandsList: []api.RefCommands{
					{
						Ref:      "org.repo",
						Commands: "rpms",
					},
				},
				RpmBuildLocationList: []api.RefLocation{
					{
						Ref:      "org.repo",
						Location: "here",
					},
				},
			},
			expected: []error{errors.New("it is not permissible to directly set: ‘binary_build_commands_list’, ‘test_binary_build_commands_list’, ‘rpm_build_commands_list’, or ‘rpm_build_location_list’")},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.Resources = map[string]api.ResourceRequirements{"*": {Requests: map[string]string{"cpu": "1"}}}
			err := validateReleaseBuildConfiguration(tc.input, "org", "repo", tc.mergedConfig)
			testhelper.Diff(t, "error", err, tc.expected, testhelper.EquateErrorMessage)
		})
	}
}
