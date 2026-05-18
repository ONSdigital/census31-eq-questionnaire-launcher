package authentication

import (
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/oidc"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/clients"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/settings"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/surveys"
	"github.com/gofrs/uuid"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/json"
	"gopkg.in/square/go-jose.v2/jwt"

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
	kid := fmt.Sprintf("%x", sha1.Sum(pubBytes))

	return &PrivateKeyResult{privateKey, kid}, nil
}

// QuestionnaireSchema is a minimal representation of a questionnaire schema used for extracting the metadata and questionnaire identifiers
type QuestionnaireSchema struct {
	Metadata   []Metadata `json:"metadata"`
	SchemaName string     `json:"schema_name"`
	SurveyType string     `json:"theme"`
	SurveyId   string     `json:"survey_id"`
}

// Metadata is a representation of the metadata within the schema with an additional `Default` value
type Metadata struct {
	Name      string `json:"name"`
	Validator string `json:"type"`
	Default   string `json:"default"`
}

func isTopLevelMetadata(key string) bool {
	switch key {
	case
		"case_id",
		"region_code",
		"channel",
		"language_code",
		"collection_exercise_sid",
		"response_expires_at",
		"response_id",
		"schema_name",
		"schema_url",
		"cir_instrument_id",
		"version",
		"account_service_url":
		return true
	}
	return false
}

func getSurveyMetadataFromClaims(claimValues map[string][]string, data map[string]interface{}, claims map[string]interface{}, surveyMetadata map[string]interface{}) {
	for key, value := range claimValues {
		if isTopLevelMetadata(key) {
			claims[key] = value[0]
		} else if key == "roles" {
			claims[key] = value
		} else {
			data[key] = value[0]
		}
	}
	surveyMetadata["data"] = data
	claims["survey_metadata"] = surveyMetadata
}

func generateClaimsV2(claimValues map[string][]string, schema QuestionnaireSchema) (claims map[string]interface{}) {

	var roles []string
	if rolesValues, ok := claimValues["roles"]; ok {
		roles = rolesValues
	} else {
		roles = []string{"dumper"}
	}

	claims = make(map[string]interface{})

	claims["roles"] = roles
	TxID, _ := uuid.NewV4()
	claims["tx_id"] = TxID.String()
	claims["version"] = "v2"

	// always send survey_id for business/test surveys unless it's already in survey metadata
	_, ok := claimValues["survey_id"]
	if !ok && !isSocialSurvey(schema.SurveyType) {
		claimValues["survey_id"] = []string{schema.SurveyId}
	}

	surveyMetadata := make(map[string]interface{})
	data := make(map[string]interface{})

	if isSocialSurvey(schema.SurveyType) {
		receiptingKeys := []string{"qid"}
		surveyMetadata["receipting_keys"] = receiptingKeys
	}

	getSurveyMetadataFromClaims(claimValues, data, claims, surveyMetadata)

	log.Printf("Using claims: %s", claims)

	return claims
}

// GenerateJwtClaims creates a jwtClaim needed to generate a token
func GenerateJwtClaims() (jwtClaims map[string]interface{}) {
	issued := time.Now()
	expires := issued.Add(time.Minute * 10) // TODO: Support custom exp: r.PostForm.Get("exp")

	jwtClaims = make(map[string]interface{})

	jwtClaims["iat"] = jwt.NewNumericDate(issued)
	jwtClaims["exp"] = jwt.NewNumericDate(expires)
	jti, _ := uuid.NewV4()
	jwtClaims["jti"] = jti.String()

	return jwtClaims
}

func launcherSchemaFromURL(url string) (launcherSchema surveys.LauncherSchema, error string) {
	resp, err := clients.GetHTTPClient().Get(url)
	if err != nil {
		panic(err)
	}

	if resp.StatusCode != 200 {
		return launcherSchema, fmt.Sprintf("Failed to load Schema from %s", url)
	}

	responseBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
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

func validateSchema(payload []byte) (error string) {
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

	responseBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err.Error()
	}

	if resp.StatusCode != 200 {
		return string(responseBody)
	}

	return ""
}

func getSchemaClaims(LauncherSchema surveys.LauncherSchema) map[string]interface{} {

	schemaClaims := make(map[string]interface{})
	if LauncherSchema.URL != "" {
		schemaClaims["schema_url"] = LauncherSchema.URL
	} else if LauncherSchema.CIRInstrumentID != "" {
		schemaClaims["cir_instrument_id"] = LauncherSchema.CIRInstrumentID
	}

	return schemaClaims
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

	token, err := jwt.SignedAndEncrypted(signer, encryptor).Claims(cl).CompactSerialize()

	if err != nil {
		return "", &TokenError{Desc: "Error signing and encrypting JWT", From: err}
	}

	log.Println("Created signed/encrypted JWT:", token)

	return token, nil
}

func getBooleanOrDefault(key string, values map[string][]string, defaultValue bool) bool {
	if keyValues, ok := values[key]; ok {
		booleanValue, _ := strconv.ParseBool(keyValues[0])
		return booleanValue
	}

	return defaultValue
}

func getStringOrDefault(key string, values map[string][]string, defaultValue string) string {
	if keyValues, ok := values[key]; ok {
		return keyValues[0]
	}

	return defaultValue
}

// GenerateTokenFromDefaultsV2 coverts a set of DEFAULT values into a JWT
func GenerateTokenFromDefaultsV2(schemaURL string, accountServiceURL string, urlValues url.Values) (token string, error string) {
	launcherSchema, validationError := launcherSchemaFromURL(schemaURL)
	if validationError != "" {
		return "", validationError
	}

	schema, error := getSchema(launcherSchema)
	if error != "" {
		return "", fmt.Sprintf("getSchema failed err: %v", error)
	}

	urlValues["account_service_url"] = []string{accountServiceURL}

	claims := generateClaimsV2(urlValues, schema)

	requiredSchemaMetadata, error := getRequiredSchemaMetadata(launcherSchema)
	if error != "" {
		return "", fmt.Sprintf("getRequiredSchemaMetadata failed err: %v", error)
	}

	surveyMetadata := make(map[string]interface{})
	updatedData := make(map[string]interface{})

	if claims["survey_metadata"] != nil {
		surveyMetadata = claims["survey_metadata"].(map[string]interface{})
	}

	initialData := surveyMetadata["data"].(map[string]interface{})

	for key, value := range initialData {
		updatedData[key] = value
	}

	/*
		The method call below is used to add boolean type URL parameters to requiredSchemaMetadata as without it,
		it leads to improper typing, e.g. flag_1=true, 'true' would be considered a string rather than an boolean
	*/
	requiredSchemaMetadata = addUrlBooleanMetadata(updatedData, requiredSchemaMetadata)

	for _, metadata := range requiredSchemaMetadata {
		if metadata.Validator == "boolean" {
			updatedData[metadata.Name] = getBooleanOrDefault(metadata.Name, urlValues, false)

			continue
		}
		updatedData[metadata.Name] = getStringOrDefault(metadata.Name, urlValues, metadata.Default)
	}

	surveyMetadata["data"] = updatedData
	claims["survey_metadata"] = surveyMetadata

	jwtClaims := GenerateJwtClaims()
	for key, v := range jwtClaims {
		claims[key] = v
	}

	schemaClaims := getSchemaClaims(launcherSchema)
	for key, v := range schemaClaims {
		claims[key] = v
	}

	token, tokenError := generateTokenFromClaims(claims)
	if tokenError != nil {
		return token, fmt.Sprintf("GenerateTokenFromDefaults failed err: %v", tokenError)
	}

	return token, ""
}

func addUrlBooleanMetadata(updatedMetadata map[string]interface{}, requiredSchemaMetadata []Metadata) []Metadata {
	for metadataName, metadataValue := range updatedMetadata {
		convertedValue := strings.ToLower(metadataValue.(string))
		if strings.EqualFold(convertedValue, "true") || strings.Contains(convertedValue, "false") {
			requiredSchemaMetadata = append(requiredSchemaMetadata, Metadata{Name: metadataName, Validator: "boolean", Default: "false"})
		}
	}
	return requiredSchemaMetadata
}

// TransformSchemaParamsToName Returns a schema name from business schema parameters
func TransformSchemaParamsToName(postValues url.Values) string {
	if postValues.Get("schema_name") != "" {
		return postValues["schema_name"][0]
	}

	eqId := postValues.Get("eq_id")
	formType := postValues.Get("form_type")
	schemaName := fmt.Sprintf("%s_%s", eqId, formType)

	return schemaName
}

// GenerateTokenFromPost converts a set of POST values into a JWT
func GenerateTokenFromPost(postValues url.Values) (string, string) {
	log.Println("POST received: ", postValues)

	schemaName := TransformSchemaParamsToName(postValues)
	schemaUrl := postValues.Get("schema_url")
	cirInstrumentId := postValues.Get("cir_instrument_id")

	launcherSchema := surveys.GetLauncherSchema(schemaName, schemaUrl, cirInstrumentId)

	schema, error := getSchema(launcherSchema)
	if error != "" {
		return "", fmt.Sprintf("getSchema failed err: %v", error)
	}

	var claims = generateClaimsV2(postValues, schema)

	jwtClaims := GenerateJwtClaims()
	for key, v := range jwtClaims {
		claims[key] = v
	}

	schemaClaims := getSchemaClaims(launcherSchema)
	for key, v := range schemaClaims {
		claims[key] = v
	}

	requiredMetadata, error := getRequiredSchemaMetadata(launcherSchema)
	if error != "" {
		return "", fmt.Sprintf(" getRequiredSchemaMetadata failed err: %v", error)
	}

	// Doesn't work for top level boolean metadata
	for _, metadata := range requiredMetadata {
		if metadata.Validator == "boolean" {
			surveyMetadata := claims["survey_metadata"].(map[string]interface{})["data"]
			_, isset := surveyMetadata.(map[string]interface{})[metadata.Name]
			surveyMetadata.(map[string]interface{})[metadata.Name] = isset
		}
	}

	if launcherSchema.Name != "" && claims["schema_name"] == "" {
		claims["schema_name"] = launcherSchema.Name
	}

	token, tokenError := generateTokenFromClaims(claims)
	if tokenError != nil {
		return token, fmt.Sprintf("GenerateTokenFromPost failed err: %v", tokenError)
	}

	return token, ""
}

func GetSurveyData(launcherSchema surveys.LauncherSchema) (QuestionnaireSchema, string) {
	schema, error := getSchema(launcherSchema)
	if error != "" {
		return QuestionnaireSchema{}, fmt.Sprintf("getSchema failed err: %v", error)
	}

	defaults := GetDefaultValues()

	for i, value := range schema.Metadata {

		if strings.Contains(value.Name, "BARCODE") {
			schema.Metadata[i].Default = "BAR" + fmt.Sprintf("%08d", rand.Int63n(1e8))
		} else {
			schema.Metadata[i].Default = defaults[value.Name]
		}

		if value.Validator == "boolean" {
			schema.Metadata[i].Default = "false"
		}
	}

	fillNonDefaults(schema)

	claims := make([]string, 0)
	for _, v := range schema.Metadata {
		claims = append(claims, v.Name)
	}

	mandatoryClaims := getMandatatoryClaims(schema.SurveyType, defaults)

	missingClaims := getMissingMandatoryClaims(claims, mandatoryClaims)

	schema.Metadata = append(schema.Metadata, missingClaims...)

	return schema, ""
}

// getRequiredSchemaMetadata Gets the required metadata from a schema
func getRequiredSchemaMetadata(launcherSchema surveys.LauncherSchema) ([]Metadata, string) {
	surveyData, err := GetSurveyData(launcherSchema)
	return surveyData.Metadata, err
}

func getSchema(launcherSchema surveys.LauncherSchema) (QuestionnaireSchema, string) {
	var url string
	var schema QuestionnaireSchema

	client := clients.GetHTTPClient()

	if launcherSchema.URL != "" {
		url = launcherSchema.URL
	} else if launcherSchema.CIRInstrumentID != "" {
		hostURL := settings.Get("CIR_API_BASE_URL")

		log.Println("Collection Instrument ID: ", launcherSchema.CIRInstrumentID)
		url = fmt.Sprintf("%s/v2/retrieve_collection_instrument?guid=%s", hostURL, launcherSchema.CIRInstrumentID)

		_, err := oidc.ConfigureClientAuthentication(client, "CIR_OAUTH2_CLIENT_ID")
		if err != nil {
			log.Print(err)
			return schema, fmt.Sprintf("Unable to generate CIR authentication credentials %s", url)
		}
	} else {
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

	if resp.StatusCode != 200 {
		log.Print("Invalid response code for schema from: ", url)
		return schema, fmt.Sprintf("Failed to load Schema from %s", url)
	}

	responseBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
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

func getMandatatoryClaims(surveyType string, defaults map[string]string) []Metadata {
	var claims []Metadata
	if isSocialSurvey(surveyType) {
		claims = []Metadata{
			{"qid", "false", defaults["qid"]},
		}

	} else {
		claims = []Metadata{
			{"ru_ref", "false", defaults["ru_ref"]},
			{"period_id", "false", defaults["period_id"]},
			{"user_id", "false", defaults["user_id"]},
		}
	}

	return claims
}

func isSocialSurvey(surveyType string) bool {
	return surveyType == "health" || surveyType == "social" || surveyType == "ukhsa-ons"
}

func getMissingMandatoryClaims(claims []string, mandatoryClaims []Metadata) []Metadata {
	missingClaims := make([]Metadata, 0)
	for _, v := range mandatoryClaims {
		if !(stringInSlice(v.Name, claims)) {
			missingClaims = append(missingClaims, v)
		}
	}

	return missingClaims
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func fillNonDefaults(schema QuestionnaireSchema) {
	arbitraryUuid, _ := uuid.NewV4()
	metadataValues := make(map[string]string)
	metadataValues["date"] = "2016-05-11"
	metadataValues["string"] = "Dummy text"
	metadataValues["url"] = "https://example.com"
	metadataValues["uuid"] = arbitraryUuid.String()
	metadataValues["iso_8601_date_string"] = "2016-05-10T12:34:56+00:00"
	for i, value := range schema.Metadata {
		if value.Default == "" {
			schema.Metadata[i].Default = metadataValues[(value.Validator)]
		}
	}
}

// GetDefaultValues Returns a map of default values for metadata keys
func GetDefaultValues() map[string]string {
	defaults := make(map[string]string)
	collectionExerciseSid, _ := uuid.NewV4()
	sdsDatasetId, _ := uuid.NewV4()

	var PARTICIPANT_ID = "ABC-" + fmt.Sprintf("%011d", rand.Int63n(1e11))

	defaults["collection_exercise_sid"] = collectionExerciseSid.String()
	defaults["qid"] = fmt.Sprintf("%016d", rand.Int63n(1e16))
	defaults["version"] = "v2"
	defaults["case_type"] = "B"
	defaults["user_id"] = "UNKNOWN"
	defaults["period_id"] = "201605"
	defaults["period_str"] = "May 2017"
	defaults["collection_exercise_sid"] = collectionExerciseSid.String()
	defaults["ru_ref"] = "12345678901A"
	defaults["ru_name"] = "ESSENTIAL ENTERPRISE LTD."
	defaults["ref_p_start_date"] = "2016-05-01"
	defaults["ref_p_end_date"] = "2016-05-31"
	defaults["return_by"] = "2016-06-12"
	defaults["trad_as"] = "ESSENTIAL ENTERPRISE LTD."
	defaults["employment_date"] = "2016-06-10"
	defaults["region_code"] = "GB-ENG"
	defaults["language_code"] = "en"
	defaults["case_ref"] = "1000000000000001"
	defaults["address_line1"] = "68 Abingdon Road"
	defaults["address_line2"] = ""
	defaults["locality"] = ""
	defaults["town_name"] = "Goathill"
	defaults["postcode"] = "PE12 4GH"
	defaults["display_address"] = "68 Abingdon Road, Goathill"
	defaults["country"] = "E"
	defaults["PARTICIPANT_ID"] = PARTICIPANT_ID
	defaults["FIRST_NAME"] = "John"
	defaults["TEST_QUESTIONS"] = "F"
	defaults["sds_dataset_id"] = sdsDatasetId.String()
	defaults["survey_id"] = "123"
	defaults["WINDOW_START_DATE"] = "2016-05-01"
	defaults["WINDOW_CLOSE_DATE"] = "2016-05-31"
	defaults["PORTAL_ID"] = fmt.Sprintf("%07d", rand.Int63n(1e7))
	defaults["PARTICIPANT_WINDOW_ID"] = PARTICIPANT_ID + "-" + fmt.Sprintf("%03d", rand.Int63n(1e3))

	return defaults
}
