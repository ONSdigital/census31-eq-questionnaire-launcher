// Package authentication handles JWT generation and validation for survey requests
package authentication

import (
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/clients"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/settings"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/surveys"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/gofrs/uuid"

	"bytes"
	"log"
	"path"
	"strconv"
	"strings"
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
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default string `json:"default"`
}

func isTopLevelMetadata(key string) bool {
	switch key {
	case
		"case_id",
		"region_code",
		"channel",
		"language_code",
		"collection_exercise_sid",
		"response_id",
		"schema_name",
		"schema_url",
		"version",
		"roles",
		"account_service_url":
		return true
	}
	return false
}

func addJwtClaims(claims map[string]interface{}) {
	for key, value := range GenerateJwtClaims() {
		claims[key] = value
	}
}

func parseClaimValues(claimValues url.Values) map[string]interface{} {
	parsedClaimValues := make(map[string]interface{})

	for key, value := range claimValues {
		if len(value) == 0 || value[0] == "" {
			continue
		}

		if key == "roles" {
			parsedClaimValues[key] = value

			continue
		}

		booleanValue, err := strconv.ParseBool(value[0])
		if err == nil {
			parsedClaimValues[key] = booleanValue

			continue
		}

		parsedClaimValues[key] = value[0]
	}

	return parsedClaimValues
}

func addTopLevelClaims(claims map[string]interface{}, claimValues map[string]interface{}) {
	var roles []string
	if rolesValues, ok := claimValues["roles"].([]string); ok {
		roles = rolesValues
	} else {
		roles = []string{"dumper"}
	}

	claims["roles"] = roles
	claims["tx_id"] = uuid.Must(uuid.NewV4()).String()
	claims["version"] = "v2"

	for key, value := range claimValues {
		if _, ok := claims[key]; ok {
			continue
		}

		if isTopLevelMetadata(key) {
			claims[key] = value
		}
	}
}

func addSchemaClaim(claims map[string]interface{}, launcherSchema surveys.LauncherSchema) {
	if launcherSchema.URL != "" {
		claims["schema_url"] = launcherSchema.URL
		delete(claims, "schema_name")

		return
	}

	if launcherSchema.Name != "" {
		claims["schema_name"] = launcherSchema.Name
		delete(claims, "schema_url")
	}
}

func requiredSchemaMetadataByName(launcherSchema surveys.LauncherSchema) (map[string]Metadata, string) {
	surveyData, err := GetSurveyData(launcherSchema)
	if err != "" {
		return nil, fmt.Sprintf("GetSurveyData failed err: %v", err)
	}

	requiredSchemaMetadata := make(map[string]Metadata)
	for _, metadata := range surveyData.Metadata {
		requiredSchemaMetadata[metadata.Name] = metadata
	}

	return requiredSchemaMetadata, ""
}

func filterSurveyMetadataClaims(claimValues map[string]interface{}, requiredSchemaMetadata map[string]Metadata, includeDefaults bool) map[string]interface{} {
	surveyMetadataValues := make(map[string]interface{})

	for name, metadata := range requiredSchemaMetadata {
		if metadata.Type == "boolean" {
			value, isset := claimValues[name]
			if !isset {
				surveyMetadataValues[name] = false
				continue
			}

			if booleanValue, ok := value.(bool); ok {
				surveyMetadataValues[name] = booleanValue
				continue
			}

			surveyMetadataValues[name] = true
			continue
		}

		if value, ok := claimValues[name]; ok {
			surveyMetadataValues[name] = value
			continue
		}

		if includeDefaults {
			if metadata.Default == "" {
				continue
			}

			booleanValue, err := strconv.ParseBool(metadata.Default)
			if err == nil {
				surveyMetadataValues[name] = booleanValue
				continue
			}

			surveyMetadataValues[name] = metadata.Default
		}
	}

	return surveyMetadataValues
}

// GenerateJwtClaims creates a jwtClaim needed to generate a token
func GenerateJwtClaims() (jwtClaims map[string]interface{}) {
	issued := time.Now()
	expires := issued.Add(time.Minute * 10) // Future enhancement: support custom exp via request payload.

	jwtClaims = make(map[string]interface{})

	jwtClaims["iat"] = jwt.NewNumericDate(issued)
	jwtClaims["exp"] = jwt.NewNumericDate(expires)
	jti, _ := uuid.NewV4()
	jwtClaims["jti"] = jti.String()

	return jwtClaims
}

func launcherSchemaFromURL(url string) (launcherSchema surveys.LauncherSchema, errMsg string) {
	resp, err := clients.GetHTTPClient().Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		return launcherSchema, fmt.Sprintf("Failed to load Schema from %s", url)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	validationError := validateSchema(responseBody)
	if validationError != "" {
		return launcherSchema, validationError
	}

	var schema QuestionnaireSchema
	if err := json.Unmarshal(responseBody, &schema); err != nil {
		panic(err)
	}

	cacheBust := ""
	if !strings.Contains(url, "?") {
		cacheBust = "?bust=" + time.Now().Format("20060102150405")
	}

	schemaName := ""

	if schema.SchemaName == "" {
		lastSlash := strings.LastIndex(url, "/")
		if lastSlash != -1 {
			lastDot := strings.LastIndex(url, ".")
			if lastDot == -1 {
				lastDot = len(url)
			}
			schemaName = url[lastSlash+1 : lastDot]
		}
	} else {
		schemaName = schema.SchemaName
	}

	launcherSchema = surveys.LauncherSchema{
		URL:        url + cacheBust,
		Name:       schemaName,
		SurveyType: schema.SurveyType,
	}

	return launcherSchema, ""
}

func validateSchema(payload []byte) (errMsg string) {
	if settings.Get("SCHEMA_VALIDATOR_URL") == "" {
		return ""
	}

	validateURL, _ := url.Parse(settings.Get("SCHEMA_VALIDATOR_URL"))
	validateURL.Path = path.Join(validateURL.Path, "validate")

	log.Println("Validating schema: ", validateURL.String())

	resp, err := http.Post(validateURL.String(), "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close() //nolint:errcheck

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err.Error()
	}

	if resp.StatusCode != 200 {
		return string(responseBody)
	}

	return ""
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

// generateTokenFromClaims creates a token though encryption using the private and public keys
func generateTokenFromClaims(cl map[string]interface{}) (string, *TokenError) {
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

// GenerateTokenFromDefaults coverts a set of DEFAULT values into a JWT
func GenerateTokenFromDefaults(schemaURL string, urlValues url.Values) (token string, errMsg string) {
	launcherSchema, validationError := launcherSchemaFromURL(schemaURL)
	if validationError != "" {
		return "", validationError
	}

	parsedClaimValues := parseClaimValues(urlValues)
	requiredSchemaMetadata, requiredSchemaMetadataErr := requiredSchemaMetadataByName(launcherSchema)
	if requiredSchemaMetadataErr != "" {
		return "", requiredSchemaMetadataErr
	}

	claims := make(map[string]interface{})
	addJwtClaims(claims)
	addTopLevelClaims(claims, parsedClaimValues)
	addSchemaClaim(claims, launcherSchema)
	claims["survey_metadata"] = filterSurveyMetadataClaims(parsedClaimValues, requiredSchemaMetadata, true)
	log.Printf("Using claims: %s", claims)

	token, tokenError := generateTokenFromClaims(claims)
	if tokenError != nil {
		return token, fmt.Sprintf("GenerateTokenFromDefaults failed err: %v", tokenError)
	}

	return token, ""
}

// GenerateTokenFromPost converts a set of POST values into a JWT
func GenerateTokenFromPost(postValues url.Values) (string, string) {
	log.Println("POST received: ", postValues)

	schemaName := postValues.Get("schema_name")
	schemaURL := postValues.Get("schema_url")
	launcherSchema := surveys.GetLauncherSchema(schemaName, schemaURL)

	parsedClaimValues := parseClaimValues(postValues)
	requiredSchemaMetadata, requiredSchemaMetadataErr := requiredSchemaMetadataByName(launcherSchema)
	if requiredSchemaMetadataErr != "" {
		return "", fmt.Sprintf(" %v", requiredSchemaMetadataErr)
	}

	claims := make(map[string]interface{})
	addJwtClaims(claims)
	addTopLevelClaims(claims, parsedClaimValues)
	addSchemaClaim(claims, launcherSchema)
	claims["survey_metadata"] = filterSurveyMetadataClaims(parsedClaimValues, requiredSchemaMetadata, false)
	log.Printf("Using claims: %s", claims)

	token, tokenError := generateTokenFromClaims(claims)
	if tokenError != nil {
		return token, fmt.Sprintf("GenerateTokenFromPost failed err: %v", tokenError)
	}

	return token, ""
}

// GetSurveyData returns a QuestionnaireSchema and any error from the provided LauncherSchema
func GetSurveyData(launcherSchema surveys.LauncherSchema) (QuestionnaireSchema, string) {
	schema, err := getSchema(launcherSchema)
	if err != "" {
		return QuestionnaireSchema{}, fmt.Sprintf("getSchema failed err: %v", err)
	}

	defaults := GetDefaultValues()

	for i, value := range schema.Metadata {
		if value.Type == "boolean" {
			schema.Metadata[i].Default = "false"
		} else {
			schema.Metadata[i].Default = defaults[value.Name]
		}
	}

	fillNonDefaults(schema)
	return schema, ""
}

func getSchema(launcherSchema surveys.LauncherSchema) (QuestionnaireSchema, string) {
	var url string
	var schema QuestionnaireSchema

	client := clients.GetHTTPClient()

	switch {
	case launcherSchema.URL != "":
		url = launcherSchema.URL
	default:
		hostURL := settings.Get("SURVEY_RUNNER_SCHEMA_URL")

		log.Println("Name: ", launcherSchema.Name)
		url = fmt.Sprintf("%s/schemas/%s", hostURL, launcherSchema.Name)
	}

	log.Println("Loading metadata from schema:", url)

	resp, err := client.Get(url)
	if err != nil {
		log.Println("Failed to load schema from:", url)
		return schema, fmt.Sprintf("Failed to load Schema from %s", url)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != 200 {
		log.Print("Invalid response code for schema from: ", url)
		return schema, fmt.Sprintf("Failed to load Schema from %s", url)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Print(err)
		return schema, fmt.Sprintf("Failed to load Schema from %s", url)
	}

	if err := json.Unmarshal(responseBody, &schema); err != nil {
		log.Print(err)
		return schema, fmt.Sprintf("Failed to unmarshal Schema from %s", url)
	}

	return schema, ""
}

func fillNonDefaults(schema QuestionnaireSchema) {
	arbitraryUUID, _ := uuid.NewV4()
	metadataValues := make(map[string]string)
	metadataValues["date"] = "2016-05-11"
	metadataValues["string"] = "Dummy text"
	metadataValues["url"] = "https://example.com"
	metadataValues["uuid"] = arbitraryUUID.String()
	metadataValues["iso_8601_date_string"] = "2016-05-10T12:34:56+00:00"
	for i, value := range schema.Metadata {
		if value.Default == "" {
			schema.Metadata[i].Default = metadataValues[(value.Type)]
		}
	}
}

// GetDefaultValues Returns a map of default values for metadata keys
func GetDefaultValues() map[string]string {
	defaults := make(map[string]string)

	defaults["collection_exercise_sid"] = uuid.Must(uuid.NewV4()).String()
	defaults["case_type"] = "B"
	defaults["user_id"] = "UNKNOWN"
	defaults["period_id"] = "201605"
	defaults["ru_ref"] = "12345678901A"
	defaults["ru_name"] = "ESSENTIAL ENTERPRISE LTD."
	defaults["ref_p_start_date"] = "2016-05-01"
	defaults["ref_p_end_date"] = "2016-05-31"
	defaults["return_by"] = "2016-06-12"
	defaults["trad_as"] = "ESSENTIAL ENTERPRISE LTD."
	defaults["employment_date"] = "2016-06-10"
	defaults["region_code"] = "GB-ENG"
	defaults["language_code"] = "en"
	defaults["display_address"] = "68 Abingdon Road, Goathill"

	return defaults
}
