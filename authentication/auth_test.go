package authentication

import (
	"net/url"
	"testing"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/surveys"
)

func TestFilterSurveyMetadataClaimsKeepsOnlyRequiredSchemaMetadata(t *testing.T) {
	claimValues := filterSurveyMetadataClaims(parseClaimValues(url.Values{
		"schema_name":     {"census_household_gb_eng"},
		"schema_url":      {""},
		"roles":           {"dumper"},
		"display_address": {"68 Abingdon Road, Goathill"},
		"period_id":       {"201605"},
		"empty_metadata":  {""},
	}), map[string]Metadata{"display_address": {Name: "display_address"}}, false)

	if _, ok := claimValues["schema_url"]; ok {
		t.Fatal("expected empty schema_url to be omitted from survey metadata values")
	}

	if _, ok := claimValues["schema_name"]; ok {
		t.Fatal("expected top-level schema_name to be omitted from survey metadata values")
	}

	if _, ok := claimValues["roles"]; ok {
		t.Fatal("expected top-level roles to be omitted from survey metadata values")
	}

	if _, ok := claimValues["empty_metadata"]; ok {
		t.Fatal("expected empty metadata to be omitted from survey metadata values")
	}

	if _, ok := claimValues["period_id"]; ok {
		t.Fatal("expected non-required metadata to be omitted from survey metadata values")
	}

	if claimValues["display_address"] != "68 Abingdon Road, Goathill" {
		t.Fatalf("expected display_address metadata to be retained, got %v", claimValues["display_address"])
	}
}

func TestAddTopLevelClaimsSetsDefaultsAndTopLevelValues(t *testing.T) {
	claims := make(map[string]interface{})

	addTopLevelClaims(claims, parseClaimValues(url.Values{
		"schema_name":   {"census_household_gb_eng"},
		"language_code": {"en"},
		"channel":       {"true"},
		"roles":         {"dumper", "flusher"},
		"period_id":     {"201605"},
	}))

	if claims["schema_name"] != "census_household_gb_eng" {
		t.Fatalf("expected schema_name claim to be retained, got %v", claims["schema_name"])
	}

	if claims["language_code"] != "en" {
		t.Fatalf("expected language_code claim to be retained, got %v", claims["language_code"])
	}

	if claims["channel"] != true {
		t.Fatalf("expected boolean top-level claim to be parsed, got %v", claims["channel"])
	}

	roles := claims["roles"].([]string)
	if len(roles) != 2 || roles[0] != "dumper" || roles[1] != "flusher" {
		t.Fatalf("expected roles claim to be retained, got %v", roles)
	}

	if claims["version"] != "v2" {
		t.Fatalf("expected version claim to default to v2, got %v", claims["version"])
	}

	if _, ok := claims["tx_id"]; !ok {
		t.Fatal("expected tx_id claim to be set")
	}

	if _, ok := claims["period_id"]; ok {
		t.Fatal("expected survey metadata value to be omitted from top-level claims")
	}
}

func TestFilterSurveyMetadataClaimsAppliesDefaults(t *testing.T) {
	claimValues := filterSurveyMetadataClaims(
		map[string]interface{}{
			"period_id":       "201605",
			"flag":            true,
			"display_address": "68 Abingdon Road, Goathill",
		},
		map[string]Metadata{
			"period_id":       {Name: "period_id", Default: "201606"},
			"flag":            {Name: "flag", Type: "boolean", Default: "false"},
			"display_address": {Name: "display_address"},
			"missing_value":   {Name: "missing_value", Default: "default"},
			"not_required":    {Name: "not_required"},
		},
		true,
	)

	if claimValues["period_id"] != "201605" {
		t.Fatalf("expected URL period_id to be retained, got %v", claimValues["period_id"])
	}

	if claimValues["flag"] != true {
		t.Fatalf("expected URL flag to be retained, got %v", claimValues["flag"])
	}

	if claimValues["missing_value"] != "default" {
		t.Fatalf("expected missing value default to be applied, got %v", claimValues["missing_value"])
	}

	if claimValues["display_address"] != "68 Abingdon Road, Goathill" {
		t.Fatalf("expected existing metadata values to be retained, got %v", claimValues["display_address"])
	}
}

func TestFilterSurveyMetadataClaimsAppliesPostedBooleanMetadata(t *testing.T) {
	surveyMetadata := filterSurveyMetadataClaims(
		parseClaimValues(url.Values{
			"flag":      {"on"},
			"period_id": {"201605"},
		}),
		map[string]Metadata{
			"flag":         {Name: "flag", Type: "boolean"},
			"missing_flag": {Name: "missing_flag", Type: "boolean"},
			"period_id":    {Name: "period_id"},
		},
		false,
	)

	if _, ok := surveyMetadata["data"]; ok {
		t.Fatalf("expected metadata to remain flat, got %v", surveyMetadata)
	}

	if surveyMetadata["flag"] != true {
		t.Fatalf("expected present boolean metadata to be true, got %v", surveyMetadata["flag"])
	}

	if surveyMetadata["missing_flag"] != false {
		t.Fatalf("expected missing boolean metadata to be false, got %v", surveyMetadata["missing_flag"])
	}

	if surveyMetadata["period_id"] != "201605" {
		t.Fatalf("expected non-boolean metadata to be preserved, got %v", surveyMetadata["period_id"])
	}
}

func TestFilterSurveyMetadataClaimsParsesExplicitBooleanMetadata(t *testing.T) {
	surveyMetadata := filterSurveyMetadataClaims(
		parseClaimValues(url.Values{
			"flag": {"false"},
		}),
		map[string]Metadata{"flag": {Name: "flag", Type: "boolean"}},
		false,
	)

	if surveyMetadata["flag"] != false {
		t.Fatalf("expected explicit boolean metadata to be parsed, got %v", surveyMetadata["flag"])
	}
}

func TestAddSchemaClaimUsesSchemaURL(t *testing.T) {
	claims := map[string]interface{}{
		"schema_name": "census_household_gb_eng",
	}

	addSchemaClaim(claims, surveys.LauncherSchema{
		Name: "census_household_gb_eng",
		URL:  "https://example.com/schema.json",
	})

	if claims["schema_url"] != "https://example.com/schema.json" {
		t.Fatalf("expected schema_url claim, got %v", claims["schema_url"])
	}

	if _, ok := claims["schema_name"]; ok {
		t.Fatalf("expected schema_name claim to be omitted, got %v", claims)
	}
}

func TestAddSchemaClaimUsesSchemaName(t *testing.T) {
	claims := map[string]interface{}{
		"schema_url": "https://example.com/schema.json",
	}

	addSchemaClaim(claims, surveys.LauncherSchema{
		Name: "census_household_gb_eng",
	})

	if claims["schema_name"] != "census_household_gb_eng" {
		t.Fatalf("expected schema_name claim, got %v", claims["schema_name"])
	}

	if _, ok := claims["schema_url"]; ok {
		t.Fatalf("expected schema_url claim to be omitted, got %v", claims)
	}
}
