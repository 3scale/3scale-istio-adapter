package templating

import "testing"

func TestK8sResourceNameValidation(t *testing.T) {
	const maxLen = resourceNameValueMaxLength
	inputs := []struct {
		name       string
		original   string
		maxLen     int
		fixUp      bool
		updated    string
		expectErrs bool
	}{
		{
			name:       "Test empty not fixed up",
			original:   "",
			fixUp:      false,
			expectErrs: true,
		},
		{
			name:     "Test empty fix up",
			original: "",
			fixUp:    true,
			updated:  "a-resource",
		},
		{
			name:     "Test valid string with dots that should be modified",
			original: "some.valid.input",
			fixUp:    true,
			updated:  "some.valid.input",
		},
		{
			name:     "Test invalid string that should be modified",
			original: "-ome-invalid-input",
			fixUp:    true,
			updated:  "aome-invalid-input",
		},
		{
			name:       "Test invalid string that should not be modified",
			original:   "-some-invalid-input",
			fixUp:      false,
			expectErrs: true,
		},
		{
			name:     "Test invalid length (positive) is set to default",
			original: "some-valid-input",
			maxLen:   10000000,
			fixUp:    false,
			updated:  "some-valid-input",
		},
		{
			name:     "Test invalid length (negative) is set to default",
			original: "some-valid-input",
			maxLen:   -1,
			fixUp:    false,
			updated:  "some-valid-input",
		},
		{
			name:       "Test long string that should be valid if fixed up - fails",
			original:   "some-valid-input",
			maxLen:     3,
			fixUp:      false,
			updated:    "som",
			expectErrs: true,
		},

		{
			name:       "Test long string that should be valid if fixed up - fixed",
			original:   "some-valid-input",
			maxLen:     3,
			fixUp:      true,
			updated:    "som",
			expectErrs: false,
		},
		{
			name:       "Test valid string that should not be modified",
			original:   "some-valid-input",
			fixUp:      true,
			updated:    "some-valid-input",
			expectErrs: false,
		},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			if input.maxLen == 0 {
				input.maxLen = maxLen
			}
			s, errs := K8sResourceNameValidation(input.original, input.maxLen, input.fixUp)
			if len(errs) > 0 {
				if !input.expectErrs {
					t.Errorf("expected no errors but got %d", len(errs))
				}
				return
			}

			if s != input.updated {
				t.Errorf("expected output of %s but got %s", input.updated, s)
			}
		})
	}
}

func TestK8sLabelValueValidation(t *testing.T) {
	const maxLen = dns1123LabelValueMaxLength
	inputs := []struct {
		name       string
		original   string
		maxLen     int
		fixUp      bool
		updated    string
		expectErrs bool
	}{
		{
			name:       "Test empty not fixed up",
			original:   "",
			fixUp:      false,
			expectErrs: true,
		},
		{
			name:     "Test empty fix up",
			original: "",
			fixUp:    true,
			updated:  "",
		},
		{
			name:     "Test valid string with dots that should be modified",
			original: "some.valid.input",
			fixUp:    true,
			updated:  "some.valid.input",
		},
		{
			name:     "Test invalid string that should be modified",
			original: "-ome-invalid-input",
			fixUp:    true,
			updated:  "aome-invalid-input",
		},
		{
			name:       "Test invalid string that should not be modified",
			original:   "-some-invalid-input",
			fixUp:      false,
			expectErrs: true,
		},
		{
			name:     "Test invalid length (positive) is set to default",
			original: "some-valid-input",
			maxLen:   10000000,
			fixUp:    false,
			updated:  "some-valid-input",
		},
		{
			name:     "Test invalid length (negative) is set to default",
			original: "some-valid-input",
			maxLen:   -1,
			fixUp:    false,
			updated:  "some-valid-input",
		},
		{
			name:       "Test long string that should be valid if fixed up - fails",
			original:   "some-valid-input",
			maxLen:     3,
			fixUp:      false,
			updated:    "som",
			expectErrs: true,
		},

		{
			name:       "Test long string that should be valid if fixed up - fixed",
			original:   "some-valid-input",
			maxLen:     3,
			fixUp:      true,
			updated:    "som",
			expectErrs: false,
		},
		{
			name:       "Test valid string that should not be modified",
			original:   "some-valid-input",
			fixUp:      true,
			updated:    "some-valid-input",
			expectErrs: false,
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			if input.maxLen == 0 {
				input.maxLen = maxLen
			}
			s, errs := K8sLabelValueValidation(input.original, input.maxLen, input.fixUp)
			if len(errs) > 0 {
				if !input.expectErrs {
					t.Errorf("expected no errors but got %d", len(errs))
				}
				return
			}

			if s != input.updated {
				t.Errorf("expected output of %s but got %s", input.updated, s)
			}
		})

	}
}
