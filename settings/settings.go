package settings

import "os"

var _settings map[string]string

func setSetting(key string, defaultValue string) {
	if value, present := os.LookupEnv(key); present {
		_settings[key] = value
	} else {
		_settings[key] = defaultValue
	}
}

func init() {
	_settings = make(map[string]string)
	setSetting("GO_LAUNCH_A_SURVEY_LISTEN_HOST", "0.0.0.0")
	setSetting("GO_LAUNCH_A_SURVEY_LISTEN_PORT", "8000")
	setSetting("SURVEY_RUNNER_URL", "http://localhost:5000")
	setSetting("SURVEY_RUNNER_SCHEMA_URL", Get("SURVEY_RUNNER_URL"))
	setSetting("SCHEMA_VALIDATOR_URL", "")
	setSetting("SURVEY_REGISTER_URL", "")
	setSetting("SDS_API_BASE_URL", "http://localhost:5003")
	setSetting("CIR_API_BASE_URL", "http://localhost:5004")
	setSetting("JWT_ENCRYPTION_KEY_PATH", "jwt-test-keys/sdc-user-authentication-encryption-sr-public-key.pem")
	setSetting("JWT_SIGNING_KEY_PATH", "jwt-test-keys/sdc-user-authentication-signing-launcher-private-key.pem")
	setSetting("OIDC_TOKEN_VALIDITY_IN_SECONDS", "3600")
	setSetting("OIDC_TOKEN_LEEWAY_IN_SECONDS", "300")
	setSetting("OIDC_TOKEN_BACKEND", "local")
	setSetting("SDS_OAUTH2_CLIENT_ID", "")
	setSetting("CIR_OAUTH2_CLIENT_ID", "")
	setSetting("SDS_ENABLED_IN_ENV", "true")
}

// Get returns the value for the specified named setting
func Get(name string) string {
	return _settings[name]
}
