package authentication

import (
	"net/url"
	"testing"
)

func TestGetSurveyMetadataFromClaimsSkipsEmptyValues(t *testing.T) {
	claims := make(map[string]interface{})

	getSurveyMetadataFromClaims(url.Values{
		"schema_name":     {"census_household_gb_eng"},
		"schema_url":      {""},
		"display_address": {"68 Abingdon Road, Goathill"},
		"empty_metadata":  {""},
	}, claims)

	if _, ok := claims["schema_url"]; ok {
		t.Fatal("expected empty schema_url to be omitted from claims")
	}

	if claims["schema_name"] != "census_household_gb_eng" {
		t.Fatalf("expected schema_name claim to be retained, got %v", claims["schema_name"])
	}

	surveyMetadata := claims["survey_metadata"].(map[string]interface{})
	if _, ok := surveyMetadata["empty_metadata"]; ok {
		t.Fatal("expected empty metadata to be omitted from survey metadata")
	}

	if surveyMetadata["display_address"] != "68 Abingdon Road, Goathill" {
		t.Fatalf("expected display_address metadata to be retained, got %v", surveyMetadata["display_address"])
	}
}
