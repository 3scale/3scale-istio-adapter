package templating

import (
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

type validationCheckerFn func(string, int) bool
type validationCopierFn func([]byte, string, int, bool) (bool, []byte)

func isLowerAlphaNum(ch byte) bool {
	if ch < '0' || ch > 'z' {
		return false
	} else if ch > '9' && ch < 'a' {
		return false
	}

	return true
}

func isLowerAlphaNumDash(ch byte) bool {
	return isLowerAlphaNum(ch) || ch == '-'
}

func isAlphaNum(ch byte) bool {
	return isLowerAlphaNum(ch) || (ch >= 'A' && ch <= 'Z')
}

func isAlphaNumDashDot(ch byte) bool {
	return isAlphaNum(ch) || ch == '-' || ch == '.'
}

func isAlphaNumDashDotUnderscore(ch byte) bool {
	return isAlphaNumDashDot(ch) || ch == '_'
}

func checkLengthWithTrimming(value string, maxLength int, trim bool) (string, bool, int) {
	var valid bool

	// check if the value is too long
	length := len(value)

	if length > maxLength {
		if trim {
			// trim the rightmost part which is usually the most generic
			return value[:maxLength], true, maxLength
		}
	} else {
		valid = true
	}

	return value, valid, length
}

// Receives a string and a validator function that receives an index in the
// string and the character and returns whether the character is valid or not.
// Returns the list of positions where validation errors were found.
func validateString(s string, validator func(string, int) bool) []int {
	var failedIndexes []int

	length := len(s)

	for idx := 0; idx < length; idx++ {
		if !validator(s, idx) {
			failedIndexes = append(failedIndexes, idx)
		}
	}

	return failedIndexes
}

// Validates and copies the string (possibly modified) to a new byte array
//
// Needs a validator function and a copier function.
// The validator function is passed straight to validateString
// The copier function receives the byte array being built, the string, its index,
// and whether the source character passed validation, and returns the (potentially
// reallocated) byte array and whether the copied character was valid.
//
// Returns the copied and possibly modified string, and the list of source positions where
// validation errors were found.
func validateAndCopyString(s string, validator validationCheckerFn, copier validationCopierFn) (string, []int) {
	newValue := make([]byte, 0, len(s))

	failedIndexes := validateString(s, func(s string, idx int) bool {
		ok := validator(s, idx)
		ok, newValue = copier(newValue, s, idx, ok)

		return ok
	})

	return string(newValue), failedIndexes
}

// takes a user supplied length and ensures it is within bounds else return the forced vale
func maxLenEnforcer(suppliedLen, minBound, forceTo int) int {
	if suppliedLen < minBound || suppliedLen > forceTo {
		return forceTo
	}
	return suppliedLen
}

// returns a copier function - a closure to the supplied params
func getCopierFN(length int, fixup bool, edgeFix, bodyFix byte) validationCopierFn {
	return func(byteSlice []byte, s string, idx int, ok bool) (bool, []byte) {
		var c byte

		if ok || !fixup {
			c = s[idx]
		} else {
			// can always fix this
			ok = true

			if idx == 0 || idx == length-1 {
				c = edgeFix
			} else {
				c = bodyFix
			}
		}

		return ok, append(byteSlice, c)
	}
}

// returns a validationCheckerFn closure required by k8sDNS1123LabelValidation
func getK8DNS1123LabelValidationCheckerFN(length int) validationCheckerFn {
	return func(s string, idx int) bool {
		c := s[idx]

		// check the edges, beginning and ending of the string
		if idx == 0 || idx == length-1 {
			return isLowerAlphaNum(c)
		}

		// check the body
		return isLowerAlphaNumDash(c)
	}
}

// returns a validationCheckerFn closure required by K8sLabelValueValidation
func getK8LabelValidationCheckerFN(length int) validationCheckerFn {
	return func(s string, idx int) bool {
		c := s[idx]

		// check the edges, beginning and ending of the string
		if idx == 0 || idx == length-1 {
			return isAlphaNum(c)
		}

		// check the body
		return isAlphaNumDashDotUnderscore(c)
	}
}

// wrapper function for copying and validation for k8sDNS1123LabelValidation
func k8DNS1123ValidateAndCopy(value string, length int, fixup bool) (string, []int) {
	return validateAndCopyString(value, getK8DNS1123LabelValidationCheckerFN(length), getCopierFN(length, fixup, dns1123LabelValueEdgeFix, dns1123LabelValueBodyFix))
}

// wrapper function for copying and validation for K8sLabelValueValidation
func k8LabelValidateAndCopy(value string, length int, fixup bool) (string, []int) {
	return validateAndCopyString(value, getK8LabelValidationCheckerFN(length), getCopierFN(length, fixup, labelValueEdgeFix, labelValueBodyFix))
}

const dns1123LabelMaxPositionsReported = 10
const dns1123LabelValueMaxLength int = 63
const dns1123LabelValueBodyFix byte = '-'
const dns1123LabelValueEdgeFix byte = 'a'

// Validate and potentially fix up a DNS RFC 1123 label value
//
// The rules from RFC 1123 are:
// * Maximum 63 characters long, minimum 1 character long
// * Lower case alphanumeric characters, plus '-' except in the first or last character.
func k8sDNS1123LabelValidation(value string, maxLen int, fixup bool) (string, []error) {
	var errs []error

	maxLen = maxLenEnforcer(maxLen, 1, dns1123LabelValueMaxLength)

	value, valid, length := checkLengthWithTrimming(value, maxLen, fixup)

	if !valid {
		errs = append(errs, fmt.Errorf("error. DNS 1123 label value %s too long (%d bytes/%d max)", value, maxLen, length))
	}

	// 0-length is not a valid label value
	if length == 0 {
		errs = append(errs, fmt.Errorf("error. DNS 1123 label value cannot be empty"))
	} else {

		copiedValue, errorPositions := k8DNS1123ValidateAndCopy(value, length, fixup)

		if errorPositions != nil {
			posLen := len(errorPositions)
			if posLen > dns1123LabelMaxPositionsReported {
				posLen = dns1123LabelMaxPositionsReported
			}

			errorPositionsStr := make([]string, 0, posLen)

			for i := 0; i < posLen; i++ {
				errorPositionsStr = append(errorPositionsStr, strconv.Itoa(errorPositions[i]))
			}

			errs = append(errs, fmt.Errorf("error. DNS 1123 label value %s contains invalid characters at positions %s", value, strings.Join(errorPositionsStr, ", ")))
		}

		value = copiedValue
	}

	// validate the DNS 1123 label with k8s code
	validationErrs := validation.IsDNS1123Label(value)
	if len(validationErrs) > 0 {
		errs = append(errs, fmt.Errorf("error. DNS 1123 label value %s does not pass upstream k8s validation:\n%s", value, strings.Join(validationErrs, "\n")))
	}

	return value, errs
}

const resourceNameValueMaxLength int = 253
const resourceNameDefaultValue string = "a-resource"

// Validate and potentially fix up k8s resource names
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/
//
// The rules as of March 2019 are:
// * Must be a DNS SUBDOMAIN by the rules of RFC 1123, consisting of:
//   - Maximum 253 characters long, minimum 1
//   - One or more lowercase RFC 1123 labels separated by '.'
//
// Parameters:
// - value: the resource name string to validate
// - maxLen: the maximum length in case you have extra constraints, or use a negative
//           value to indicate that a value up to the spec's maximum length is validated.
// - fixup: set to true to try to correct any deviation from the spec.
//
// Returned values:
// - string: the validated and possibly corrected value
// - []error: array of errors describing the issues found
func K8sResourceNameValidation(value string, maxLen int, fixup bool) (string, []error) {
	var errs []error

	maxLen = maxLenEnforcer(maxLen, 1, resourceNameValueMaxLength)

	value, valid, length := checkLengthWithTrimming(value, maxLen, fixup)

	if !valid {
		errs = append(errs, fmt.Errorf("error. Resource name %s too long (%d bytes/%d max)", value, maxLen, length))
	}

	labels := strings.Split(value, ".")

	// we use this counter to avoid accumulating empty labels when fixing up
	i := 0
	for _, label := range labels {
		// ignore empty labels if fixing up the value
		if fixup && len(label) == 0 {
			continue
		}
		label, errors := k8sDNS1123LabelValidation(label, -1, fixup)
		if len(errors) > 0 {
			errs = append(errs, errors...)
		}
		labels[i] = label
		i += 1
	}

	// Cannot have 0 labels
	if i == 0 {
		if fixup {
			length = len(resourceNameDefaultValue)
			if length > maxLen {
				length = maxLen
			}
			value = resourceNameDefaultValue[:length]
		} else {
			errs = append(errs, fmt.Errorf("error. Resource name cannot be empty"))
		}
	} else {
		// join valid labels
		value = strings.Join(labels[:i], ".")
	}

	// validate that we can create a k8s object with the current (maybe fixed) value
	validationErrs := validation.IsDNS1123Subdomain(value)
	if len(validationErrs) > 0 {
		errs = append(errs, fmt.Errorf("error. Resource name %s does not pass upstream k8s validation:\n%s", value, strings.Join(validationErrs, "\n")))
	}

	return value, errs
}

const labelValueMaxPositionsReported = 10
const labelValueMaxLength int = 63
const labelValueBodyFix byte = '_'
const labelValueEdgeFix byte = 'a'

// Validate and potentially fix up k8s label values
// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
//
// The rules as of March 2019 are:
// * Maximum 63 characters long
// * Must be empty or start and end with an alphanumeric character (caps
//   included), with alphanumerics, dashes, underscores and dots allowed in
//   between.
//
// Parameters:
// - value: the label value string to validate
// - maxLen: the maximum length in case you have extra constraints, or use a negative
//           value to indicate that a value up to the spec's maximum length is validated.
// - fixup: set to true to try to correct any deviation from the spec.
//
// Returned values:
// - string: the validated and possibly corrected value
// - []error: array of errors describing the issues found
func K8sLabelValueValidation(value string, maxLen int, fixup bool) (string, []error) {
	var errs []error

	// use -1 to use the spec's maximum length
	maxLen = maxLenEnforcer(maxLen, 0, resourceNameValueMaxLength)

	value, valid, length := checkLengthWithTrimming(value, maxLen, fixup)

	if !valid {
		errs = append(errs, fmt.Errorf("error. Label value too long (%d bytes/%d max): %s", length, maxLen, value))
	}

	// 0-length is a valid value (although arguably useless)
	if length > 0 {

		newValue, errorPositions := k8LabelValidateAndCopy(value, length, fixup)

		if errorPositions != nil {
			posLen := len(errorPositions)
			if posLen > labelValueMaxPositionsReported {
				posLen = labelValueMaxPositionsReported
			}

			errorPositionsStr := make([]string, 0, posLen)

			for i := 0; i < posLen; i++ {
				errorPositionsStr = append(errorPositionsStr, strconv.Itoa(errorPositions[i]))
			}

			errs = append(errs, fmt.Errorf("error. Label value %s contains invalid characters at positions %s", value, strings.Join(errorPositionsStr, ", ")))
		}

		value = newValue
	}

	// validate that we can create a k8s object with the current (maybe fixed) value
	validationErrs := validation.IsValidLabelValue(value)
	if len(validationErrs) > 0 {
		errs = append(errs, fmt.Errorf("error. Label value %s does not pass upstream k8s validation:\n%s", value, strings.Join(validationErrs, "\n")))
	}

	return value, errs
}
