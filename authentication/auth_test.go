package authentication

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/surveys"
)

func TestGetClaimValueReportsMissingClaim(t *testing.T) {
	value, missing, err := getClaimValue(url.Values{}, "channel")
	if err != nil {
		t.Fatal(err)
	}
	if !missing {
		t.Fatal("expected missing claim to be reported as missing")
	}
	if value != "" {
		t.Errorf("value = %#v, expected empty string", value)
	}
}

func TestGetClaimValueKeepsTopLevelBooleanLikeValueAsString(t *testing.T) {
	value, missing, err := getClaimValue(url.Values{"channel": {"true"}}, "channel")
	if err != nil {
		t.Fatal(err)
	}
	if missing {
		t.Fatal("expected channel claim to be present")
	}
	if value != "true" {
		t.Errorf("value = %#v, expected string \"true\"", value)
	}
}

func TestGetClaimValueRejectsMultipleValues(t *testing.T) {
	_, _, err := getClaimValue(url.Values{"channel": {"web", "phone"}}, "channel")
	if err == nil {
		t.Fatal("expected multiple claim values to return an error")
	}
}

func TestAddTopLevelClaims(t *testing.T) {
	claims := make(map[string]interface{})
	values := url.Values{
		"language_code": {"en"},
		"channel":       {"true"},
		"claim_x":       {"should not be added"},
	}

	if err := addTopLevelClaims(claims, values, false); err != nil {
		t.Fatal(err)
	}

	if claims["language_code"] != "en" {
		t.Errorf("language_code = %v, expected en", claims["language_code"])
	}
	if value, ok := claims["channel"].(string); !ok || value != "true" {
		t.Errorf("channel = %#v, expected string \"true\"", claims["channel"])
	}
	if _, ok := claims["claim_x"]; ok {
		t.Error("unexpected claim_x in top-level claims")
	}
	if claims["version"] != "v2" {
		t.Errorf("version = %v, expected v2", claims["version"])
	}
	if _, ok := claims["tx_id"]; !ok {
		t.Error("missing tx_id")
	}
}

func TestAddTopLevelClaimsSetsRolesClaim(t *testing.T) {
	dumperClaims := make(map[string]interface{})
	if err := addTopLevelClaims(dumperClaims, url.Values{}, false); err != nil {
		t.Fatal(err)
	}
	if roles, ok := dumperClaims["roles"].([]string); !ok || len(roles) != 1 || roles[0] != "dumper" {
		t.Errorf("dumper roles = %v, expected [dumper]", dumperClaims["roles"])
	}

	flusherClaims := make(map[string]interface{})
	if err := addTopLevelClaims(flusherClaims, url.Values{}, true); err != nil {
		t.Fatal(err)
	}
	if roles, ok := flusherClaims["roles"].([]string); !ok || len(roles) != 1 || roles[0] != "flusher" {
		t.Errorf("flusher roles = %v, expected [flusher]", flusherClaims["roles"])
	}
}

func TestGenerateSurveyMetadataClaimsSelectsAndDefaultsMetadata(t *testing.T) {
	claims, err := generateSurveyMetadataClaims(
		url.Values{
			"true_claim":   {"true"},
			"false_claim":  {"false"},
			"string_claim": {"test"},
		},
		[]Metadata{
			{Name: "true_claim", Type: "boolean"},
			{Name: "false_claim", Type: "boolean"},
			{Name: "missing_boolean_claim", Type: "boolean"},
			{Name: "string_claim", Type: "string"},
			{Name: "default_claim", Type: "string", Default: "B"},
			{Name: "missing_claim", Type: "string"},
			{Name: "optional_with_default", Type: "string", Optional: true, Default: "do not use"},
			{Name: "optional_without_default", Type: "string", Optional: true},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	expectedClaims := map[string]interface{}{
		"true_claim":            true,
		"false_claim":           false,
		"missing_boolean_claim": false,
		"string_claim":          "test",
		"default_claim":         "B",
	}
	for name, expected := range expectedClaims {
		if claims[name] != expected {
			t.Errorf("%s = %#v, expected %#v", name, claims[name], expected)
		}
	}
	for _, name := range []string{"missing_claim", "optional_with_default", "optional_without_default"} {
		if _, ok := claims[name]; ok {
			t.Errorf("unexpected %s claim: %v", name, claims[name])
		}
	}
}

func TestGenerateSurveyMetadataClaimsInvalidBooleanReturnsError(t *testing.T) {
	_, err := generateSurveyMetadataClaims(
		url.Values{"flag": {"sometimes"}},
		[]Metadata{{Name: "flag", Type: "boolean"}},
	)
	if err == nil {
		t.Fatal("expected invalid boolean value to return an error")
	}
}

func TestAddSchemaClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tests := []struct {
		name            string
		launcherSchema  surveys.LauncherSchema
		expectedClaim   string
		expectedValue   string
		expectCacheBust bool
	}{
		{
			name:            "schema URL takes precedence and gets cache busting",
			launcherSchema:  surveys.LauncherSchema{URL: server.URL + "/schema.json"},
			expectedClaim:   "schema_url",
			expectedValue:   server.URL + "/schema.json",
			expectCacheBust: true,
		},
		{
			name:           "schema URL with an existing query is left unchanged",
			launcherSchema: surveys.LauncherSchema{URL: server.URL + "/schema.json?version=1"},
			expectedClaim:  "schema_url",
			expectedValue:  server.URL + "/schema.json?version=1",
		},
		{
			name:           "schema name when URL is absent",
			launcherSchema: surveys.LauncherSchema{Name: "example_schema"},
			expectedClaim:  "schema_name",
			expectedValue:  "example_schema",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := make(map[string]interface{})
			err := addSchemaClaim(claims, test.launcherSchema)
			if err != nil {
				t.Fatalf("addSchemaClaim failed: %v", err)
			}

			actualValue, ok := claims[test.expectedClaim].(string)
			if !ok {
				t.Fatalf("%s = %v, expected a string", test.expectedClaim, claims[test.expectedClaim])
			}
			if test.expectCacheBust {
				parsedURL, err := url.Parse(actualValue)
				if err != nil {
					t.Fatalf("could not parse schema URL %q: %v", actualValue, err)
				}
				if parsedURL.Query().Get("bust") == "" {
					t.Errorf("schema URL %q is missing cache-bust query parameter", actualValue)
				}
				parsedURL.RawQuery = ""
				actualValue = parsedURL.String()
			}
			if actualValue != test.expectedValue {
				t.Errorf("%s = %v, expected %s", test.expectedClaim, actualValue, test.expectedValue)
			}
		})
	}
}

func TestAddSchemaClaimUsesCensusSelector(t *testing.T) {
	tests := []struct {
		name               string
		expectedFormType   string
		expectedRegionCode string
	}{
		{name: "census_household_gb_eng", expectedFormType: "H", expectedRegionCode: "GB-ENG"},
		{name: "census_household_gb_wls", expectedFormType: "H", expectedRegionCode: "GB-WLS"},
		{name: "census_individual_gb_eng", expectedFormType: "I", expectedRegionCode: "GB-ENG"},
		{name: "census_communal_establishment_gb_eng", expectedFormType: "C", expectedRegionCode: "GB-ENG"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := make(map[string]interface{})
			if err := addSchemaClaim(claims, surveys.LauncherSchema{Name: test.name}); err != nil {
				t.Fatalf("addSchemaClaim failed: %v", err)
			}

			if _, ok := claims["schema_name"]; ok {
				t.Fatal("unexpected schema_name claim for census schema")
			}
			selector, ok := claims["schema"].(map[string]string)
			if !ok {
				t.Fatalf("schema = %v, expected a selector map", claims["schema"])
			}
			if selector["survey"] != "census" {
				t.Errorf("schema survey = %q, expected census", selector["survey"])
			}
			if selector["form_type"] != test.expectedFormType {
				t.Errorf("schema form_type = %q, expected %q", selector["form_type"], test.expectedFormType)
			}
			if selector["region_code"] != test.expectedRegionCode {
				t.Errorf("schema region_code = %q, expected %q", selector["region_code"], test.expectedRegionCode)
			}
		})
	}
}

func TestAddSchemaClaimReturnsErrorWhenSchemaURLCannotBeLoaded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	err := addSchemaClaim(make(map[string]interface{}), surveys.LauncherSchema{URL: server.URL + "/missing.json"})
	if err == nil {
		t.Fatal("expected unavailable schema URL to return an error")
	}
}
