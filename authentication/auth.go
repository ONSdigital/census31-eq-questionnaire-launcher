// Package authentication handles JWT generation and validation for survey requests
package authentication

import (
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/clients"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/settings"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/surveys"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/gofrs/uuid"
)

// KeyLoadError describes an error that can occur during key loading
type KeyLoadError struct {
	// Op is the operation which caused the error, such as
	// "read", "parse" or "cast".
	Op string

	// Err is a description of the error that occurred during the operation.
	Err string
}

func (e *KeyLoadError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Op + ": " + e.Err
}

// PublicKeyResult is a wrapper for the public key and the kid that identifies it
type PublicKeyResult struct {
	key *rsa.PublicKey
	kid string
}

// PrivateKeyResult is a wrapper for the private key and the kid that identifies it
type PrivateKeyResult struct {
	key *rsa.PrivateKey
	kid string
}

func loadEncryptionKey() (*PublicKeyResult, *KeyLoadError) {
	encryptionKeyPath := settings.Get("JWT_ENCRYPTION_KEY_PATH")

	keyData, err := os.ReadFile(encryptionKeyPath)
	if err != nil {
		return nil, &KeyLoadError{Op: "read", Err: "Failed to read encryption key from file: " + encryptionKeyPath}
	}

	block, _ := pem.Decode(keyData)
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, &KeyLoadError{Op: "parse", Err: "Failed to parse encryption key PEM"}
	}

	// Keep SHA-1 KID derivation for compatibility with runner key lookup.
	kid := fmt.Sprintf("%x", sha1.Sum(keyData))

	publicKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, &KeyLoadError{Op: "cast", Err: "Failed to cast key to rsa.PublicKey"}
	}

	return &PublicKeyResult{publicKey, kid}, nil
}

func loadSigningKey() (*PrivateKeyResult, *KeyLoadError) {
	signingKeyPath := settings.Get("JWT_SIGNING_KEY_PATH")
	keyData, err := os.ReadFile(signingKeyPath)
	if err != nil {
		return nil, &KeyLoadError{Op: "read", Err: "Failed to read signing key from file: " + signingKeyPath}
	}

	block, _ := pem.Decode(keyData)
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, &KeyLoadError{Op: "parse", Err: "Failed to parse signing key from PEM"}
	}

	PublicKey, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, &KeyLoadError{Op: "marshal", Err: "Failed to marshal public key"}
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: PublicKey,
	})
	// Keep SHA-1 KID derivation for compatibility with runner key lookup.
	kid := fmt.Sprintf("%x", sha1.Sum(pubBytes))

	return &PrivateKeyResult{privateKey, kid}, nil
}

// QuestionnaireSchema is a minimal representation of a questionnaire schema used for extracting the metadata and questionnaire identifiers
type QuestionnaireSchema struct {
	Metadata   []Metadata `json:"metadata"`
	SchemaName string     `json:"schema_name"`
	SurveyType string     `json:"theme"`
}

// Metadata is a representation of the metadata within the schema with an additional `Default` value
type Metadata struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Optional bool   `json:"optional"`
	Default  string `json:"default"`
}

var settableTopLevelMetadata = []string{
	"case_id",
	"collection_exercise_sid",
	"response_id",
	"channel",
	"language_code",
	"account_service_url",
	"account_service_log_out_url",
}

var defaultSurveyMetadataValues = map[string]string{
	"case_type":        "B",
	"user_id":          "UNKNOWN",
	"period_id":        "201605",
	"ru_ref":           "12345678901A",
	"ru_name":          "ESSENTIAL ENTERPRISE LTD.",
	"ref_p_start_date": "2016-05-01",
	"ref_p_end_date":   "2016-05-31",
	"return_by":        "2016-06-12",
	"trad_as":          "ESSENTIAL ENTERPRISE LTD.",
	"employment_date":  "2016-06-10",
	"display_address":  "68 Abingdon Road, Goathill",
}

// Get the claim value from the submitted values, reporting any missing values
func getClaimValue(submittedValues url.Values, claimName string) (string, bool, error) {
	values, ok := submittedValues[claimName]
	if !ok || len(values) == 0 {
		return "", true, nil
	}

	// Although roles can have multiple values, it's not configurable / settable via the submitted values
	if len(values) > 1 {
		return "", false, fmt.Errorf("expected one value for claim %q, got %d", claimName, len(values))
	}

	// If the value is an empty string, treat it as missing
	if values[0] == "" {
		return "", true, nil
	}

	return values[0], false, nil
}

func generateJwtClaims() (jwtClaims map[string]interface{}) {
	issued := time.Now()
	expires := issued.Add(time.Minute * 10)

	jwtClaims = make(map[string]interface{})

	jwtClaims["iat"] = jwt.NewNumericDate(issued)
	jwtClaims["exp"] = jwt.NewNumericDate(expires)
	jwtClaims["jti"] = uuid.Must(uuid.NewV4()).String()

	return jwtClaims
}

func addTopLevelClaims(claims map[string]interface{}, submittedValues url.Values, flushAction bool) error {
	if flushAction {
		claims["roles"] = []string{"flusher"}
	} else {
		claims["roles"] = []string{"dumper"}
	}

	claims["tx_id"] = uuid.Must(uuid.NewV4()).String()
	claims["version"] = "v2"

	for _, key := range settableTopLevelMetadata {
		value, missing, err := getClaimValue(submittedValues, key)
		if err != nil {
			return err
		}
		if !missing {
			claims[key] = value
		}
	}

	return nil
}

func validateRemoteSchemaExists(url string) error {
	resp, err := clients.GetHTTPClient().Get(url)
	if err != nil {
		return fmt.Errorf("failed to load schema from %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to load schema from %s", url)
	}

	return nil
}

func addSchemaClaim(claims map[string]interface{}, launcherSchema surveys.LauncherSchema) error {
	if launcherSchema.URL != "" {
		if err := validateRemoteSchemaExists(launcherSchema.URL); err != nil {
			return err
		}

		if !strings.Contains(launcherSchema.URL, "?") {
			claims["schema_url"] = launcherSchema.URL + "?bust=" + time.Now().Format("20060102150405")
		} else {
			claims["schema_url"] = launcherSchema.URL
		}

		return nil
	}

	if launcherSchema.Name != "" {
		if strings.HasPrefix(launcherSchema.Name, "census_") {
			schema, err := getCensusSchemaClaim(launcherSchema.Name)
			if err != nil {
				return err
			}
			claims["schema"] = schema
			return nil
		}

		claims["schema_name"] = launcherSchema.Name
	}

	return nil
}

func getCensusSchemaClaim(schemaName string) (map[string]string, error) {
	formTypes := []struct {
		name string
		code string
	}{
		{name: "household", code: "H"},
		{name: "individual", code: "I"},
		{name: "communal_establishment", code: "C"},
	}

	censusSchemaName := strings.TrimPrefix(schemaName, "census_")
	for _, formType := range formTypes {
		prefix := formType.name + "_"
		if strings.HasPrefix(censusSchemaName, prefix) {
			regionCode := strings.TrimPrefix(censusSchemaName, prefix)
			if regionCode == "" {
				return nil, fmt.Errorf("invalid census schema name %q", schemaName)
			}

			return map[string]string{
				"survey":      "census",
				"form_type":   formType.code,
				"region_code": strings.ToUpper(strings.ReplaceAll(regionCode, "_", "-")),
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid census schema name %q", schemaName)
}

func defaultSurveyMetadataValue(metadata Metadata) string {
	if value := defaultSurveyMetadataValues[metadata.Name]; value != "" {
		return value
	}

	switch metadata.Type {
	case "date":
		return "2016-05-11"
	case "string":
		return "Dummy text"
	case "url":
		return "https://example.com"
	case "uuid":
		return uuid.Must(uuid.NewV4()).String()
	case "iso_8601_date_string":
		return "2016-05-10T12:34:56+00:00"
	default:
		return ""
	}
}

func generateSurveyMetadataClaims(submittedValues url.Values, schemaMetadata []Metadata) (map[string]interface{}, error) {
	surveyMetadataClaims := make(map[string]interface{})

	for _, metadata := range schemaMetadata {
		name := metadata.Name
		value, missing, err := getClaimValue(submittedValues, name)
		if err != nil {
			return nil, err
		}

		if missing {
			if metadata.Optional {
				continue
			}
			if metadata.Default != "" {
				surveyMetadataClaims[name] = metadata.Default
				continue
			}
			if metadata.Type == "boolean" {
				// An unchecked checkbox will not submit a value, so default to false
				// This should use radio options so the false is explicit and this could be removed
				surveyMetadataClaims[name] = false
				continue
			}

			continue
		}

		if metadata.Type == "boolean" {
			booleanValue, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid boolean value %v for metadata %q: %w", value, name, err)
			}
			surveyMetadataClaims[name] = booleanValue
			continue
		}

		surveyMetadataClaims[name] = value
	}

	return surveyMetadataClaims, nil
}

// TokenError describes an error that can occur during JWT generation
type TokenError struct {
	// Err is a description of the error that occurred.
	Desc string

	// From is optionally the original error from which this one was caused.
	From error
}

func (e *TokenError) Error() string {
	if e == nil {
		return "<nil>"
	}
	err := e.Desc
	if e.From != nil {
		err += " (" + e.From.Error() + ")"
	}
	return err
}

func generateTokenWithClaims(cl map[string]interface{}) (string, *TokenError) {
	privateKeyResult, keyErr := loadSigningKey()
	if keyErr != nil {
		return "", &TokenError{Desc: "Error loading signing key", From: keyErr}
	}

	publicKeyResult, keyErr := loadEncryptionKey()
	if keyErr != nil {
		return "", &TokenError{Desc: "Error loading encryption key", From: keyErr}
	}

	opts := jose.SignerOptions{}
	opts.WithType("JWT")
	opts.WithHeader("kid", privateKeyResult.kid)

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: privateKeyResult.key}, &opts)
	if err != nil {
		return "", &TokenError{Desc: "Error creating JWT signer", From: err}
	}

	encryptor, err := jose.NewEncrypter(
		jose.A256GCM,
		jose.Recipient{Algorithm: jose.RSA_OAEP, Key: publicKeyResult.key, KeyID: publicKeyResult.kid},
		(&jose.EncrypterOptions{}).WithType("JWT").WithContentType("JWT"))

	if err != nil {
		return "", &TokenError{Desc: "Error creating JWT signer", From: err}
	}

	token, err := jwt.SignedAndEncrypted(signer, encryptor).Claims(cl).Serialize()

	if err != nil {
		return "", &TokenError{Desc: "Error signing and encrypting JWT", From: err}
	}

	log.Println("Created signed/encrypted JWT:", token)

	return token, nil
}

// GenerateToken creates a signed and encrypted JWT from the submitted values
func GenerateToken(submittedValues url.Values, flushAction bool) (string, error) {
	launcherSchema := surveys.GetLauncherSchema(submittedValues.Get("schema_name"), submittedValues.Get("schema_url"))

	claims := generateJwtClaims()

	if err := addTopLevelClaims(claims, submittedValues, flushAction); err != nil {
		return "", fmt.Errorf("add top level claims failed: %w", err)
	}

	if err := addSchemaClaim(claims, launcherSchema); err != nil {
		return "", fmt.Errorf("add schema claim failed: %w", err)
	}

	schema, err := GetSchema(launcherSchema)
	if err != nil {
		return "", fmt.Errorf("get schema failed: %w", err)
	}
	surveyMetadataClaims, err := generateSurveyMetadataClaims(submittedValues, schema.Metadata)
	if err != nil {
		return "", fmt.Errorf("generate survey metadata claims failed: %w", err)
	}
	claims["survey_metadata"] = surveyMetadataClaims

	log.Printf("Using claims: %s", claims)

	token, tokenError := generateTokenWithClaims(claims)
	if tokenError != nil {
		return token, fmt.Errorf("generate token with claims failed: %v", tokenError)
	}

	return token, nil
}

// GetSchema returns a QuestionnaireSchema and any error from the provided LauncherSchema
func GetSchema(launcherSchema surveys.LauncherSchema) (QuestionnaireSchema, error) {
	responseBody, err := getSchemaContent(launcherSchema)
	if err != nil {
		return QuestionnaireSchema{}, fmt.Errorf("failed to get schema: %w", err)
	}

	var schema QuestionnaireSchema
	if err := json.Unmarshal(responseBody, &schema); err != nil {
		log.Print(err)
		return QuestionnaireSchema{}, fmt.Errorf("failed to unmarshal schema: %w", err)
	}

	for i, metadata := range schema.Metadata {
		schema.Metadata[i].Default = defaultSurveyMetadataValue(metadata)
	}

	return schema, nil
}

func getSchemaContent(launcherSchema surveys.LauncherSchema) ([]byte, error) {
	var url string

	client := clients.GetHTTPClient()

	switch {
	case launcherSchema.URL != "":
		url = launcherSchema.URL
	default:
		hostURL := settings.Get("SURVEY_RUNNER_SCHEMA_URL")

		log.Println("Name: ", launcherSchema.Name)
		url = fmt.Sprintf("%s/schemas/%s", hostURL, launcherSchema.Name)
	}

	log.Println("Loading schema from:", url)

	resp, err := client.Get(url)
	if err != nil {
		log.Println("Failed to load schema from:", url)
		return nil, fmt.Errorf("failed to load schema from %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		log.Print("Invalid response code for schema from: ", url)
		return nil, fmt.Errorf("failed to load schema from %s", url)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Print(err)
		return nil, fmt.Errorf("failed to load schema from %s: %w", url, err)
	}

	return responseBody, nil
}
