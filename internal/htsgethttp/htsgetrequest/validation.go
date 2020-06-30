// Package htsgetrequest provides operations for parsing htsget-related
// parameters from the HTTP request, and performing validation and
// transformation
//
// Module validation.go defines functions for validating whether the value of
// HTTP request parameters are acceptable
package htsgetrequest

import (
	"bufio"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ga4gh/htsget-refserver/internal/config"
	"github.com/ga4gh/htsget-refserver/internal/htsgeterror"
	"github.com/ga4gh/htsget-refserver/internal/htsgetutils"
)

// validationByParam (map[string]func(string, *HtsgetRequest) (bool, string)):
// the correct validation function for each request parameter name. each function
// returns a boolean indicating whether the parameter passed validation, and a
// string indicating why validation failed (if it failed)
var validationByParam = map[string]func(string, *HtsgetRequest) (bool, string){
	"id":               validateID,
	"format":           validateFormat,
	"class":            validateClass,
	"referenceName":    validateReferenceNameExists,
	"start":            validateStart,
	"end":              validateEnd,
	"fields":           validateFields,
	"tags":             noValidation,
	"notags":           validateNoTags,
	"HtsgetBlockClass": validateClass,
	"HtsgetBlockId":    noValidation,
	"HtsgetNumBlocks":  noValidation,
}

// errorsByParam (map[string]func(http.ResponseWriter, *string)): the correct
// error to raise for each request parameter validation
var errorsByParam = map[string]func(http.ResponseWriter, *string){
	"id":               htsgeterror.NotFound,
	"format":           htsgeterror.UnsupportedFormat,
	"class":            htsgeterror.InvalidInput,
	"referenceName":    htsgeterror.InvalidRange,
	"start":            htsgeterror.InvalidRange,
	"end":              htsgeterror.InvalidRange,
	"fields":           htsgeterror.InvalidInput,
	"tags":             htsgeterror.InvalidInput,
	"notags":           htsgeterror.InvalidInput,
	"HtsgetBlockClass": htsgeterror.InvalidInput,
	"HtsgetBlockId":    htsgeterror.InternalServerError,
	"HtsgetNumBlocks":  htsgeterror.InternalServerError,
}

// isInteger determines if a string can be parsed as an integer
//
// Arguments
//	value (string): string to check
// Returns
//	(bool): true if the string can be converted to an integer, false if not
func isInteger(value string) bool {
	_, err := strconv.Atoi(value)
	if err != nil {
		return false
	}
	return true
}

// isGreaterThanEqualToZero determines if a string can be parsed as an integer,
// and is greater than or equal to zero
//
// Arguments
//	value (string): string to check
// Returns
// (bool): true if string is a valid integer greater than or equal to zero
func isGreaterThanEqualToZero(value string) bool {
	if !isInteger(value) {
		return false
	}
	num, _ := strconv.Atoi(value)
	if num < 0 {
		return false
	}
	return true
}

// noValidation is an empty validation function for request parameters that do
// not need to be validated. always returns true
//
// Arguments
//	value (string): parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): always true
//	(string): always empty
func noValidation(value string, htsgetReq *HtsgetRequest) (bool, string) {
	return true, ""
}

// validateID validates the 'id' path parameter. checks if an object matching
// the 'id' could be found from the data source
//
// Arguments:
//	id (string): id parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if a resource matching id could be found from the data source
//	(string): diagnostic message if error encountered
func validateID(id string, htsgetReq *HtsgetRequest) (bool, string) {
	res, err := http.Head(config.DataSourceURL + htsgetutils.FilePath(id))
	if err != nil {
		return false, "The requested resource was not found"
	}
	res.Body.Close()
	if res.Status == "404 Not Found" {
		return false, "The requested resource was not found"
	}
	return true, ""
}

// validateFormat validates the 'format' query string parameter. checks if the
// requested format is one of the allowed options
//
// Arguments
//	format (string): format parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if an allowed format was requested
//	(string): diagnostic message if error encountered
func validateFormat(format string, htsgetReq *HtsgetRequest) (bool, string) {
	switch strings.ToUpper(format) {
	case "BAM":
		return true, ""
	case "CRAM":
		return false, "CRAM not supported" // currently not supported
	default:
		return false, "file format: '" + format + "' not supported"
	}
}

// validateClass validates the 'class' query string parameter. checks if the
// requested class is one of the allowed options
//
// Arguments
//	class (string): class parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if an allowed class was requested
//	(string): diagnostic message if error encountered
func validateClass(class string, htsgetReq *HtsgetRequest) (bool, string) {
	switch strings.ToLower(class) {
	case "header":
		return true, ""
	case "body":
		return false, "'body' only requests currently not supported" // currently not supported
	default:
		return false, "class: '" + class + "' not supported"
	}
}

// validateReferenceNameExists validates the 'referenceName' query string
// parameter. checks if the requested reference contig/chromosome is in the
// BAM/CRAM header sequence dictionary
//
// Arguments
//	referenceName (string): referenceName parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if requested reference sequence name is in sequence dictionary
//	(string): diagnostic message if error encountered
func validateReferenceNameExists(referenceName string, htsgetReq *HtsgetRequest) (bool, string) {
	id := htsgetReq.ID()
	cmd := exec.Command("samtools", "view", "-H", config.DataSourceURL+htsgetutils.FilePath(id))
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return false, "Could not access requested file"
	}
	cmd.Start()
	reader := bufio.NewReader(pipe)
	l, _, err := reader.ReadLine()

	for ; err == nil; l, _, err = reader.ReadLine() {
		if strings.Contains(string(l), "SN:"+referenceName) {
			return true, ""
		}
	}
	cmd.Wait()
	return false, "invalid 'referenceName': " + referenceName
}

// validateStart validates the 'start' query string parameter. checks that it is
// a valid, non-zero integer, and that it is being used correctly in conjunction
// with 'referenceName'
//
// Arguments
//	start (string): start parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if start is correctly specified
//	(string): diagnostic message if error encountered
func validateStart(start string, htsgetReq *HtsgetRequest) (bool, string) {
	referenceName := htsgetReq.ReferenceName()

	// start requires referenceName to be specified as well
	if referenceName == "*" || referenceName == "" {
		return false, "'start' cannot be set without 'referenceName'"
	}

	// start must be an integer
	if !isInteger(start) {
		return false, "'start' is not a valid integer"
	}

	// start must be >= 0
	if !isGreaterThanEqualToZero(start) {
		return false, "'start' must be greater than or equal to zero"
	}

	return true, ""
}

// validateEnd validates the 'end' query string parameter. checks that it is a
// valid, non-zero integer, that it's being used correctly in conjunction with
// 'referenceName', and that the end coordinate is greater than the start
// coordinate
//
// Arguments
//	end (string): end parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if end is correctly specified
//	(string): diagnostic message if error encountered
func validateEnd(end string, htsgetReq *HtsgetRequest) (bool, string) {
	referenceName := htsgetReq.ReferenceName()
	start := htsgetReq.Start()

	// end required referenceName to be specified as well
	if referenceName == "*" || referenceName == "" {
		return false, "'end' cannot be set without 'referenceName'"
	}

	// end must be an integer
	if !isInteger(end) {
		return false, "'end' is not a valid integer"
	}

	// end must be >= 0
	if !isGreaterThanEqualToZero(end) {
		return false, "'end' must be greater than or equal to zero"
	}

	// if start is specified, end must be greater than start
	if start != "-1" {
		startNum, startErr := strconv.Atoi(start)
		endNum, endErr := strconv.Atoi(end)
		if startErr != nil || endErr != nil {
			return false, "error converting 'start' and/or 'end' to integers"
		}
		if startNum >= endNum {
			return false, "'end' MUST be higher than 'start'"
		}
	}

	return true, ""
}

// validateFields validates the 'fields' query string parameter. checks that
// every requested field is an acceptable value (an expected BAM/CRAM field)
//
// Arguments
//	fields (string): unsplit fields parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if all requested fields are canonical field names
//	(string): diagnostic message if error encountered
func validateFields(fields string, htsgetReq *HtsgetRequest) (bool, string) {
	fieldsList := splitAndUppercase(fields)
	for _, fieldItem := range fieldsList {
		if _, ok := config.BamFields[fieldItem]; !ok {
			return false, "'" + fieldItem + "' not an acceptable field"
		}
	}
	return true, ""
}

// validateNoTags validates the 'notags' query string parameter. checks that
// there is no overlap between tags included by 'tags' and tags excluded by
// 'notags'
//
// Arguments
//	notags (string): unsplit notags parameter value
//	htsgetReq (*HtsgetRequest): htsget request object
// Returns
//	(bool): true if there is no overlap between tags and notags
//	(string): diagnostic message if error encountered
func validateNoTags(notags string, htsgetReq *HtsgetRequest) (bool, string) {
	tagsList := htsgetReq.Tags()
	notagsList := splitOnComma(notags)

	for _, tagItem := range tagsList {
		for _, notagItem := range notagsList {
			if tagItem == notagItem {
				return false, "'" + tagItem + "' cannot be in both 'tags' and 'notags'"
			}
		}
	}
	return true, ""
}
