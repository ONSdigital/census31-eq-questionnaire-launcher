// Package main is the entry point for the questionnaire launcher application
package main // import "github.com/ONSdigital/census31-eq-questionnaire-launcher"

import (
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ONSdigital/census31-eq-questionnaire-launcher/authentication"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/settings"
	"github.com/ONSdigital/census31-eq-questionnaire-launcher/surveys"
	"github.com/go-jose/go-jose/v4/json"
	"github.com/gofrs/uuid"
	"github.com/gorilla/mux"
)

func randomNumericString(n int) string {
	var letter = []rune("0123456789")

	output := make([]rune, n)
	for i := range output {
		output[i] = letter[rand.Intn(len(letter))]
	}
	return string(output)
}

func serveTemplate(templateName string, data interface{}, w http.ResponseWriter, r *http.Request) {
	lp := filepath.Join("templates", "layout.html")
	fp := filepath.Join("templates", filepath.Clean(templateName))

	// Return a 404 if the template doesn't exist or is directory
	info, err := os.Stat(fp)
	if err != nil && (os.IsNotExist(err) || info.IsDir()) {
		log.Println("Cannot find: " + fp)
		http.NotFound(w, r)
		return
	}

	// Register custom template functions
	tmpl := template.New("layout.html").Funcs(template.FuncMap{
		"HasPrefix": strings.HasPrefix,
	})

	tmpl, err = tmpl.ParseFiles(lp, fp)
	if err != nil {
		log.Println(err.Error())
		http.Error(w, http.StatusText(500), 500)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Println(err.Error())
		http.Error(w, http.StatusText(500), 500)
	}
}

type page struct {
	Schemas                 map[string][]surveys.LauncherSchema
	AccountServiceURL       string
	AccountServiceLogOutURL string
}

func getStatusHandler(w http.ResponseWriter, _ *http.Request) {
	_, writeError := w.Write([]byte("OK"))
	if writeError != nil {
		http.Error(w, fmt.Sprintf("Write failed to write data as part of an HTTP reply: %v", writeError), 500)
		return
	}
}

func getLaunchHandler(w http.ResponseWriter, r *http.Request) {
	launcherURL := getLauncherURL(r)
	p := page{
		Schemas:                 surveys.GetAvailableSchemas(),
		AccountServiceURL:       launcherURL,
		AccountServiceLogOutURL: launcherURL,
	}
	serveTemplate("launch.html", p, w, r)
}

func postLaunchHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("POST. r.ParseForm() err: %v", err), 500)
		return
	}
	redirectURL(w, r)
}

func getSchemaHandler(w http.ResponseWriter, r *http.Request) {
	schemaName := r.URL.Query().Get("schema_name")
	schemaURL := r.URL.Query().Get("schema_url")

	launcherSchema := surveys.GetLauncherSchema(schemaName, schemaURL)

	schema, err := authentication.GetSchema(launcherSchema)
	if err != nil {
		http.Error(w, fmt.Sprintf("GetSchema err: %v", err), 500)
		return
	}

	schemaJSON, _ := json.Marshal(schema)

	_, writeError := w.Write([]byte(schemaJSON))
	if writeError != nil {
		http.Error(w, fmt.Sprintf("Write failed to write data as part of an HTTP reply: %v", writeError), 500)
		return
	}
}

func getLauncherURL(r *http.Request) string {
	protocol := r.Header.Get("X-Forwarded-Proto")
	if protocol == "" {
		protocol = "http"
	}

	return protocol + "://" + r.Host
}

func redirectURL(w http.ResponseWriter, r *http.Request) {
	hostURL := settings.Get("SURVEY_RUNNER_URL")

	launchAction := r.PostForm.Get("action_launch") != ""
	flushAction := r.PostForm.Get("action_flush") != ""
	log.Println("Request: " + r.PostForm.Encode())
	log.Println("POST received: ", r.PostForm)

	token, err := authentication.GenerateToken(r.PostForm, flushAction)

	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	switch {
	case flushAction:
		http.Redirect(w, r, hostURL+"/flush?token="+token, http.StatusTemporaryRedirect)
	case launchAction:
		http.Redirect(w, r, hostURL+"/session?token="+token, http.StatusMovedPermanently)
	default:
		http.Error(w, "Invalid Action", 500)
	}
}

func quickLauncherHandler(w http.ResponseWriter, r *http.Request) {
	accountServiceURL := getLauncherURL(r)
	urlValues := r.URL.Query()

	schemaURL := urlValues.Get("schema_url")
	log.Println("Quick launch request received", schemaURL)

	defaultClaims := []struct {
		name  string
		value string
	}{
		{name: "collection_exercise_sid", value: uuid.Must(uuid.NewV4()).String()},
		{name: "case_id", value: uuid.Must(uuid.NewV4()).String()},
		{name: "response_id", value: randomNumericString(16)},
		{name: "language_code", value: "en"},
		{name: "account_service_url", value: accountServiceURL},
	}

	for _, claim := range defaultClaims {
		if _, exists := urlValues[claim.name]; !exists {
			urlValues.Set(claim.name, claim.value)
		}
	}

	token, err := authentication.GenerateToken(urlValues, false)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if schemaURL != "" {
		redirectURL := settings.Get("SURVEY_RUNNER_URL") + "/session?token=" + token
		http.Redirect(w, r, redirectURL, http.StatusFound)
	} else {
		http.Error(w, "Not Found", 404)
	}
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/", getLaunchHandler).Methods("GET")
	r.HandleFunc("/", postLaunchHandler).Methods("POST")
	r.HandleFunc("/quick-launch", quickLauncherHandler).Methods("GET")
	r.HandleFunc("/schema", getSchemaHandler).Methods("GET")
	r.HandleFunc("/status", getStatusHandler).Methods("GET")

	// Serve static assets
	staticFs := http.FileServer(http.Dir("static"))
	staticHandler := http.StripPrefix("/static/", staticFs)
	r.PathPrefix("/static/").Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		staticHandler.ServeHTTP(w, r)
	}))

	// Bind to a port and pass our router in
	hostname := settings.Get("GO_LAUNCH_A_SURVEY_LISTEN_HOST") + ":" + settings.Get("GO_LAUNCH_A_SURVEY_LISTEN_PORT")

	log.Println("Listening on " + hostname)
	log.Fatal(http.ListenAndServe(hostname, r))
}
